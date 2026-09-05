package db

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	MysqlDriver    = "mysql"
	SqliteDriver   = "sqlite"
	PostgresDriver = "postgres"
)

const (
	DefaultMaxOpenConns = 12
	DefaultMaxIdleConns = 12
)

var (
	ErrNilConfig         = errors.New("db config is required")
	ErrDriverRequired    = errors.New("db driver is required")
	ErrDSNRequired       = errors.New("db dsn is required")
	ErrUnsupportedDriver = errors.New("db driver is unsupported")
)

type Config struct {
	*gorm.Config
	Driver             string        `json:"driver"`
	DSN                string        `json:"dsn"`
	MaxOpenConns       int           `json:"maxOpenConns" default:"12"`
	MaxIdleConns       int           `json:"maxIdleConns" default:"12"`
	ConnMaxLifetime    time.Duration `json:"connMaxLifetime"`
	ConnMaxIdleTime    time.Duration `json:"connMaxIdleTime"`
	Debug              bool          `json:"debug"`
	DisableAutoMigrate bool          `json:"disableAutoMigrate"`
}

func NewDefaultConfig() *Config {
	return &Config{
		MaxOpenConns: DefaultMaxOpenConns,
		MaxIdleConns: DefaultMaxIdleConns,
	}
}

func ParseDriver(raw string) (string, error) {
	driver := strings.ToLower(strings.TrimSpace(raw))
	switch driver {
	case MysqlDriver, SqliteDriver, PostgresDriver:
		return driver, nil
	case "":
		return "", ErrDriverRequired
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedDriver, raw)
	}
}

func (c *Config) normalized() (*Config, error) {
	if c == nil {
		return nil, ErrNilConfig
	}

	cfg := *c
	driver, err := ParseDriver(c.Driver)
	if err != nil {
		return nil, err
	}
	cfg.Driver = driver

	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, ErrDSNRequired
	}

	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = DefaultMaxOpenConns
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = DefaultMaxIdleConns
	}
	cfg.Config = cloneGORMConfig(c.Config)

	return &cfg, nil
}

func cloneGORMConfig(cfg *gorm.Config) *gorm.Config {
	namingStrategy := schema.NamingStrategy{
		SingularTable: true,
	}

	if cfg == nil {
		return &gorm.Config{
			NamingStrategy: namingStrategy,
			TranslateError: true,
		}
	}

	cloned := *cfg
	if cloned.NamingStrategy == nil {
		cloned.NamingStrategy = namingStrategy
	}
	return &cloned
}
