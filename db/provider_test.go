package db

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestProviderNewSession(t *testing.T) {
	raw, err := NewWithConfig(&Config{
		Driver: SqliteDriver,
		DSN:    ":memory:",
	})
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}

	var provider Provider = raw
	ctx := context.WithValue(context.Background(), "request_id", "req-1")

	session := provider.NewSession(
		ctx,
		WithDebug(),
		WithMaster(),
		WithDeleted(),
		WithSelectForUpdate(),
	)
	if session == nil {
		t.Fatalf("NewSession() should return non-nil session")
	}
	if got := session.Statement.Context.Value("request_id"); got != "req-1" {
		t.Fatalf("session context value = %v, want req-1", got)
	}
}

func TestProviderTransaction(t *testing.T) {
	raw, err := NewWithConfig(&Config{
		Driver: SqliteDriver,
		DSN:    ":memory:",
	})
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}

	var provider Provider = raw
	ctx := context.WithValue(context.Background(), "request_id", "req-2")

	var called bool
	err = provider.Transaction(ctx, func(tx *gorm.DB) error {
		called = true
		if tx == nil {
			t.Fatalf("transaction tx should not be nil")
		}
		if got := tx.Statement.Context.Value("request_id"); got != "req-2" {
			t.Fatalf("transaction context value = %v, want req-2", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}
	if !called {
		t.Fatalf("Transaction() callback should be called")
	}
}

func TestProviderClose(t *testing.T) {
	raw, err := NewWithConfig(&Config{
		Driver: SqliteDriver,
		DSN:    ":memory:",
	})
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}

	var provider Provider = raw
	if err := provider.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestProviderNilDB(t *testing.T) {
	var provider Provider = (*DB)(nil)

	if got := provider.NewSession(context.Background()); got != nil {
		t.Fatalf("NewSession() = %v, want nil", got)
	}

	err := provider.Transaction(context.Background(), func(tx *gorm.DB) error {
		return nil
	})
	if !errors.Is(err, ErrNilSessionDB) {
		t.Fatalf("Transaction() error = %v, want ErrNilSessionDB", err)
	}
}
