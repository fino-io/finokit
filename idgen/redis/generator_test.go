package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fino-io/finokit/config"
	finoredis "github.com/fino-io/finokit/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T) (*goredis.Client, func(), error) {
	m := miniredis.NewMiniRedis()
	if err := m.Start(); err != nil {
		return nil, nil, err
	}

	opts := &goredis.Options{Addr: m.Addr()}
	cli := goredis.NewClient(opts)

	cleanup := func() {
		_ = cli.Close()
		m.Close()
	}

	return cli, cleanup, nil
}

type trackingProvider struct {
	raw        goredis.UniversalClient
	closed     bool
	closeErr   error
	closeCalls int
}

func (p *trackingProvider) Raw() goredis.UniversalClient {
	return p.raw
}

func (p *trackingProvider) Ping(ctx context.Context) error {
	return p.raw.Ping(ctx).Err()
}

func (p *trackingProvider) Close() error {
	p.closed = true
	p.closeCalls++
	return p.closeErr
}

func TestNew(t *testing.T) {
	t.Run("validates client", func(t *testing.T) {
		gen, err := NewWithClient(nil, []int64{1})
		require.ErrorIs(t, err, ErrNilClient)
		require.Nil(t, gen)

		cli, cleanup, err := newTestClient(t)
		require.NoError(t, err)
		t.Cleanup(cleanup)

		gen, err = NewWithClient(cli, nil)
		require.ErrorIs(t, err, ErrEmptyServerIDs)
		require.Nil(t, gen)
	})

	t.Run("new with provider", func(t *testing.T) {
		cli, cleanup, err := newTestClient(t)
		require.NoError(t, err)
		t.Cleanup(cleanup)

		provider, err := finoredis.Wrap(cli)
		require.NoError(t, err)
		gen, err := NewWithProvider(&Config{ServerIDs: []int64{1}}, provider)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, gen.Close())
		})

		id, err := gen.GenID(context.Background())
		require.NoError(t, err)
		require.NotZero(t, id)
	})

	t.Run("loads config", func(t *testing.T) {
		cli, cleanup, err := newTestClient(t)
		require.NoError(t, err)
		t.Cleanup(cleanup)

		loadTestConfigs(t, map[string]string{
			"redis.yaml": fmt.Sprintf("redis:\n  addresses:\n    - %s\n", cli.Options().Addr),
			"idgen.yaml": "idgen:\n  serverIDs:\n    - 1\n",
		})

		gen, err := New()
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, gen.Close())
		})

		id, err := gen.GenID(context.Background())
		require.NoError(t, err)
		require.NotZero(t, id)
	})
}

func TestGeneratorGenMultiIDs(t *testing.T) {
	ctx := context.Background()

	cli, cleanup, err := newTestClient(t)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	idgen, err := NewWithClient(cli, []int64{0, 1, 2})
	require.NoError(t, err)

	ids, err := idgen.GenMultiIDs(ctx, 10)
	assert.Nil(t, err)
	assert.Equal(t, 10, len(ids))

	id, err := idgen.GenID(ctx)
	assert.Nil(t, err)
	assert.True(t, id >= time.Now().UnixNano()-(id>>32))
}

func TestNewWithClientValidateArgs(t *testing.T) {
	idgen, err := NewWithClient(nil, []int64{1})
	require.ErrorIs(t, err, ErrNilClient)
	assert.Nil(t, idgen)

	cli, cleanup, err := newTestClient(t)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	idgen, err = NewWithClient(cli, nil)
	require.ErrorIs(t, err, ErrEmptyServerIDs)
	assert.Nil(t, idgen)
}

func TestNewWithProviderOwnsProvider(t *testing.T) {
	cli, cleanup, err := newTestClient(t)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	closeErr := errors.New("close err")
	provider := &trackingProvider{raw: cli, closeErr: closeErr}
	gen, err := NewWithProvider(&Config{ServerIDs: []int64{1}}, provider)
	require.NoError(t, err)
	require.False(t, provider.closed)

	require.ErrorIs(t, gen.Close(), closeErr)
	provider.closeErr = nil
	require.ErrorIs(t, gen.Close(), closeErr)
	assert.True(t, provider.closed)
	assert.Equal(t, 1, provider.closeCalls)
}

func loadTestConfig(t *testing.T, filename, body string) {
	t.Helper()
	loadTestConfigs(t, map[string]string{filename: body})
}

func loadTestConfigs(t *testing.T, files map[string]string) {
	t.Helper()

	if err := config.InitDefault(config.WithWatcherDisabled()); err != nil {
		t.Fatalf("InitDefault() failed: %v", err)
	}

	dir := t.TempDir()
	for filename, body := range files {
		path := filepath.Join(dir, "config", filename)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll() failed: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile() failed: %v", err)
		}
	}
	if err := config.LoadPath(filepath.Join(dir, "config")); err != nil {
		t.Fatalf("LoadPath() failed: %v", err)
	}
}
