package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fino-io/finokit/config/reader"
	"github.com/fino-io/finokit/config/source"
	sourcememory "github.com/fino-io/finokit/config/source/memory"
)

func TestNewFileSourcesFiltersBySuffix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/a.yaml", "a: 1\n")
	writeFile(t, dir+"/b.dev.yaml", "b: 2\n")
	writeFile(t, dir+"/nested/c.json", `{"c":3}`)
	writeFile(t, dir+"/ignored.txt", "ignored")

	all := newFileSources(dir, "")
	require.Len(t, all, 3)

	dev := newFileSources(dir, "dev")
	require.Len(t, dev, 1)
}

func TestDefaultSourcesUsesConfigPathEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/config.yaml", "name: fino\n")
	t.Setenv("CONFIG_PATH", dir)

	sources := defaultSources()
	require.NotEmpty(t, sources)
}

func TestWatchCloserCloseIdempotent(t *testing.T) {
	w := &watchCloser{exit: make(chan struct{})}
	require.NoError(t, w.Close())
	require.NoError(t, w.Close())
}

func TestWatchFuncUninitializedReturnsError(t *testing.T) {
	isolateDefaultConfig(t)
	defaultConfigInited.Store(false)

	closer, err := WatchFunc(nil, "any.path")
	require.Nil(t, closer)
	require.ErrorIs(t, err, ErrDefaultConfigUninitialized)
}

func TestWatchFuncReceivesUpdates(t *testing.T) {
	isolateDefaultConfig(t)

	src := sourcememory.NewSource(sourcememory.WithJSON([]byte(`{"project":{"name":"v1"}}`)))
	require.NoError(t, InitDefault(WithSource(src)))

	gotCh := make(chan string, 1)
	closer, err := WatchFunc(func(v reader.Value) {
		gotCh <- v.String("")
	}, "project.name")
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()

	updater, ok := src.(interface{ Update(*source.ChangeSet) })
	require.True(t, ok)

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case got := <-gotCh:
			require.Equal(t, "v2", got)
			return
		case <-ticker.C:
			updater.Update(&source.ChangeSet{
				Data:   []byte(`{"project":{"name":"v2"}}`),
				Format: "json",
			})
		case <-timer.C:
			t.Fatal("did not receive watch callback update in time")
		}
	}
}
