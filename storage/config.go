package storage

import (
	"errors"
	"strings"
)

const DefaultMaxObjectSize = 4 << 30

var ErrNilConfig = errors.New("storage config is required")

type Config struct {
	// Endpoint must be host:port without a scheme.
	Endpoint string `json:"endpoint"`
	// Secure enables HTTPS for the S3-compatible client.
	Secure     bool   `json:"secure"`
	Region     string `json:"region"`
	BucketName string `json:"bucketName"`
	AccessKey  string `json:"accessKey"`
	SecretKey  string `json:"secretKey"`
	// MaxObjectSize limits objects read from or written to storage.
	// The default is 4 GiB.
	MaxObjectSize int64 `json:"maxObjectSize"`
}

type ConfigOption func(*Config)

func NewConfig(opts ...ConfigOption) *Config {
	cfg := &Config{
		MaxObjectSize: DefaultMaxObjectSize,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("endpoint is required")
	}
	if strings.TrimSpace(c.BucketName) == "" {
		return errors.New("bucket name is required")
	}
	if c.MaxObjectSize <= 0 {
		return errors.New("max object size must be greater than 0")
	}
	return nil
}

func (c *Config) normalized() (*Config, error) {
	if c == nil {
		return nil, ErrNilConfig
	}

	cfg := *c
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.BucketName = strings.TrimSpace(cfg.BucketName)
	if cfg.MaxObjectSize <= 0 {
		cfg.MaxObjectSize = DefaultMaxObjectSize
	}

	return &cfg, nil
}
