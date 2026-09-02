package logs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/fino-io/finokit/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
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

	require.NoError(t, InitFromConfig())
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

	logger.Log(context.Background(), Entry{
		Level:   InfoLevel,
		Message: "persisted",
		Fields:  []Field{{Key: "user", Value: "bob"}},
	})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), `"message":"persisted"`))
	require.True(t, strings.Contains(string(data), `"user":"bob"`))
}

func TestContextLoggingAddsFieldsAndTrace(t *testing.T) {
	previous := DefaultLogger()
	recorder := &recordingLogger{}
	SetLogger(recorder)
	t.Cleanup(func() { SetLogger(previous) })

	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), span)
	ctx = WithFields(ctx, Field{Key: "request_id", Value: "request-1"})
	Ctx(ctx).Infow("handled", "result", "ok", "trace_id", "spoofed")

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	require.Equal(t, "request-1", testFieldValue(entry.Fields, "request_id"))
	require.Equal(t, "01000000000000000000000000000000", testFieldValue(entry.Fields, "trace_id"))
	require.Equal(t, "0200000000000000", testFieldValue(entry.Fields, "span_id"))
	require.Equal(t, "ok", testFieldValue(entry.Fields, "result"))
}

type recordingLogger struct {
	level   Level
	fields  []Field
	entries []Entry
}

func (l *recordingLogger) SetLevel(level Level) { l.level = level }
func (l *recordingLogger) GetLevel() Level      { return l.level }

func (l *recordingLogger) With(fields ...Field) Logger {
	return &recordingLogger{level: l.level, fields: append(append([]Field(nil), l.fields...), fields...)}
}

func (l *recordingLogger) Log(_ context.Context, entry Entry) {
	entry.Fields = append(append([]Field(nil), l.fields...), entry.Fields...)
	l.entries = append(l.entries, entry)
}

func testFieldValue(fields []Field, key string) interface{} {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
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

func TestContextCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context-caller.log")
	previous := DefaultLogger()
	t.Cleanup(func() { SetLogger(previous) })
	SetLogger(NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "file",
		File: FileConfig{
			Path:   path,
			Encode: "json",
		},
	}))

	line := logFromServerContext(context.Background())
	entry := readServerLogEntry(t, path)
	require.Equal(t, "context caller", entry["message"])
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

func logFromServerContext(ctx context.Context) int {
	_, _, line, _ := runtime.Caller(0)
	Ctx(ctx).Infow("context caller")
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
