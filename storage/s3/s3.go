package s3

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/samber/lo"

	"github.com/fino-io/finokit/logs"
	"github.com/fino-io/finokit/storage"
)

type S3Client struct {
	s3  s3iface.S3API
	cfg *storage.Config
}

func Register() {
	storage.Register(storage.VendorS3, NewS3)
}

func NewS3(cfg *storage.Config) (storage.Storage, error) {
	return NewS3Client(cfg)
}

func NewS3Client(cfg *storage.Config) (*S3Client, error) {
	if cfg == nil {
		return nil, storage.ErrNilConfig
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	newSession, err := session.NewSession(&aws.Config{
		Region:           lo.ToPtr(cfg.Region),
		Endpoint:         lo.ToPtr(cfg.Endpoint),
		Credentials:      credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, ""),
		S3ForcePathStyle: lo.ToPtr(true),
	})
	if err != nil {
		return nil, logs.NewErrorw("failed to create S3 session", "error", err)
	}

	cli := s3.New(newSession)
	return &S3Client{s3: cli, cfg: cfg}, nil
}

func (c *S3Client) Read(ctx context.Context, key string, opts ...storage.Option) (*storage.Object, error) {
	bucket := c.bucketName(opts...)
	info, err := c.stat(ctx, bucket, key)
	if err != nil {
		return nil, err
	}

	// small object, read in memory
	if info.Size < c.cfg.CacheSizeGT {
		input := &s3.GetObjectInput{
			Bucket: lo.ToPtr(bucket),
			Key:    lo.ToPtr(key),
		}
		obj, err := c.s3.GetObjectWithContext(ctx, input)
		if err != nil {
			return nil, logs.NewErrorf("failed to get object %s: %v", key, err)
		}
		defer func() {
			_ = obj.Body.Close()
		}()

		data, err := io.ReadAll(obj.Body)
		if err != nil {
			return nil, logs.NewErrorf("failed to read object body(%s), error: %v", key, err)
		}

		return &storage.Object{
			LastModified: lo.FromPtr(obj.LastModified),
			ETag:         lo.FromPtr(obj.ETag),
			Key:          key,
			ContentType:  lo.FromPtr(obj.ContentType),
			Content:      data,
			Size:         lo.FromPtr(obj.ContentLength),
		}, nil
	}

	// big object, download to local file in parts
	tempFile, err := os.CreateTemp("", "s3-download-*.tmp")
	if err != nil {
		return nil, logs.NewErrorf("failed to create temp file, error: %v", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	logs.Debugw("s3 object will be downloaded", "bucket", bucket, "key", key, "path", tempFile.Name(), "size", info.Size)

	if err := c.download(ctx, bucket, key, tempFile, opts...); err != nil {
		return nil, err
	}

	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return nil, logs.NewErrorf("failed to seek to temp file, error: %v", err)
	}

	data, err := io.ReadAll(tempFile)
	if err != nil {
		return nil, logs.NewErrorf("failed to read temp file, error: %v", err)
	}

	return &storage.Object{
		LastModified: info.LastModified,
		Key:          key,
		ContentType:  info.ContentType,
		Content:      data,
		Size:         info.Size,
	}, nil
}

func (c *S3Client) Write(ctx context.Context, obj *storage.Object, opts ...storage.Option) error {
	size, err := c.validateWriteObject(obj)
	if err != nil {
		return err
	}

	bucket := c.bucketName(opts...)
	input := &s3.PutObjectInput{
		Bucket:        lo.ToPtr(bucket),
		Key:           lo.ToPtr(obj.Key),
		Body:          bytes.NewReader(obj.Content),
		ContentLength: lo.ToPtr(size),
	}
	if obj.ContentType != "" {
		input.ContentType = lo.ToPtr(obj.ContentType)
	}

	if _, err := c.s3.PutObjectWithContext(ctx, input); err != nil {
		return logs.NewErrorw("failed to write object", "bucket", bucket, "key", obj.Key, "error", err)
	}

	return nil
}

func (c *S3Client) List(ctx context.Context, prefix string, opts ...storage.Option) (storage.ListResult, error) {
	options := storage.ApplyOptions(opts...)
	bucket := lo.CoalesceOrEmpty(options.Bucket, c.cfg.BucketName)
	input := &s3.ListObjectsV2Input{Bucket: lo.ToPtr(bucket), Prefix: lo.ToPtr(prefix)}
	if options.PageToken != "" {
		input.ContinuationToken = lo.ToPtr(options.PageToken)
	}
	output, err := c.s3.ListObjectsV2WithContext(ctx, input)
	if err != nil {
		return storage.ListResult{}, logs.NewErrorw("failed to list objects", "bucket", bucket, "prefix", prefix, "error", err)
	}

	objects := make([]storage.Object, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, storage.Object{
			Key:          lo.FromPtr(object.Key),
			Size:         lo.FromPtr(object.Size),
			LastModified: lo.FromPtr(object.LastModified),
			ETag:         lo.FromPtr(object.ETag),
		})
	}

	return storage.ListResult{
		Objects:       objects,
		NextPageToken: lo.FromPtr(output.NextContinuationToken),
	}, nil
}

func (c *S3Client) Remove(ctx context.Context, keys []string, opts ...storage.Option) error {
	if len(keys) == 0 {
		return nil
	}

	options := storage.ApplyOptions(opts...)
	bucket := lo.CoalesceOrEmpty(options.Bucket, c.cfg.BucketName)
	objects := make([]*s3.ObjectIdentifier, len(keys))
	for i, key := range keys {
		objects[i] = &s3.ObjectIdentifier{Key: lo.ToPtr(key)}
	}

	output, err := c.s3.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: lo.ToPtr(bucket),
		Delete: &s3.Delete{Objects: objects},
	})
	if err != nil {
		return logs.NewErrorw("failed to delete objects", "bucket", bucket, "error", err)
	}
	if len(output.Errors) > 0 {
		return logs.NewErrorw("failed to delete object", "bucket", bucket, "error", output.Errors[0])
	}

	return nil
}

func (c *S3Client) Download(ctx context.Context, key, path string, opts ...storage.Option) error {
	bucket := c.bucketName(opts...)
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return logs.NewErrorw("failed to create file", "path", path, "error", err)
	}
	defer func() {
		_ = f.Close()
	}()

	return c.download(ctx, bucket, key, f, opts...)
}

func (c *S3Client) Upload(ctx context.Context, localFile, key string, opts ...storage.Option) error {
	options := storage.ApplyOptions(opts...)
	bucket := c.bucketName(opts...)

	f, err := os.Open(filepath.Clean(localFile))
	if err != nil {
		return logs.NewErrorw("failed to open local file", "path", localFile, "error", err)
	}
	defer func() {
		_ = f.Close()
	}()

	uploader := s3manager.NewUploaderWithClient(c.s3, func(u *s3manager.Uploader) {
		u.PartSize = c.cfg.UploadPartSize
		u.Concurrency = options.Concurrency
	})

	output, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: lo.ToPtr(bucket),
		Key:    lo.ToPtr(key),
		Body:   f,
	})
	if err != nil {
		return logs.NewErrorw("failed to upload file", "bucket", bucket, "key", key, "path", localFile, "error", err)
	}

	logs.Infof("uploaded object %q, upload_id=%s", key, output.UploadID)
	return nil
}

func (c *S3Client) PresignedDownloadURL(ctx context.Context, key string, opts ...storage.Option) (string, error) {
	_ = ctx
	option := storage.ApplyOptions(opts...)
	bucket := lo.CoalesceOrEmpty(option.Bucket, c.cfg.BucketName)
	input := &s3.GetObjectInput{
		Bucket: lo.ToPtr(bucket),
		Key:    lo.ToPtr(key),
	}
	req, _ := c.s3.GetObjectRequest(input)
	url, _, err := req.PresignRequest(option.TTL)
	return url, err
}

func (c *S3Client) PresignedUploadURL(ctx context.Context, key string, opts ...storage.Option) (string, error) {
	_ = ctx
	option := storage.ApplyOptions(opts...)
	bucket := lo.CoalesceOrEmpty(option.Bucket, c.cfg.BucketName)
	input := &s3.PutObjectInput{
		Bucket: lo.ToPtr(bucket),
		Key:    lo.ToPtr(key),
	}
	req, _ := c.s3.PutObjectRequest(input)
	url, _, err := req.PresignRequest(option.TTL)
	return url, err
}

func (c *S3Client) Stat(ctx context.Context, key string) (*storage.Object, error) {
	return c.stat(ctx, c.cfg.BucketName, key)
}

func (c *S3Client) stat(ctx context.Context, bucket, key string) (*storage.Object, error) {
	input := &s3.HeadObjectInput{
		Bucket: lo.ToPtr(bucket),
		Key:    lo.ToPtr(key),
	}
	output, err := c.s3.HeadObjectWithContext(ctx, input, nil)
	if err != nil {
		return nil, logs.NewErrorf("failed to get s3 head object(%s): %v", key, err)
	}

	return &storage.Object{
		LastModified: lo.FromPtr(output.LastModified),
		Key:          key,
		ContentType:  lo.FromPtr(output.ContentType),
		Size:         lo.FromPtr(output.ContentLength),
	}, nil
}

func (c *S3Client) download(ctx context.Context, bucket, key string, tempFile io.WriterAt, opts ...storage.Option) error {
	options := storage.ApplyOptions(opts...)
	input := &s3.GetObjectInput{
		Bucket: lo.ToPtr(bucket),
		Key:    lo.ToPtr(key),
	}

	downloader := s3manager.NewDownloaderWithClient(c.s3, func(d *s3manager.Downloader) {
		d.PartSize = c.cfg.DownloadPartSize
		d.Concurrency = options.Concurrency
	})

	if _, err := downloader.DownloadWithContext(ctx, tempFile, input); err != nil {
		return logs.NewErrorw("failed to download object", "bucket", bucket, "key", key, "error", err)
	}

	return nil
}

func (c *S3Client) bucketName(opts ...storage.Option) string {
	option := storage.ApplyOptions(opts...)
	return lo.CoalesceOrEmpty(option.Bucket, c.cfg.BucketName)
}

func (c *S3Client) validateWriteObject(obj *storage.Object) (int64, error) {
	if obj == nil {
		return 0, logs.NewError("object is required")
	}
	if obj.Key == "" {
		return 0, logs.NewError("object key is required")
	}

	contentSize := int64(len(obj.Content))
	size := obj.Size
	if size == 0 {
		size = contentSize
	}
	if size < 0 {
		return 0, logs.NewError("object size must be greater than or equal to 0")
	}
	if size != contentSize {
		return 0, logs.NewErrorw("object size does not match content length", "key", obj.Key, "size", size, "contentLength", contentSize)
	}
	if size > c.cfg.MaxObjectSize {
		return 0, logs.NewErrorw("object size exceeds max object size", "key", obj.Key, "size", size, "maxObjectSize", c.cfg.MaxObjectSize)
	}

	return size, nil
}
