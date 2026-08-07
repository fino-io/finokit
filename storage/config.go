package storage

import (
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/fino-io/finokit/logs"
)

var (
	ErrNilConfig         = errors.New("storage config is required")
	ErrUnsupportedVendor = errors.New("storage vendor is unsupported")
)

type Config struct {
	Vendor string `json:"vendor" default:"minio"`

	// Endpoint must be host:port without a scheme.
	Endpoint string `json:"endpoint"`
	// Secure enables HTTPS for the MinIO client.
	Secure     bool   `json:"secure"`
	Region     string `json:"region"`
	BucketName string `json:"bucketName"`
	AccessKey  string `json:"accessKey"`
	SecretKey  string `json:"secretKey"`

	// For download, if the size of the object is less than CacheSizeGT, it will be
	// read into memory, or else it will be downloaded to local file in parts.
	// Default is 64KB, must be greater than 0.
	CacheSizeGT int64 `json:"cacheSizeGt"`
	// For download, the size of each part. Default is 10MB, must be greater than 0.
	DownloadPartSize int64 `json:"downloadPartSize"`
	// For upload, the size of each part. Default is 5MB, must be greater than 0.
	UploadPartSize int64 `json:"uploadPartSize"`
	// The maximum size of the uploaded object. Default is 4GB, must be greater than
	// 0.
	MaxObjectSize int64 `json:"maxObjectSize"`
}

const (
	DefaultCacheSizeGT      = 64 << 10
	DefaultUploadPartSize   = 5 << 20
	DefaultDownloadPartSize = 10 << 20
	DefaultMaxObjectSize    = 4 << 30
	MinPartSize             = s3manager.MinUploadPartSize
	DefaultConcurrency      = 3
	DefaultSignTTL          = 24 * time.Hour
)

type ConfigOption func(*Config)

func NewConfig(opts ...ConfigOption) *Config {
	cfg := &Config{
		CacheSizeGT:      DefaultCacheSizeGT,
		DownloadPartSize: DefaultDownloadPartSize,
		UploadPartSize:   DefaultUploadPartSize,
		MaxObjectSize:    DefaultMaxObjectSize,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func (c *Config) Validate() error {
	if c.BucketName == "" {
		return logs.NewError("bucket name is required")
	}
	if c.CacheSizeGT <= 0 {
		return logs.NewError("cache_size_gt must be greater than 0")
	}
	if c.DownloadPartSize < MinPartSize {
		return logs.NewErrorf("download_part_size must be greater than %d", MinPartSize)
	}
	if c.UploadPartSize < MinPartSize {
		return logs.NewErrorf("upload_part_size must be greater than %d", MinPartSize)
	}
	if c.MaxObjectSize <= 0 {
		return logs.NewError("max_object_size must be greater than 0")
	}
	return nil
}

func (c *Config) normalized() (*Config, error) {
	if c == nil {
		return nil, ErrNilConfig
	}

	cfg := *c
	cfg.Vendor = strings.ToLower(strings.TrimSpace(cfg.Vendor))
	if cfg.Vendor == "" {
		cfg.Vendor = VendorMinio
	}

	if cfg.CacheSizeGT <= 0 {
		cfg.CacheSizeGT = DefaultCacheSizeGT
	}
	if cfg.DownloadPartSize <= 0 {
		cfg.DownloadPartSize = DefaultDownloadPartSize
	}
	if cfg.UploadPartSize <= 0 {
		cfg.UploadPartSize = DefaultUploadPartSize
	}
	if cfg.MaxObjectSize <= 0 {
		cfg.MaxObjectSize = DefaultMaxObjectSize
	}

	return &cfg, nil
}
