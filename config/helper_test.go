package config

import (
	"os"
	"path/filepath"
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

	sources, err := defaultSources()
	require.NoError(t, err)
	require.NotEmpty(t, sources)
}

func TestServiceSourcesSelectsServiceDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/configs/auth-server/config.yaml", "name: auth\n")
	writeFile(t, root+"/configs/user-server/config.yaml", "name: user\n")

	previousWorkDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { require.NoError(t, os.Chdir(previousWorkDir)) })

	sources, err := serviceSources("auth-server")
	require.NoError(t, err)
	require.Len(t, sources, 1)
	changeSet, err := sources[0].Read()
	require.NoError(t, err)
	require.Equal(t, "name: auth\n", string(changeSet.Data))
}

func TestServiceSourcesSupportsMountedServiceDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/configs/config.yaml", "name: auth\n")

	previousWorkDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { require.NoError(t, os.Chdir(previousWorkDir)) })

	sources, err := serviceSources("auth-server")
	require.NoError(t, err)
	require.Len(t, sources, 1)
	changeSet, err := sources[0].Read()
	require.NoError(t, err)
	require.Equal(t, "name: auth\n", string(changeSet.Data))
}

func TestServiceSourcesRejectsInvalidServiceName(t *testing.T) {
	invalidNames := []string{
		"../auth-server",
		".",
		"..",
		filepath.Join(t.TempDir(), "auth-server"),
	}
	for _, name := range invalidNames {
		_, err := serviceSources(name)
		require.EqualError(t, err, "service name must be a single path segment")
	}
}

func TestDefaultSourcesLoadsDotEnv(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, workDir+"/.env", `FINOKIT_DOTENV_TEST="loaded value" # comment`)

	previousWorkDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(previousWorkDir)) })

	const key = "FINOKIT_DOTENV_TEST"
	previousValue, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.Setenv(key, previousValue))
		} else {
			require.NoError(t, os.Unsetenv(key))
		}
	})

	_, err = defaultSources()
	require.NoError(t, err)
	require.Equal(t, "loaded value", os.Getenv(key))
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
