package s3

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	awss3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/fino-io/finokit/storage"
)

type mockS3API struct {
	s3iface.S3API
	putObjectWithContextFunc     func(aws.Context, *awss3.PutObjectInput, ...request.Option) (*awss3.PutObjectOutput, error)
	listObjectsV2WithContextFunc func(aws.Context, *awss3.ListObjectsV2Input, ...request.Option) (*awss3.ListObjectsV2Output, error)
	deleteObjectsWithContextFunc func(aws.Context, *awss3.DeleteObjectsInput, ...request.Option) (*awss3.DeleteObjectsOutput, error)
}

func (m *mockS3API) PutObjectWithContext(ctx aws.Context, input *awss3.PutObjectInput, opts ...request.Option) (*awss3.PutObjectOutput, error) {
	return m.putObjectWithContextFunc(ctx, input, opts...)
}

func (m *mockS3API) ListObjectsV2WithContext(ctx aws.Context, input *awss3.ListObjectsV2Input, opts ...request.Option) (*awss3.ListObjectsV2Output, error) {
	return m.listObjectsV2WithContextFunc(ctx, input, opts...)
}

func (m *mockS3API) DeleteObjectsWithContext(ctx aws.Context, input *awss3.DeleteObjectsInput, opts ...request.Option) (*awss3.DeleteObjectsOutput, error) {
	return m.deleteObjectsWithContextFunc(ctx, input, opts...)
}

func TestS3ClientWrite(t *testing.T) {
	t.Run("writes object with default bucket", func(t *testing.T) {
		var gotBucket string
		var gotKey string
		var gotContentType string
		var gotContentLength int64
		var gotBody []byte

		client := &S3Client{
			s3: &mockS3API{
				putObjectWithContextFunc: func(_ aws.Context, input *awss3.PutObjectInput, _ ...request.Option) (*awss3.PutObjectOutput, error) {
					var err error
					gotBucket = aws.StringValue(input.Bucket)
					gotKey = aws.StringValue(input.Key)
					gotContentType = aws.StringValue(input.ContentType)
					gotContentLength = aws.Int64Value(input.ContentLength)
					gotBody, err = io.ReadAll(input.Body)
					if err != nil {
						t.Fatalf("unexpected body read error: %v", err)
					}
					return &awss3.PutObjectOutput{}, nil
				},
			},
			cfg: storage.NewConfig(func(cfg *storage.Config) {
				cfg.BucketName = "default-bucket"
			}),
		}

		obj := &storage.Object{
			Key:         "demo.txt",
			ContentType: "text/plain",
			Content:     []byte("hello"),
		}
		if err := client.Write(context.Background(), obj); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		if gotBucket != "default-bucket" {
			t.Fatalf("bucket = %q, want %q", gotBucket, "default-bucket")
		}
		if gotKey != "demo.txt" {
			t.Fatalf("key = %q, want %q", gotKey, "demo.txt")
		}
		if gotContentType != "text/plain" {
			t.Fatalf("contentType = %q, want %q", gotContentType, "text/plain")
		}
		if gotContentLength != int64(len("hello")) {
			t.Fatalf("contentLength = %d, want %d", gotContentLength, len("hello"))
		}
		if string(gotBody) != "hello" {
			t.Fatalf("body = %q, want %q", string(gotBody), "hello")
		}
	})

	t.Run("uses option bucket when provided", func(t *testing.T) {
		var gotBucket string

		client := &S3Client{
			s3: &mockS3API{
				putObjectWithContextFunc: func(_ aws.Context, input *awss3.PutObjectInput, _ ...request.Option) (*awss3.PutObjectOutput, error) {
					gotBucket = aws.StringValue(input.Bucket)
					return &awss3.PutObjectOutput{}, nil
				},
			},
			cfg: storage.NewConfig(func(cfg *storage.Config) {
				cfg.BucketName = "default-bucket"
			}),
		}

		err := client.Write(context.Background(), &storage.Object{
			Key:     "demo.txt",
			Content: []byte("hello"),
		}, storage.WithBucket("override-bucket"))
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if gotBucket != "override-bucket" {
			t.Fatalf("bucket = %q, want %q", gotBucket, "override-bucket")
		}
	})

	t.Run("rejects mismatched size", func(t *testing.T) {
		client := &S3Client{
			cfg: storage.NewConfig(func(cfg *storage.Config) {
				cfg.BucketName = "default-bucket"
			}),
		}

		err := client.Write(context.Background(), &storage.Object{
			Key:     "demo.txt",
			Size:    10,
			Content: []byte("hello"),
		})
		if err == nil {
			t.Fatal("Write() error = nil, want non-nil")
		}
	})
}

func TestS3ClientList(t *testing.T) {
	modified := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	client := &S3Client{
		s3: &mockS3API{
			listObjectsV2WithContextFunc: func(_ aws.Context, input *awss3.ListObjectsV2Input, _ ...request.Option) (*awss3.ListObjectsV2Output, error) {
				if got, want := aws.StringValue(input.Bucket), "override-bucket"; got != want {
					t.Fatalf("bucket = %q, want %q", got, want)
				}
				if got, want := aws.StringValue(input.Prefix), "assets/"; got != want {
					t.Fatalf("prefix = %q, want %q", got, want)
				}
				if got, want := aws.StringValue(input.ContinuationToken), "page-1"; got != want {
					t.Fatalf("continuationToken = %q, want %q", got, want)
				}
				return &awss3.ListObjectsV2Output{
					Contents: []*awss3.Object{{
						Key:          aws.String("assets/logo.svg"),
						Size:         aws.Int64(42),
						LastModified: aws.Time(modified),
						ETag:         aws.String("etag-1"),
					}},
					NextContinuationToken: aws.String("page-2"),
				}, nil
			},
		},
		cfg: storage.NewConfig(func(cfg *storage.Config) {
			cfg.BucketName = "default-bucket"
		}),
	}

	result, err := client.List(context.Background(), "assets/", storage.WithBucket("override-bucket"), storage.WithPageToken("page-1"))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := result.NextPageToken, "page-2"; got != want {
		t.Fatalf("next page token = %q, want %q", got, want)
	}
	if len(result.Objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(result.Objects))
	}
	if got, want := result.Objects[0], (storage.Object{Key: "assets/logo.svg", Size: 42, LastModified: modified, ETag: "etag-1"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("object = %#v, want %#v", got, want)
	}
}

func TestS3ClientRemove(t *testing.T) {
	t.Run("deletes all keys in one request with bucket override", func(t *testing.T) {
		client := &S3Client{
			s3: &mockS3API{
				deleteObjectsWithContextFunc: func(_ aws.Context, input *awss3.DeleteObjectsInput, _ ...request.Option) (*awss3.DeleteObjectsOutput, error) {
					if got, want := aws.StringValue(input.Bucket), "override-bucket"; got != want {
						t.Fatalf("bucket = %q, want %q", got, want)
					}
					if got, want := len(input.Delete.Objects), 2; got != want {
						t.Fatalf("delete objects = %d, want %d", got, want)
					}
					if got, want := aws.StringValue(input.Delete.Objects[0].Key), "one"; got != want {
						t.Fatalf("first key = %q, want %q", got, want)
					}
					if got, want := aws.StringValue(input.Delete.Objects[1].Key), "two"; got != want {
						t.Fatalf("second key = %q, want %q", got, want)
					}
					return &awss3.DeleteObjectsOutput{}, nil
				},
			},
			cfg: storage.NewConfig(func(cfg *storage.Config) {
				cfg.BucketName = "default-bucket"
			}),
		}

		if err := client.Remove(context.Background(), []string{"one", "two"}, storage.WithBucket("override-bucket")); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
	})

	t.Run("does not call S3 for empty keys", func(t *testing.T) {
		client := &S3Client{cfg: storage.NewConfig(func(cfg *storage.Config) {
			cfg.BucketName = "default-bucket"
		})}

		if err := client.Remove(context.Background(), nil); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
	})

	t.Run("returns an error for object delete errors", func(t *testing.T) {
		client := &S3Client{
			s3: &mockS3API{
				deleteObjectsWithContextFunc: func(_ aws.Context, _ *awss3.DeleteObjectsInput, _ ...request.Option) (*awss3.DeleteObjectsOutput, error) {
					return &awss3.DeleteObjectsOutput{Errors: []*awss3.Error{{Key: aws.String("one"), Code: aws.String("AccessDenied")}}}, nil
				},
			},
			cfg: storage.NewConfig(func(cfg *storage.Config) {
				cfg.BucketName = "default-bucket"
			}),
		}

		if err := client.Remove(context.Background(), []string{"one"}); err == nil {
			t.Fatal("Remove() error = nil, want non-nil")
		}
	})
}
