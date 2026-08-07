package db

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fino-io/finokit/config"
	"gorm.io/gorm"
)

func TestParseDriver(t *testing.T) {
	t.Run("normalize mysql", func(t *testing.T) {
		got, err := ParseDriver("  MySQL ")
		if err != nil {
			t.Fatalf("ParseDriver() error = %v", err)
		}
		if got != MysqlDriver {
			t.Fatalf("ParseDriver() = %q, want %q", got, MysqlDriver)
		}
	})

	t.Run("normalize sqlite", func(t *testing.T) {
		got, err := ParseDriver("SQLITE")
		if err != nil {
			t.Fatalf("ParseDriver() error = %v", err)
		}
		if got != SqliteDriver {
			t.Fatalf("ParseDriver() = %q, want %q", got, SqliteDriver)
		}
	})

	t.Run("empty driver", func(t *testing.T) {
		_, err := ParseDriver("  ")
		if !errors.Is(err, ErrDriverRequired) {
			t.Fatalf("ParseDriver() error = %v, want ErrDriverRequired", err)
		}
	})

	t.Run("unsupported driver", func(t *testing.T) {
		_, err := ParseDriver("oracle")
		if !errors.Is(err, ErrUnsupportedDriver) {
			t.Fatalf("ParseDriver() error = %v, want ErrUnsupportedDriver", err)
		}
	})
}

func TestConfigNormalized(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *Config
		_, err := cfg.normalized()
		if !errors.Is(err, ErrNilConfig) {
			t.Fatalf("normalized() error = %v, want ErrNilConfig", err)
		}
	})

	t.Run("missing dsn", func(t *testing.T) {
		_, err := (&Config{Driver: SqliteDriver}).normalized()
		if !errors.Is(err, ErrDSNRequired) {
			t.Fatalf("normalized() error = %v, want ErrDSNRequired", err)
		}
	})

	t.Run("fill defaults", func(t *testing.T) {
		cfg := &Config{
			Driver: " sqlite ",
			DSN:    "file:test.db",
		}
		got, err := cfg.normalized()
		if err != nil {
			t.Fatalf("normalized() error = %v", err)
		}
		if got.Driver != SqliteDriver {
			t.Fatalf("driver = %q, want %q", got.Driver, SqliteDriver)
		}
		if got.MaxOpenConns != DefaultMaxOpenConns {
			t.Fatalf("maxOpenConns = %d, want %d", got.MaxOpenConns, DefaultMaxOpenConns)
		}
		if got.MaxIdleConns != DefaultMaxIdleConns {
			t.Fatalf("maxIdleConns = %d, want %d", got.MaxIdleConns, DefaultMaxIdleConns)
		}
		if got.Config == nil {
			t.Fatalf("gorm config should not be nil")
		}
		if got.Config.NamingStrategy == nil {
			t.Fatalf("gorm naming strategy should not be nil")
		}
	})

	t.Run("clone gorm config", func(t *testing.T) {
		gormCfg := &gorm.Config{PrepareStmt: true}
		cfg := &Config{
			Config: gormCfg,
			Driver: PostgresDriver,
			DSN:    "postgres://fino",
		}
		got, err := cfg.normalized()
		if err != nil {
			t.Fatalf("normalized() error = %v", err)
		}
		if got.Config == gormCfg {
			t.Fatalf("gorm config should be cloned")
		}
		if !got.PrepareStmt {
			t.Fatalf("prepare statement option should be preserved")
		}
	})
}

func TestNewDialector(t *testing.T) {
	cases := []struct {
		name   string
		driver string
		dsn    string
	}{
		{name: "mysql", driver: MysqlDriver, dsn: "root:pwd@tcp(127.0.0.1:3306)/db"},
		{name: "sqlite", driver: SqliteDriver, dsn: ":memory:"},
		{name: "postgres", driver: PostgresDriver, dsn: "host=localhost user=test dbname=test sslmode=disable"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			d, err := newDialector(tc.driver, tc.dsn)
			if err != nil {
				t.Fatalf("newDialector() error = %v", err)
			}
			if d == nil {
				t.Fatalf("newDialector() should not return nil")
			}
		})
	}

	t.Run("unsupported driver", func(t *testing.T) {
		_, err := newDialector("oracle", "dsn")
		if !errors.Is(err, ErrUnsupportedDriver) {
			t.Fatalf("newDialector() error = %v, want ErrUnsupportedDriver", err)
		}
	})
}

func TestOpenAndNew(t *testing.T) {
	t.Run("validation error", func(t *testing.T) {
		_, err := NewWithConfig(&Config{
			Driver: "invalid",
			DSN:    "dsn",
		})
		if !errors.Is(err, ErrUnsupportedDriver) {
			t.Fatalf("NewWithConfig() error = %v, want ErrUnsupportedDriver", err)
		}
	})

	t.Run("new with config", func(t *testing.T) {
		db, err := NewWithConfig(&Config{
			Driver: "sqlite",
			DSN:    ":memory:",
		})
		if err != nil {
			t.Fatalf("NewWithConfig() error = %v", err)
		}
		if db == nil || db.DB == nil {
			t.Fatalf("NewWithConfig() should return valid db instance")
		}
		if db.Config.Driver != SqliteDriver {
			t.Fatalf("driver = %q, want %q", db.Config.Driver, SqliteDriver)
		}
		if db.Config.MaxOpenConns != DefaultMaxOpenConns {
			t.Fatalf("maxOpenConns = %d, want %d", db.Config.MaxOpenConns, DefaultMaxOpenConns)
		}
	})

	t.Run("new loads config", func(t *testing.T) {
		loadTestConfig(t, "db.yaml", "db:\n  driver: sqlite\n  dsn: \":memory:\"\n")

		db, err := New()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if db == nil || db.DB == nil {
			t.Fatalf("New() should return valid db instance")
		}
		if db.Config.Driver != SqliteDriver {
			t.Fatalf("driver = %q, want %q", db.Config.Driver, SqliteDriver)
		}
	})
}

func loadTestConfig(t *testing.T, filename, body string) {
	t.Helper()

	if err := config.InitDefault(config.WithWatcherDisabled()); err != nil {
		t.Fatalf("InitDefault() failed: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := config.LoadPath(filepath.Join(dir, "config")); err != nil {
		t.Fatalf("LoadPath() failed: %v", err)
	}
}

func TestJSONValuer(t *testing.T) {
	v := JSONValuer{}

	t.Run("marshal value", func(t *testing.T) {
		got, err := v.Value(map[string]int{"a": 1})
		if err != nil {
			t.Fatalf("Value() error = %v", err)
		}
		s, ok := got.(string)
		if !ok {
			t.Fatalf("Value() type = %T, want string", got)
		}
		var out map[string]int
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			t.Fatalf("Value() result is not valid json: %v", err)
		}
		if out["a"] != 1 {
			t.Fatalf("Value() json content is unexpected: %v", out)
		}
	})

	t.Run("nil pointer returns nil", func(t *testing.T) {
		var in *struct {
			A int `json:"a"`
		}
		got, err := v.Value(in)
		if err != nil {
			t.Fatalf("Value() error = %v", err)
		}
		if got != nil {
			t.Fatalf("Value() = %v, want nil", got)
		}
	})

	t.Run("zero struct returns nil", func(t *testing.T) {
		got, err := v.Value(struct {
			A int `json:"a"`
		}{})
		if err != nil {
			t.Fatalf("Value() error = %v", err)
		}
		if got != nil {
			t.Fatalf("Value() = %v, want nil", got)
		}
	})
}

func TestJSONScanner(t *testing.T) {
	s := JSONScanner{}

	t.Run("scan bytes", func(t *testing.T) {
		dst := struct {
			A int `json:"a"`
		}{}
		if err := s.Scan(&dst, []byte(`{"a":1}`)); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if dst.A != 1 {
			t.Fatalf("Scan() dst.A = %d, want 1", dst.A)
		}
	})

	t.Run("scan string", func(t *testing.T) {
		dst := struct {
			A int `json:"a"`
		}{}
		if err := s.Scan(&dst, `{"a":2}`); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if dst.A != 2 {
			t.Fatalf("Scan() dst.A = %d, want 2", dst.A)
		}
	})

	t.Run("nil source ignored", func(t *testing.T) {
		dst := struct {
			A int `json:"a"`
		}{A: 3}
		if err := s.Scan(&dst, nil); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if dst.A != 3 {
			t.Fatalf("Scan() dst.A = %d, want 3", dst.A)
		}
	})

	t.Run("nil destination ignored", func(t *testing.T) {
		var dst *struct {
			A int `json:"a"`
		}
		if err := s.Scan(dst, []byte(`{"a":1}`)); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
	})

	t.Run("unsupported source type", func(t *testing.T) {
		dst := struct {
			A int `json:"a"`
		}{}
		if err := s.Scan(&dst, 1); err == nil {
			t.Fatalf("Scan() expected error for unsupported source")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		dst := struct {
			A int `json:"a"`
		}{}
		if err := s.Scan(&dst, `{"a":`); err == nil {
			t.Fatalf("Scan() expected error for invalid json")
		}
	})
}

func TestGormDBJSONType(t *testing.T) {
	t.Run("nil db", func(t *testing.T) {
		if got := gormDBJSONType(nil); got != "" {
			t.Fatalf("gormDBJSONType(nil) = %q, want empty string", got)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		db, err := NewWithConfig(&Config{
			Driver: SqliteDriver,
			DSN:    ":memory:",
		})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		if got := gormDBJSONType(db.DB); got != "JSON" {
			t.Fatalf("gormDBJSONType(sqlite) = %q, want JSON", got)
		}
	})
}

func TestIsNilValue(t *testing.T) {
	var p *int
	if !isNilValue(p) {
		t.Fatalf("isNilValue(nil pointer) should be true")
	}

	x := 1
	if isNilValue(&x) {
		t.Fatalf("isNilValue(non-nil pointer) should be false")
	}

	if isNilValue(struct{}{}) {
		t.Fatalf("isNilValue(struct{}{}) should be false")
	}
}
