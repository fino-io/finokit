package db

import (
	"context"
	"errors"
	"io"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

type Provider interface {
	NewSession(ctx context.Context, opts ...Option) *gorm.DB
	Transaction(ctx context.Context, fn func(tx *gorm.DB) error, opts ...Option) error

	io.Closer
}

var _ Provider = (*DB)(nil)

var ErrNilSessionDB = errors.New("db session gorm db is nil")

type Option func(*option)

type option struct {
	tx *gorm.DB

	debug           bool
	master          bool
	deleted         bool
	selectForUpdate bool
}

func (d *DB) NewSession(ctx context.Context, opts ...Option) *gorm.DB {
	if d == nil || d.DB == nil {
		return nil
	}

	session := d.DB.Session(&gorm.Session{})

	opt := applyOptions(opts...)
	if opt.tx != nil {
		session = opt.tx
	}
	if opt.debug {
		session = session.Debug()
	}
	if opt.master {
		session = session.Clauses(dbresolver.Write)
	}
	if opt.deleted {
		session = session.Unscoped()
	}
	if opt.selectForUpdate {
		session = session.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	return session.WithContext(ctx)
}

func (d *DB) Transaction(ctx context.Context, fn func(tx *gorm.DB) error, opts ...Option) error {
	session := d.NewSession(ctx, opts...)
	if session == nil {
		return ErrNilSessionDB
	}
	return session.Transaction(fn)
}

func (d *DB) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}

	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func applyOptions(opts ...Option) *option {
	o := &option{}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

func WithDebug() Option {
	return func(o *option) {
		o.debug = true
	}
}

func WithMaster() Option {
	return func(o *option) {
		o.master = true
	}
}

func WithTransaction(tx *gorm.DB) Option {
	return func(o *option) {
		o.tx = tx
	}
}

func WithDeleted() Option {
	return func(o *option) {
		o.deleted = true
	}
}

func WithSelectForUpdate() Option {
	return func(o *option) {
		o.selectForUpdate = true
	}
}
