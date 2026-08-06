package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"

	"github.com/chaos-io/chaos/logs"
	"github.com/chaos-io/chaos/storage"
)

// 消除隐式注册，需在项目中显式注册
// func init() {
// 	storage.Register("minio", NewMinio)
// }

type Minio struct {
	client     *minio.Client
	bucketName string
}

func Register() {
	storage.Register(storage.VendorMinio, NewMinio)
}

func NewMinio(cfg *storage.Config) (storage.Storage, error) {
	if cfg == nil {
		return nil, storage.ErrNilConfig
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure,
	})
	if err != nil {
		return nil, logs.NewErrorw(fmt.Sprintf("failed to minio connect %s", cfg.Endpoint), "error", err)
	}

	m := &Minio{client: client}
	if err := m.ensureBucket(context.Background(), cfg.BucketName); err != nil {
		return nil, logs.NewErrorw("failed to get bucket name", "error", err)
	}

	return m, nil
}

func (m *Minio) ensureBucket(ctx context.Context, name string) error {
	if len(name) > 0 && name != m.bucketName {
		if ok, err := m.client.BucketExists(ctx, name); err != nil {
			return err
		} else if !ok {
			if err = m.client.MakeBucket(ctx, name, minio.MakeBucketOptions{}); err != nil {
				return logs.NewErrorw("minio failed to create bucket", "bucketName", name, "error", err)
			}
		}
		m.bucketName = name
	}

	return nil
}

func (m *Minio) Read(ctx context.Context, key string, opts ...storage.Option) (*storage.Object, error) {
	obj, err := m.client.GetObject(ctx, m.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		errResponse := &minio.ErrorResponse{}
		if errors.As(err, errResponse) && errResponse.Code == "NoSuchKey" && strings.HasPrefix(key, "/") {
			key = key[1:]
			bucketName := m.bucketName
			if pos := strings.Index(key, "/"); pos > 0 {
				bucketName = key[0:pos]
				key = key[pos:]
			}
			if obj, err = m.client.GetObject(ctx, bucketName, key, minio.GetObjectOptions{}); err != nil {
				if errors.As(err, errResponse) && errResponse.Code == "NoSuchKey" {
					return nil, logs.NewErrorw("failed to found the key", "key", key)
				}
				return nil, err
			}
		}
	}

	info, err := obj.Stat()
	if err != nil {
		errResponse := &minio.ErrorResponse{}
		if errors.As(err, errResponse) && errResponse.Code == "NoSuchKey" {
			return nil, logs.NewErrorw("failed to found the key", "key", key)
		}
		return nil, err
	}

	defer func() {
		_ = obj.Close()
	}()

	object := &storage.Object{
		ETag:         info.ETag,
		Key:          info.Key,
		LastModified: info.LastModified,
		Size:         info.Size,
		ContentType:  info.ContentType,
	}

	object.Content = make([]byte, info.Size)
	if size, err := obj.Read(object.Content); (err != nil && err != io.EOF) || size != int(info.Size) {
		return nil, logs.NewErrorw("failed to read the content from the minio object", "error", err)
	}

	return object, nil
}

func (m *Minio) Write(ctx context.Context, object *storage.Object, opts ...storage.Option) error {
	_, err := m.client.PutObject(ctx, m.bucketName, object.Key, bytes.NewReader(object.Content), object.Size, minio.PutObjectOptions{})
	return err
}

func (m *Minio) List(ctx context.Context, prefix string, opts ...storage.Option) (storage.ListResult, error) {
	option := storage.ApplyOptions(opts...)
	listCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	objectsCh := m.client.ListObjects(listCtx, coalesceBucket(option.Bucket, m.bucketName), minio.ListObjectsOptions{
		Prefix:     prefix,
		Recursive:  true,
		MaxKeys:    1000,
		StartAfter: option.PageToken,
	})
	result := storage.ListResult{Objects: make([]storage.Object, 0, 1000)}
	var err error
	for object := range objectsCh {
		if object.Err != nil {
			err = object.Err
			cancel()
			break
		}

		result.Objects = append(result.Objects, storage.Object{
			Key:          object.Key,
			Size:         object.Size,
			LastModified: object.LastModified,
			ETag:         object.ETag,
			ContentType:  object.ContentType,
		})
		if len(result.Objects) == 1000 {
			result.NextPageToken = object.Key
			cancel()
			break
		}
	}
	drainObjects(objectsCh)
	if err != nil {
		return storage.ListResult{}, err
	}

	return result, nil
}

func (m *Minio) Remove(ctx context.Context, keys []string, opts ...storage.Option) error {
	if len(keys) == 0 {
		return nil
	}

	objectsCh := make(chan minio.ObjectInfo, len(keys))
	for _, key := range keys {
		objectsCh <- minio.ObjectInfo{Key: key}
	}
	close(objectsCh)

	option := storage.ApplyOptions(opts...)
	var firstErr error
	for removeErr := range m.client.RemoveObjects(ctx, coalesceBucket(option.Bucket, m.bucketName), objectsCh, minio.RemoveObjectsOptions{}) {
		if firstErr == nil && removeErr.Err != nil {
			firstErr = removeErr.Err
		}
	}
	return firstErr
}

// drainObjects waits for MinIO's listing goroutine to exit after cancellation.
func drainObjects(objects <-chan minio.ObjectInfo) {
	for range objects {
	}
}

func (m *Minio) Download(ctx context.Context, key string, path string, opts ...storage.Option) error {
	if err := m.client.FGetObject(ctx, m.bucketName, key, path, minio.GetObjectOptions{}); err != nil {
		errResponse := &minio.ErrorResponse{}
		if errors.As(err, errResponse) && errResponse.Code == "NoSuchKey" && strings.HasPrefix(key, "/") {
			key = key[1:]
			bucketName := m.bucketName
			if pos := strings.Index(key, "/"); pos > 0 {
				bucketName = key[0:pos]
				key = key[pos:]
			}
			if err = m.client.FGetObject(ctx, bucketName, key, path, minio.GetObjectOptions{}); err == nil {
				return nil
			}
		}

		return logs.NewErrorw("minio failed to download object", "key", key, "path", path, "error", err)
	}

	return nil
}

func (m *Minio) Upload(ctx context.Context, localFile string, key string, opts ...storage.Option) error {
	if _, err := m.client.FPutObject(ctx, m.bucketName, key, localFile, minio.PutObjectOptions{}); err != nil {
		return logs.NewErrorw(fmt.Sprintf("minio failed to upload %s to %s", localFile, key), "error", err)
	}

	return nil
}

func (m *Minio) PresignedDownloadURL(ctx context.Context, key string, opts ...storage.Option) (string, error) {
	option := storage.ApplyOptions(opts...)
	presignedURL, err := m.client.PresignedGetObject(ctx, coalesceBucket(option.Bucket, m.bucketName), key, option.TTL, nil)
	if err != nil {
		return "", logs.NewErrorw("minio failed to gen presigned download url", "key", key, "error", err)
	}

	return presignedURL.String(), nil
}

func (m *Minio) PresignedUploadURL(ctx context.Context, key string, opts ...storage.Option) (string, error) {
	option := storage.ApplyOptions(opts...)
	presignedURL, err := m.client.PresignedPutObject(ctx, coalesceBucket(option.Bucket, m.bucketName), key, option.TTL)
	if err != nil {
		return "", logs.NewErrorw("minio failed to gen presigned upload url", "key", key, "error", err)
	}

	return presignedURL.String(), nil
}

func coalesceBucket(bucket, fallback string) string {
	if bucket != "" {
		return bucket
	}
	return fallback
}
