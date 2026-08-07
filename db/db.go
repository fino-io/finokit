package db

import (
	"fmt"

	"github.com/fino-io/finokit/config"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type DB struct {
	*gorm.DB
	Config *Config
}

const defaultConfigKey = "db"

func New() (*DB, error) {
	cfg := &Config{}
	if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

func NewWithConfig(cfg *Config) (*DB, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}

	dialector, err := newDialector(normalized.Driver, normalized.DSN)
	if err != nil {
		return nil, err
	}
	d, err := gorm.Open(dialector, normalized.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if normalized.Debug {
		d = d.Debug()
	}

	sqlDB, err := d.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(normalized.MaxIdleConns)
	sqlDB.SetMaxOpenConns(normalized.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(normalized.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(normalized.ConnMaxIdleTime)

	return &DB{DB: d, Config: normalized}, nil
}

func newDialector(driverName, dsn string) (gorm.Dialector, error) {
	driver, err := ParseDriver(driverName)
	if err != nil {
		return nil, err
	}

	switch driver {
	case MysqlDriver:
		return mysql.Open(dsn), nil
	case SqliteDriver:
		return sqlite.Open(dsn), nil
	case PostgresDriver:
		return postgres.Open(dsn), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDriver, driverName)
	}
}
