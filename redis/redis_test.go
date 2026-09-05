package redis

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fino-io/finokit/config"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	srv := miniredis.NewMiniRedis()
	require.NoError(t, srv.Start())
	t.Cleanup(srv.Close)
	return srv
}

func TestNew(t *testing.T) {
	t.Run("validates config", func(t *testing.T) {
		cli, err := NewWithConfig(&Config{})
		require.ErrorIs(t, err, ErrEmptyAddresses)
		assert.Nil(t, cli)

		cli, err = NewWithConfig(&Config{
			Addresses: []string{"127.0.0.1:6379", "127.0.0.1:6380"},
			DB:        1,
		})
		require.ErrorIs(t, err, ErrClusterDBUnsupported)
		assert.Nil(t, cli)

		cli, err = NewWithConfig(&Config{
			Addresses:       []string{"127.0.0.1:6379"},
			MinRetryBackoff: time.Second,
			MaxRetryBackoff: 500 * time.Millisecond,
		})
		require.ErrorIs(t, err, ErrInvalidBackoff)
		assert.Nil(t, cli)
	})

	t.Run("normalizes and applies defaults", func(t *testing.T) {
		clusterCLI, err := NewWithConfig(&Config{Addresses: []string{" 127.0.0.1:6379 ", "127.0.0.1:6379", "127.0.0.1:6380"}})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, clusterCLI.Close())
		})

		cluster, ok := clusterCLI.Raw().(*goredis.ClusterClient)
		require.True(t, ok)
		assert.Equal(t, []string{"127.0.0.1:6379", "127.0.0.1:6380"}, cluster.Options().Addrs)

		singleCLI, err := NewWithConfig(&Config{Addresses: []string{"127.0.0.1:6379"}})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, singleCLI.Close()) })

		raw, ok := singleCLI.Raw().(*goredis.Client)
		require.True(t, ok)
		opts := raw.Options()
		assert.Equal(t, 3, opts.MaxRetries)
		assert.Equal(t, 8*time.Millisecond, opts.MinRetryBackoff)
		assert.Equal(t, 512*time.Millisecond, opts.MaxRetryBackoff)
		assert.Equal(t, 10*runtime.GOMAXPROCS(0), opts.PoolSize)
		assert.Equal(t, time.Second*3, opts.ReadTimeout)
		assert.Equal(t, opts.ReadTimeout, opts.WriteTimeout)
	})

	t.Run("builds service", func(t *testing.T) {
		srv := newMiniRedis(t)

		cli, err := NewWithConfig(&Config{Addresses: []string{srv.Addr()}})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, cli.Close())
		})

		ctx := context.Background()
		require.NoError(t, cli.Raw().Set(ctx, "user:1:name", "alice", time.Minute).Err())

		value, err := cli.Raw().Get(ctx, "user:1:name").Result()
		require.NoError(t, err)
		assert.Equal(t, "alice", value)
	})

	t.Run("loads config", func(t *testing.T) {
		loadTestConfig(t, "redis.yaml", "redis:\n  addresses:\n    - 127.0.0.1:6379\n")

		cli, err := New()
		require.NoError(t, err)
		require.NotNil(t, cli)
		t.Cleanup(func() {
			require.NoError(t, cli.Close())
		})
	})

	t.Run("uses default config when section is absent", func(t *testing.T) {
		require.NoError(t, config.InitDefault(config.WithWatcherDisabled()))

		cli, err := New()
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, cli.Close())
		})

		raw, ok := cli.Raw().(*goredis.Client)
		require.True(t, ok)
		require.Equal(t, ":6379", raw.Options().Addr)
	})
}

func TestService(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		cfg := NewDefaultConfig()
		require.Equal(t, []string{":6379"}, cfg.Addresses)
		require.Equal(t, 3, cfg.MaxRetries)
	})

	t.Run("wrap rejects nil", func(t *testing.T) {
		cli, err := Wrap(nil)
		require.ErrorIs(t, err, ErrNilRawClient)
		assert.Nil(t, cli)
	})

	t.Run("ping works", func(t *testing.T) {
		srv := newMiniRedis(t)

		cli, err := NewWithConfig(&Config{Addresses: []string{srv.Addr()}})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, cli.Close())
		})

		require.NoError(t, cli.Ping(context.Background()))
		require.NoError(t, cli.Close())
		require.NoError(t, cli.Close())
	})

	t.Run("nil safety", func(t *testing.T) {
		var svc *Service
		require.ErrorIs(t, svc.Ping(context.Background()), ErrNilService)
		require.NoError(t, svc.Close())
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
