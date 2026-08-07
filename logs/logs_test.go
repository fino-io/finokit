package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/fino-io/finokit/config"
	"github.com/stretchr/testify/require"
)

func TestDefaultServiceLoadsLogsConfig(t *testing.T) {
	prev := DefaultLogger()
	t.Cleanup(func() {
		SetLogger(prev)
	})

	require.NoError(t, config.InitDefault(config.WithWatcherDisabled()))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "logs.yaml"), []byte(`logs:
  level: info
  encode: console
  output: console
`), 0o644))
	require.NoError(t, config.LoadPath(dir))

	require.NoError(t, ReloadDefaultServiceFromConfig())
	logger := DefaultLogger()
	require.Equal(t, InfoLevel, logger.GetLevel())
}

func TestServiceCompatibility(t *testing.T) {
	svc := NewService(NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "console",
	}))

	require.Equal(t, InfoLevel, svc.Logger().GetLevel())
	svc.SetLogLevel(DebugLevel)
	require.Equal(t, DebugLevel, svc.Logger().GetLevel())

	err := svc.NewErrorw("failed", "kind", "network")
	require.EqualError(t, err, "failed kind: network")

	svc.Debugf("value=%s", "x")
	svc.Infow("ready", "id", 1)
}

func TestPackageDefaultLoggerSwap(t *testing.T) {
	prev := DefaultLogger()
	t.Cleanup(func() {
		SetLogger(prev)
	})

	SetLogger(NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "console",
	}))

	SetLogLevel(DebugLevel)
	require.Equal(t, DebugLevel, DefaultLogger().GetLevel())
	Infow("hello", "id", 7)
}

func TestFileOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	logger := NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "file",
		File: FileConfig{
			Path:   path,
			Encode: "json",
			// Encode:     "console",
			MaxSize:    1,
			MaxBackups: 1,
			MaxAge:     1,
		},
	})

	logger.Log(Entry{
		Level:   InfoLevel,
		Message: "persisted",
		Fields:  []Field{{Key: "user", Value: "bob"}},
	})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), `"message":"persisted"`))
	require.True(t, strings.Contains(string(data), `"user":"bob"`))
}

func TestPackageCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "caller.log")
	prev := DefaultLogger()
	t.Cleanup(func() {
		SetLogger(prev)
	})

	SetLogger(NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "file",
		File: FileConfig{
			Path:   path,
			Encode: "json",
		},
	}))

	line := logFromServerPackage()
	entry := readServerLogEntry(t, path)
	require.Equal(t, "package caller", entry["message"])
	require.Equal(t, "logs/logs_test.go:"+strconv.Itoa(line), entry["caller"])
}

func TestServiceCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service-caller.log")
	svc := NewService(NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "file",
		File: FileConfig{
			Path:   path,
			Encode: "json",
		},
	}))

	line := logFromServerService(svc)
	entry := readServerLogEntry(t, path)
	require.Equal(t, "service caller", entry["message"])
	require.Equal(t, "logs/logs_test.go:"+strconv.Itoa(line), entry["caller"])
}

func logFromServerPackage() int {
	_, _, line, _ := runtime.Caller(0)
	Infow("package caller")
	return line + 1
}

func logFromServerService(svc *Service) int {
	_, _, line, _ := runtime.Caller(0)
	svc.Infow("service caller")
	return line + 1
}

func readServerLogEntry(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entry map[string]string
	require.NoError(t, json.Unmarshal(data, &entry))
	return entry
}
