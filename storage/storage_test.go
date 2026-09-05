package storage

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Run("defaults max object size", func(t *testing.T) {
		cfg := NewConfig()
		require.Equal(t, int64(DefaultMaxObjectSize), cfg.MaxObjectSize)
	})

	t.Run("validates required fields", func(t *testing.T) {
		cfg := NewConfig()
		require.EqualError(t, cfg.Validate(), "endpoint is required")
		cfg.Endpoint = "minio.example:9000"
		require.EqualError(t, cfg.Validate(), "bucket name is required")
		cfg.BucketName = "objects"
		require.NoError(t, cfg.Validate())
	})

	t.Run("normalizes values and restores max default", func(t *testing.T) {
		cfg, err := (&Config{
			Endpoint:      "  minio.example:9000 ",
			Region:        " us-east-1 ",
			BucketName:    " objects ",
			MaxObjectSize: 0,
		}).normalized()
		require.NoError(t, err)
		require.Equal(t, "minio.example:9000", cfg.Endpoint)
		require.Equal(t, "us-east-1", cfg.Region)
		require.Equal(t, "objects", cfg.BucketName)
		require.Equal(t, int64(DefaultMaxObjectSize), cfg.MaxObjectSize)
	})

	_, err := NewWithConfig(nil)
	require.ErrorIs(t, err, ErrNilConfig)
}

func TestClientValidateWriteObject(t *testing.T) {
	client := &Client{cfg: &Config{MaxObjectSize: 5}}

	tests := []struct {
		name    string
		object  *Object
		want    int64
		wantErr string
	}{
		{name: "nil object", wantErr: "object is required"},
		{name: "missing key", object: &Object{Content: []byte("hello")}, wantErr: "object key is required"},
		{name: "infer size", object: &Object{Key: "hello.txt", Content: []byte("hello")}, want: 5},
		{name: "mismatched size", object: &Object{Key: "hello.txt", Size: 4, Content: []byte("hello")}, wantErr: "does not match"},
		{name: "negative size", object: &Object{Key: "hello.txt", Size: -1}, wantErr: "greater than or equal to 0"},
		{name: "too large", object: &Object{Key: "hello.txt", Content: []byte("toolong")}, wantErr: "exceeds max object size"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := client.validateWriteObject(test.object)
			if test.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, test.want, got)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestClientPresignOptions(t *testing.T) {
	minioClient, err := minio.New("storage.example:9000", &minio.Options{
		Creds:        credentials.NewStaticV4("access", "secret", ""),
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	})
	require.NoError(t, err)
	client := &Client{client: minioClient, cfg: &Config{BucketName: "default"}}

	got, err := client.PresignedDownloadURL(t.Context(), "hello.txt", WithBucket("override"), WithSignTTL(time.Hour))
	require.NoError(t, err)
	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "/override/hello.txt", parsed.Path)
	require.Equal(t, "3600", parsed.Query().Get("X-Amz-Expires"))
}

func TestCollectListPageStopsAtLimit(t *testing.T) {
	objects := make(chan minio.ObjectInfo, listPageSize+1)
	for i := 0; i < listPageSize+1; i++ {
		objects <- minio.ObjectInfo{Key: fmt.Sprintf("object-%04d", i)}
	}
	close(objects)

	canceled := false
	result, err := collectListPage(objects, func() { canceled = true })
	require.NoError(t, err)
	require.Len(t, result.Objects, listPageSize)
	require.Equal(t, "object-0999", result.NextPageToken)
	require.True(t, canceled)
	_, open := <-objects
	require.False(t, open, "collectListPage must drain the source channel")
}
