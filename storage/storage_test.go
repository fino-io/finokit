package storage

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/fino-io/finokit/config"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	Register("stub", func(cfg *Config) (Storage, error) {
		return &stubStorage{bucket: cfg.BucketName}, nil
	})
	Register("stub-config", func(cfg *Config) (Storage, error) {
		return &stubStorage{bucket: cfg.BucketName}, nil
	})

	t.Run("new with config", func(t *testing.T) {
		st, err := NewWithConfig(&Config{
			Vendor:     "stub",
			BucketName: "fino",
		})
		require.NoError(t, err)
		require.IsType(t, &stubStorage{}, st)
	})

	t.Run("rejects unknown vendor", func(t *testing.T) {
		st, err := NewWithConfig(&Config{
			Vendor:     "missing",
			BucketName: "fino",
		})
		require.ErrorIs(t, err, ErrUnsupportedVendor)
		require.Nil(t, st)
	})

	t.Run("loads config", func(t *testing.T) {
		loadTestConfig(t, "storage.yaml", "storage:\n  vendor: stub-config\n  bucketName: fino\n")

		st, err := New()
		require.NoError(t, err)
		require.IsType(t, &stubStorage{}, st)
	})
}

type stubStorage struct {
	bucket        string
	listResult    ListResult
	listPrefix    string
	listOptions   []Option
	removedKeys   []string
	removeOptions []Option
}

func (s *stubStorage) Read(context.Context, string, ...Option) (*Object, error) {
	return nil, nil
}

func (s *stubStorage) Write(context.Context, *Object, ...Option) error {
	return nil
}

func (s *stubStorage) Download(context.Context, string, string, ...Option) error {
	return nil
}

func (s *stubStorage) Upload(context.Context, string, string, ...Option) error {
	return nil
}

func (s *stubStorage) PresignedDownloadURL(context.Context, string, ...Option) (string, error) {
	return "", nil
}

func (s *stubStorage) PresignedUploadURL(context.Context, string, ...Option) (string, error) {
	return "", nil
}

func (s *stubStorage) List(_ context.Context, prefix string, opts ...Option) (ListResult, error) {
	s.listPrefix = prefix
	s.listOptions = append([]Option(nil), opts...)
	return s.listResult, nil
}

func (s *stubStorage) Remove(_ context.Context, keys []string, opts ...Option) error {
	s.removedKeys = append([]string(nil), keys...)
	s.removeOptions = append([]Option(nil), opts...)
	return nil
}

func TestList(t *testing.T) {
	stub := &stubStorage{listResult: ListResult{NextPageToken: "next-page"}}
	useStorageForTest(t, stub)

	result, err := List(context.Background(), "assets/", WithPageToken("current-page"))

	require.NoError(t, err)
	require.Equal(t, stub.listResult, result)
	require.Equal(t, "assets/", stub.listPrefix)
	require.Equal(t, "current-page", ApplyOptions(stub.listOptions...).PageToken)
}

func TestRemove(t *testing.T) {
	stub := &stubStorage{}
	useStorageForTest(t, stub)

	err := Remove(context.Background(), []string{"assets/logo.svg", "assets/icon.svg"}, WithPageToken("ignored-by-provider"))

	require.NoError(t, err)
	require.Equal(t, []string{"assets/logo.svg", "assets/icon.svg"}, stub.removedKeys)
	require.Equal(t, "ignored-by-provider", ApplyOptions(stub.removeOptions...).PageToken)
}

func useStorageForTest(t *testing.T, stub Storage) {
	t.Helper()

	storage = stub
	storageOnce = sync.Once{}
	storageOnce.Do(func() {})
	t.Cleanup(func() {
		storage = nil
		storageOnce = sync.Once{}
	})
}

func loadTestConfig(t *testing.T, filename, body string) {
	t.Helper()

	if err := config.InitDefault(config.WithWatcherDisabled()); err != nil {
		t.Fatalf("InitDefault() failed: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := config.LoadPath(filepath.Join(dir, "config")); err != nil {
		t.Fatalf("LoadPath() failed: %v", err)
	}
}
