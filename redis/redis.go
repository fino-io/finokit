package redis

import (
	"errors"
	"sync"

	"github.com/fino-io/finokit/config"
	goredis "github.com/redis/go-redis/v9"
)

const defaultConfigKey = "redis"

var (
	ErrNilRawClient = errors.New("redis raw client is nil")
	ErrNilService   = errors.New("redis service is nil")
)

type Service struct {
	raw goredis.UniversalClient

	closeOnce sync.Once
	closeErr  error
}

func New() (*Service, error) {
	cfg := NewDefaultConfig()
	if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

func NewWithConfig(cfg *Config) (*Service, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}

	var cli goredis.UniversalClient
	if len(normalized.Addresses) == 1 {
		cli = goredis.NewClient(&goredis.Options{
			Addr:                  normalized.Addresses[0],
			Username:              normalized.Username,
			Password:              normalized.Password,
			DB:                    normalized.DB,
			MaxRetries:            normalized.MaxRetries,
			MinRetryBackoff:       normalized.MinRetryBackoff,
			MaxRetryBackoff:       normalized.MaxRetryBackoff,
			DialTimeout:           normalized.DialTimeout,
			ReadTimeout:           normalized.ReadTimeout,
			WriteTimeout:          normalized.WriteTimeout,
			ContextTimeoutEnabled: normalized.ContextTimeoutEnabled,
			PoolSize:              normalized.PoolSize,
			MinIdleConns:          normalized.MinIdleConns,
			PoolTimeout:           normalized.PoolTimeout,
			TLSConfig:             normalized.TLSConfig,
		})
	} else {
		cli = goredis.NewClusterClient(&goredis.ClusterOptions{
			Addrs:                 normalized.Addresses,
			Username:              normalized.Username,
			Password:              normalized.Password,
			ReadOnly:              normalized.ReadOnly,
			MaxRetries:            normalized.MaxRetries,
			MinRetryBackoff:       normalized.MinRetryBackoff,
			MaxRetryBackoff:       normalized.MaxRetryBackoff,
			DialTimeout:           normalized.DialTimeout,
			ReadTimeout:           normalized.ReadTimeout,
			WriteTimeout:          normalized.WriteTimeout,
			ContextTimeoutEnabled: normalized.ContextTimeoutEnabled,
			PoolSize:              normalized.PoolSize,
			MinIdleConns:          normalized.MinIdleConns,
			PoolTimeout:           normalized.PoolTimeout,
			TLSConfig:             normalized.TLSConfig,
		})
	}

	return &Service{raw: cli}, nil
}

func Wrap(raw goredis.UniversalClient) (*Service, error) {
	if raw == nil {
		return nil, ErrNilRawClient
	}
	return &Service{raw: raw}, nil
}
