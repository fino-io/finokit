package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const listPageSize = 1000

// Client implements Storage for MinIO and AWS S3-compatible endpoints.
type Client struct {
	client *minio.Client
	cfg    *Config
}

var _ Storage = (*Client)(nil)

func newClient(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	options := minio.Options{
		Secure:       cfg.Secure,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		options.Creds = credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	}

	client, err := minio.New(cfg.Endpoint, &options)
	if err != nil {
		return nil, fmt.Errorf("create storage client for %q: %w", cfg.Endpoint, err)
	}

	result := &Client{client: client, cfg: cfg}
	if err := result.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	return result, nil
}

// ensureBucket preserves the local MinIO behavior of provisioning the configured
// bucket on startup. Per-call bucket overrides are never created implicitly.
func (c *Client) ensureBucket(ctx context.Context) error {
	bucket := c.cfg.BucketName
	exists, err := c.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check storage bucket %q: %w", bucket, err)
	}
	if exists {
		return nil
	}

	if err := c.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: c.cfg.Region}); err != nil {
		return fmt.Errorf("create storage bucket %q: %w", bucket, err)
	}
	return nil
}

func (c *Client) Read(ctx context.Context, key string, opts ...Option) (*Object, error) {
	options := ApplyOptions(opts...)
	bucket := c.bucket(options)
	object, err := c.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q from bucket %q: %w", key, bucket, err)
	}
	defer object.Close()

	info, err := object.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat object %q from bucket %q: %w", key, bucket, err)
	}
	if info.Size > c.cfg.MaxObjectSize {
		return nil, objectTooLarge(key, info.Size, c.cfg.MaxObjectSize)
	}

	limit := c.cfg.MaxObjectSize
	if limit < int64(^uint64(0)>>1) {
		limit++
	}
	content, err := io.ReadAll(io.LimitReader(object, limit))
	if err != nil {
		return nil, fmt.Errorf("read object %q from bucket %q: %w", key, bucket, err)
	}
	if int64(len(content)) > c.cfg.MaxObjectSize {
		return nil, objectTooLarge(key, int64(len(content)), c.cfg.MaxObjectSize)
	}
	size := info.Size
	if size < 0 {
		size = int64(len(content))
	}

	return &Object{
		LastModified: info.LastModified,
		ETag:         info.ETag,
		Key:          info.Key,
		ContentType:  info.ContentType,
		Content:      content,
		Size:         size,
	}, nil
}

func (c *Client) Write(ctx context.Context, object *Object, opts ...Option) error {
	size, err := c.validateWriteObject(object)
	if err != nil {
		return err
	}

	options := ApplyOptions(opts...)
	putOptions := minio.PutObjectOptions{
		ContentType: object.ContentType,
		NumThreads:  uint(options.Concurrency),
	}
	if _, err := c.client.PutObject(ctx, c.bucket(options), object.Key, bytes.NewReader(object.Content), size, putOptions); err != nil {
		return fmt.Errorf("write object %q to bucket %q: %w", object.Key, c.bucket(options), err)
	}
	return nil
}

func (c *Client) List(ctx context.Context, prefix string, opts ...Option) (ListResult, error) {
	options := ApplyOptions(opts...)
	bucket := c.bucket(options)
	listCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	objects := c.client.ListObjects(listCtx, bucket, minio.ListObjectsOptions{
		Prefix:     prefix,
		Recursive:  true,
		MaxKeys:    listPageSize,
		StartAfter: options.PageToken,
	})
	result, firstErr := collectListPage(objects, cancel)
	if firstErr != nil {
		if ctx.Err() != nil {
			return ListResult{}, ctx.Err()
		}
		return ListResult{}, fmt.Errorf("list objects in bucket %q with prefix %q: %w", bucket, prefix, firstErr)
	}
	return result, nil
}

func collectListPage(objects <-chan minio.ObjectInfo, cancel context.CancelFunc) (ListResult, error) {
	result := ListResult{Objects: make([]Object, 0, listPageSize)}
	stopped := false
	var firstErr error
	for info := range objects {
		if stopped {
			continue
		}
		if info.Err != nil {
			firstErr = info.Err
			stopped = true
			cancel()
			continue
		}
		result.Objects = append(result.Objects, Object{
			Key:          info.Key,
			Size:         info.Size,
			LastModified: info.LastModified,
			ETag:         info.ETag,
			ContentType:  info.ContentType,
		})
		if len(result.Objects) == listPageSize {
			result.NextPageToken = info.Key
			stopped = true
			cancel()
		}
	}
	return result, firstErr
}

func (c *Client) Remove(ctx context.Context, keys []string, opts ...Option) error {
	if len(keys) == 0 {
		return nil
	}

	options := ApplyOptions(opts...)
	objects := make(chan minio.ObjectInfo, len(keys))
	for _, key := range keys {
		objects <- minio.ObjectInfo{Key: key}
	}
	close(objects)

	var firstErr error
	for result := range c.client.RemoveObjects(ctx, c.bucket(options), objects, minio.RemoveObjectsOptions{}) {
		if firstErr == nil && result.Err != nil {
			firstErr = result.Err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("remove objects from bucket %q: %w", c.bucket(options), firstErr)
	}
	return nil
}

func (c *Client) Download(ctx context.Context, key, path string, opts ...Option) error {
	options := ApplyOptions(opts...)
	if err := c.client.FGetObject(ctx, c.bucket(options), key, filepath.Clean(path), minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("download object %q to %q: %w", key, path, err)
	}
	return nil
}

func (c *Client) Upload(ctx context.Context, localFile, key string, opts ...Option) error {
	options := ApplyOptions(opts...)
	info, err := os.Stat(filepath.Clean(localFile))
	if err != nil {
		return fmt.Errorf("stat upload file %q: %w", localFile, err)
	}
	if info.Size() > c.cfg.MaxObjectSize {
		return objectTooLarge(key, info.Size(), c.cfg.MaxObjectSize)
	}

	putOptions := minio.PutObjectOptions{NumThreads: uint(options.Concurrency)}
	if _, err := c.client.FPutObject(ctx, c.bucket(options), key, filepath.Clean(localFile), putOptions); err != nil {
		return fmt.Errorf("upload file %q as object %q: %w", localFile, key, err)
	}
	return nil
}

func (c *Client) PresignedDownloadURL(ctx context.Context, key string, opts ...Option) (string, error) {
	options := ApplyOptions(opts...)
	url, err := c.client.PresignedGetObject(ctx, c.bucket(options), key, options.TTL, nil)
	if err != nil {
		return "", fmt.Errorf("presign download for object %q: %w", key, err)
	}
	return url.String(), nil
}

func (c *Client) PresignedUploadURL(ctx context.Context, key string, opts ...Option) (string, error) {
	options := ApplyOptions(opts...)
	url, err := c.client.PresignedPutObject(ctx, c.bucket(options), key, options.TTL)
	if err != nil {
		return "", fmt.Errorf("presign upload for object %q: %w", key, err)
	}
	return url.String(), nil
}

func (c *Client) bucket(options RequestOptions) string {
	if options.Bucket != "" {
		return options.Bucket
	}
	return c.cfg.BucketName
}

func (c *Client) validateWriteObject(object *Object) (int64, error) {
	if object == nil {
		return 0, errors.New("object is required")
	}
	if object.Key == "" {
		return 0, errors.New("object key is required")
	}

	contentSize := int64(len(object.Content))
	size := object.Size
	if size == 0 {
		size = contentSize
	}
	if size < 0 {
		return 0, errors.New("object size must be greater than or equal to 0")
	}
	if size != contentSize {
		return 0, fmt.Errorf("object size does not match content length: key=%q size=%d contentLength=%d", object.Key, size, contentSize)
	}
	if size > c.cfg.MaxObjectSize {
		return 0, objectTooLarge(object.Key, size, c.cfg.MaxObjectSize)
	}
	return size, nil
}

func objectTooLarge(key string, size, max int64) error {
	return fmt.Errorf("object %q exceeds max object size: size=%d max=%d", key, size, max)
}
