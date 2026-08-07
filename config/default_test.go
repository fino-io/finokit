package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fino-io/finokit/config/source"
	"github.com/fino-io/finokit/config/source/env"
	"github.com/fino-io/finokit/config/source/file"
	"github.com/fino-io/finokit/config/source/memory"
)

func writeTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)
	return path
}

func TestConfigLoadWithFile(t *testing.T) {
	path := writeTempConfigFile(t, `{"foo":"bar"}`)
	conf, err := NewConfig()
	require.NoError(t, err)

	err = conf.Load(file.NewSource(
		file.WithPath(path),
	))
	require.NoError(t, err)

	v, err := conf.Get("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", v.String(""))
}

func TestConfigLoadWithInvalidFile(t *testing.T) {
	path := writeTempConfigFile(t, `{"foo":"bar"}`)
	conf, err := NewConfig()
	require.NoError(t, err)

	err = conf.Load(file.NewSource(
		file.WithPath(path),
		file.WithPath("/i/do/not/exists.json"),
	))
	require.Error(t, err)
	require.ErrorContains(t, err, "/i/do/not/exists.json")
}

func TestConfigMerge(t *testing.T) {
	path := writeTempConfigFile(t, `{
  "amqp": {
    "host": "rabbit.platform",
    "port": 80
  },
  "handler": {
    "exchange": "springCloudBus"
  }
}`)
	t.Setenv("AMQP_HOST", "rabbit.testing.com")

	conf, err := NewConfig()
	require.NoError(t, err)

	err = conf.Load(
		file.NewSource(
			file.WithPath(path),
		),
		env.NewSource(),
	)
	require.NoError(t, err)

	actualHost, err := conf.Get("amqp", "host")
	require.NoError(t, err)
	require.Equal(t, "rabbit.testing.com", actualHost.String("backup"))
}

func TestConfigWatcherDirtyOverwrite(t *testing.T) {
	const total = 100
	ss := make([]source.Source, total)
	for i := 0; i < total; i++ {
		ss[i] = memory.NewSource(memory.WithJSON([]byte(fmt.Sprintf(`{"key%d": "val%d"}`, i, i))))
	}

	conf, err := NewConfig()
	require.NoError(t, err)

	for _, s := range ss {
		require.NoError(t, conf.Load(s))
	}

	for i := range ss {
		k := fmt.Sprintf("key%d", i)
		v := fmt.Sprintf("val%d", i)
		cc, err := conf.Get(k)
		require.NoError(t, err)
		require.Equal(t, v, cc.String(""))
	}
}
