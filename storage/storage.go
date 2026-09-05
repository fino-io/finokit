package storage

import (
	"context"

	"github.com/fino-io/finokit/config"
)

const defaultConfigKey = "storage"

//go:generate mockgen -destination=mocks/storage.go -package=mocks . Storage
type Storage interface {
	Read(ctx context.Context, key string, opts ...Option) (*Object, error)
	Write(ctx context.Context, object *Object, opts ...Option) error
	List(ctx context.Context, prefix string, opts ...Option) (ListResult, error)
	Remove(ctx context.Context, keys []string, opts ...Option) error

	Download(ctx context.Context, key, path string, opts ...Option) error
	Upload(ctx context.Context, localFile, key string, opts ...Option) error

	PresignedDownloadURL(ctx context.Context, key string, opts ...Option) (string, error)
	PresignedUploadURL(ctx context.Context, key string, opts ...Option) (string, error)
}

func New() (Storage, error) {
	cfg := NewConfig()
	if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

// NewWithConfig builds the unified S3-compatible storage client.
func NewWithConfig(cfg *Config) (Storage, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	return newClient(normalized)
}
