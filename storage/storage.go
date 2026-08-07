package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/fino-io/finokit/config"
	"github.com/fino-io/finokit/logs"
)

const (
	VendorMinio = "minio"
	VendorS3    = "s3"
)

var (
	initializers     map[string]initializer
	initializersOnce sync.Once
	initializersMu   sync.RWMutex
)

const defaultConfigKey = "storage"

type initializer func(cfg *Config) (Storage, error)

func Register(name string, init initializer) {
	if init == nil {
		panic("storage: initializer is nil")
	}

	initializersOnce.Do(func() {
		initializers = make(map[string]initializer)
	})

	initializersMu.Lock()
	initializers[cfgName(name)] = init
	initializersMu.Unlock()
}

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
	cfg := &Config{}
	if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

func NewWithConfig(cfg *Config) (Storage, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}

	initializersMu.RLock()
	init, ok := initializers[cfgName(normalized.Vendor)]
	initializersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedVendor, normalized.Vendor)
	}

	return init(normalized)
}

func cfgName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

var (
	storage     Storage
	storageOnce sync.Once
)

func GetStorage() Storage {
	storageOnce.Do(func() {
		var err error
		storage, err = New()
		if err != nil {
			logs.Warnw("failed to build storage", "error", err.Error())
			storage = NewDummyStorage()
		}
	})
	return storage
}

func Read(ctx context.Context, key string, opts ...Option) (*Object, error) {
	return GetStorage().Read(ctx, key, opts...)
}

func Write(ctx context.Context, object *Object, opts ...Option) error {
	return GetStorage().Write(ctx, object, opts...)
}

func List(ctx context.Context, prefix string, opts ...Option) (ListResult, error) {
	return GetStorage().List(ctx, prefix, opts...)
}

func Remove(ctx context.Context, keys []string, opts ...Option) error {
	return GetStorage().Remove(ctx, keys, opts...)
}

func Download(ctx context.Context, key string, path string, opts ...Option) error {
	return GetStorage().Download(ctx, key, path, opts...)
}

func Upload(ctx context.Context, localFile string, key string, opts ...Option) error {
	return GetStorage().Upload(ctx, localFile, key, opts...)
}

func PresignedDownloadURL(ctx context.Context, key string, opts ...Option) (string, error) {
	return GetStorage().PresignedDownloadURL(ctx, key, opts...)
}

func PresignedUploadURL(ctx context.Context, key string, opts ...Option) (string, error) {
	return GetStorage().PresignedUploadURL(ctx, key, opts...)
}

type DummyStorage struct {
	err error
}

func NewDummyStorage() Storage                                                   { return &DummyStorage{err: errors.New("DummyStorage: not implement")} }
func (s *DummyStorage) Read(context.Context, string, ...Option) (*Object, error) { return nil, s.err }
func (s *DummyStorage) Write(context.Context, *Object, ...Option) error          { return s.err }
func (s *DummyStorage) List(context.Context, string, ...Option) (ListResult, error) {
	return ListResult{}, s.err
}
func (s *DummyStorage) Remove(context.Context, []string, ...Option) error         { return s.err }
func (s *DummyStorage) Download(context.Context, string, string, ...Option) error { return s.err }
func (s *DummyStorage) Upload(context.Context, string, string, ...Option) error   { return s.err }
func (s *DummyStorage) PresignedDownloadURL(context.Context, string, ...Option) (string, error) {
	return "", s.err
}
func (s *DummyStorage) PresignedUploadURL(context.Context, string, ...Option) (string, error) {
	return "", s.err
}
