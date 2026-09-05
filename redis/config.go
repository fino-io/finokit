package redis

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNilConfig            = errors.New("redis config is required")
	ErrEmptyAddresses       = errors.New("redis addresses is empty")
	ErrInvalidDB            = errors.New("redis db must be >= 0")
	ErrClusterDBUnsupported = errors.New("redis cluster does not support non-zero db")
	ErrInvalidPoolSize      = errors.New("redis pool size must be >= 0")
	ErrInvalidMinIdleConns  = errors.New("redis min idle conns must be >= 0")
	ErrInvalidMaxRetries    = errors.New("redis max retries must be >= -1")
	ErrInvalidBackoff       = errors.New("redis retry backoff must be >= -1")
	ErrInvalidDialTimeout   = errors.New("redis dial timeout must be >= 0")
	ErrInvalidRWTimeout     = errors.New("redis read/write timeout must be >= -2")
)

type Config struct {
	Addresses []string `json:"addresses"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	DB        int      `json:"db"`

	MaxRetries      int           `json:"maxRetries"`
	MinRetryBackoff time.Duration `json:"minRetryBackoff"`
	MaxRetryBackoff time.Duration `json:"maxRetryBackoff"`

	DialTimeout  time.Duration `json:"dialTimeout"`
	ReadTimeout  time.Duration `json:"readTimeout"`
	WriteTimeout time.Duration `json:"writeTimeout"`

	ContextTimeoutEnabled bool `json:"contextTimeoutEnabled"`

	PoolSize     int           `json:"poolSize"`
	MinIdleConns int           `json:"minIdleConns"`
	PoolTimeout  time.Duration `json:"poolTimeout"`

	ReadOnly  bool        `json:"readOnly"`
	TLSConfig *tls.Config `json:"tlsConfig"`
}

func NewDefaultConfig() *Config {
	return &Config{
		Addresses:       []string{":6379"},
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	}
}

func normalizeConfig(cfg Config) (Config, error) {
	addresses := normalizeAddresses(cfg.Addresses)
	if len(addresses) == 0 {
		return Config{}, ErrEmptyAddresses
	}

	cfg.Addresses = addresses

	if cfg.DB < 0 {
		return Config{}, ErrInvalidDB
	}
	if len(cfg.Addresses) > 1 && cfg.DB != 0 {
		return Config{}, ErrClusterDBUnsupported
	}

	if cfg.PoolSize < 0 {
		return Config{}, ErrInvalidPoolSize
	}
	if cfg.MinIdleConns < 0 {
		return Config{}, ErrInvalidMinIdleConns
	}

	if cfg.MaxRetries < -1 {
		return Config{}, ErrInvalidMaxRetries
	}
	if cfg.MinRetryBackoff < -1 || cfg.MaxRetryBackoff < -1 {
		return Config{}, ErrInvalidBackoff
	}
	if cfg.MinRetryBackoff > 0 && cfg.MaxRetryBackoff > 0 && cfg.MinRetryBackoff > cfg.MaxRetryBackoff {
		return Config{}, fmt.Errorf("%w: minRetryBackoff > maxRetryBackoff", ErrInvalidBackoff)
	}

	if cfg.DialTimeout < 0 {
		return Config{}, ErrInvalidDialTimeout
	}
	if cfg.ReadTimeout < -2 || cfg.WriteTimeout < -2 {
		return Config{}, ErrInvalidRWTimeout
	}

	// Keep explicit, predictable defaults where go-redis also uses the same values.
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.MinRetryBackoff == 0 {
		cfg.MinRetryBackoff = 8 * time.Millisecond
	}
	if cfg.MaxRetryBackoff == 0 {
		cfg.MaxRetryBackoff = 512 * time.Millisecond
	}
	return cfg, nil
}

func (c *Config) normalized() (Config, error) {
	if c == nil {
		return Config{}, ErrNilConfig
	}
	return normalizeConfig(*c)
}

func normalizeAddresses(addresses []string) []string {
	out := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, addr := range addresses {
		a := strings.TrimSpace(addr)
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}
