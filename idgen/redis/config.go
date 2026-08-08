package redis

import (
	"errors"
	"strings"

	"github.com/fino-io/finokit/config"
	finoredis "github.com/fino-io/finokit/redis"
)

const defaultConfigKey = "idgen"

var (
	ErrNilConfig      = errors.New("idgen config is required")
	ErrEmptyServerIDs = errors.New("idgen serverIDs is empty")
)

type Config struct {
	ServerIDs []int64 `json:"serverIDs"`
	Namespace string  `json:"namespace"`
}

func New() (*Generator, error) {
	cfg := &Config{}
	if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

func NewWithConfig(cfg *Config) (*Generator, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}

	provider, err := finoredis.New()
	if err != nil {
		return nil, err
	}

	g, err := NewWithClient(provider.Raw(), normalized.ServerIDs)
	if err != nil {
		_ = provider.Close()
		return nil, err
	}

	g.namespace = normalized.Namespace
	g.closeFn = func() error {
		return provider.Close()
	}
	return g, nil
}

func NewWithProvider(cfg *Config, provider finoredis.Provider) (*Generator, error) {
	if provider == nil || provider.Raw() == nil {
		return nil, ErrNilClient
	}
	if cfg == nil {
		cfg = &Config{}
		if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
			return nil, err
		}
	}

	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}

	g, err := NewWithClient(provider.Raw(), normalized.ServerIDs)
	if err != nil {
		return nil, err
	}

	g.namespace = normalized.Namespace
	return g, nil
}

func (c *Config) normalized() (*Config, error) {
	if c == nil {
		return nil, ErrNilConfig
	}
	if len(c.ServerIDs) == 0 {
		return nil, ErrEmptyServerIDs
	}

	cfg := *c
	cfg.ServerIDs = append([]int64(nil), c.ServerIDs...)
	cfg.Namespace = strings.TrimSpace(c.Namespace)
	return &cfg, nil
}
