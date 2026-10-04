package logs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fino-io/finokit/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestNewConfigLoadsLogsConfig(t *testing.T) {
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

	require.NoError(t, NewConfig())
	logger := DefaultLogger()
	require.Equal(t, InfoLevel, logger.GetLevel())
}

func TestServiceLogs(t *testing.T) {
	recorder := &recordingLogger{level: InfoLevel}
	svc := NewService(recorder)

	require.Equal(t, InfoLevel, svc.Logger().GetLevel())
	svc.SetLogLevel(DebugLevel)
	require.Equal(t, DebugLevel, svc.Logger().GetLevel())

	err := svc.NewErrorw("failed", "kind", "network")
	require.EqualError(t, err, "failed kind: network")

	svc.Debugf("value=%s", "x")
	svc.Infow("ready", "id", 1)
	require.Len(t, recorder.entries, 3)
	require.Equal(t, ErrorLevel, recorder.entries[0].Level)
	require.Equal(t, "network", testFieldValue(recorder.entries[0].Fields, "kind"))
	require.Equal(t, "value=x", recorder.entries[1].Message)
	require.Equal(t, DebugLevel, recorder.entries[1].Level)
	require.Equal(t, "ready", recorder.entries[2].Message)
	require.Equal(t, InfoLevel, recorder.entries[2].Level)
	require.Equal(t, 1, testFieldValue(recorder.entries[2].Fields, "id"))
}

func TestPackageDefaultLoggerSwap(t *testing.T) {
	prev := DefaultLogger()
	t.Cleanup(func() {
		SetLogger(prev)
	})

	recorder := &recordingLogger{level: InfoLevel}
	SetLogger(recorder)

	SetLogLevel(DebugLevel)
	require.Equal(t, DebugLevel, DefaultLogger().GetLevel())
	Infow("hello", "id", 7)
	require.Len(t, recorder.entries, 1)
	require.Equal(t, "hello", recorder.entries[0].Message)
	require.Equal(t, 7, testFieldValue(recorder.entries[0].Fields, "id"))
}

func TestFileOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	logger := NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "file",
		File: FileConfig{
			Path:       path,
			Encode:     "json",
			MaxSize:    1,
			MaxBackups: 1,
			MaxAge:     1,
		},
	})
	t.Cleanup(func() { require.NoError(t, logger.Close()) })

	logger.Log(context.Background(), Entry{
		Level:   InfoLevel,
		Message: "persisted",
		Fields:  []Field{{Key: "user", Value: "bob"}},
	})
	require.NoError(t, logger.Sync())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), `"message":"persisted"`)
	require.Contains(t, string(data), `"user":"bob"`)
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
	mu      sync.Mutex
	level   Level
	fields  []Field
	entries []Entry
}

func (l *recordingLogger) SetLevel(level Level) { l.level = level }
func (l *recordingLogger) GetLevel() Level      { return l.level }

func (l *recordingLogger) With(fields ...Field) Logger {
	return &recordingLogger{level: l.level, fields: slices.Concat(l.fields, fields)}
}

func (l *recordingLogger) Log(_ context.Context, entry Entry) {
	entry.Fields = slices.Concat(l.fields, entry.Fields)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (*recordingLogger) LevelHandler() http.Handler { return http.NotFoundHandler() }
func (*recordingLogger) Sync() error                { return nil }
func (*recordingLogger) Close() error               { return nil }

func testFieldValue(fields []Field, key string) any {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
}

func TestServiceWithLoggerKeepsContextAndTrace(t *testing.T) {
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), span)
	ctx = WithFields(ctx, Field{Key: "request_id", Value: "request-1"})
	first, second := &recordingLogger{}, &recordingLogger{}
	parent := NewService(first).WithContext(ctx)
	child := parent.WithLogger(second)
	parent.Infow("parent")
	child.Infow("child")

	require.NotSame(t, parent, child)
	for i, recorder := range []*recordingLogger{first, second} {
		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		require.Equal(t, []string{"parent", "child"}[i], entry.Message)
		require.Equal(t, "request-1", testFieldValue(entry.Fields, "request_id"))
		require.Equal(t, span.TraceID().String(), testFieldValue(entry.Fields, "trace_id"))
		require.Equal(t, span.SpanID().String(), testFieldValue(entry.Fields, "span_id"))
	}
}

func TestTraceLoggerOwnsFields(t *testing.T) {
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), span)
	fields := []Field{
		{Key: "trace_id", Value: "spoofed"},
		{Key: "span_id", Value: "spoofed"},
		{Key: "trace_id", Value: "duplicate"},
		{Key: "request_id", Value: "request-1"},
	}
	original := slices.Clone(fields)
	recorder := &recordingLogger{}
	logger := withTrace(recorder)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Log(ctx, Entry{Level: InfoLevel, Message: "handled", Fields: fields})
		}()
	}
	wg.Wait()

	require.Equal(t, original, fields)
	require.Len(t, recorder.entries, 16)
	for _, entry := range recorder.entries {
		require.ElementsMatch(t, []Field{
			{Key: "request_id", Value: "request-1"},
			{Key: "trace_id", Value: span.TraceID().String()},
			{Key: "span_id", Value: span.SpanID().String()},
		}, entry.Fields)
	}
}

func TestLoggerLevelHandler(t *testing.T) {
	logger := NewLoggerWith(&Config{Level: "info", Output: "console"})
	t.Cleanup(func() { require.NoError(t, logger.Close()) })
	child := logger.With(Field{Key: "component", Value: "worker"})
	request := httptest.NewRequest(http.MethodPut, "/log/level", strings.NewReader(`{"level":"debug"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	child.LevelHandler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, DebugLevel, logger.GetLevel())
	require.Equal(t, DebugLevel, child.GetLevel())
}

func TestPackageCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "caller.log")
	prev := DefaultLogger()
	t.Cleanup(func() {
		SetLogger(prev)
	})

	SetLogger(newTestFileLogger(t, path))

	line := logFromServerPackage()
	entry := readServerLogEntry(t, path)
	require.Equal(t, "package caller", entry["message"])
	require.Equal(t, "logs/logs_test.go:"+strconv.Itoa(line), entry["caller"])
}

func TestServiceCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service-caller.log")
	svc := NewService(newTestFileLogger(t, path))

	line := logFromServerService(svc)
	entry := readServerLogEntry(t, path)
	require.Equal(t, "service caller", entry["message"])
	require.Equal(t, "logs/logs_test.go:"+strconv.Itoa(line), entry["caller"])
}

func TestContextCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context-caller.log")
	previous := DefaultLogger()
	t.Cleanup(func() { SetLogger(previous) })
	SetLogger(newTestFileLogger(t, path))

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

func newTestFileLogger(t *testing.T, path string) Logger {
	t.Helper()
	logger := NewLoggerWith(&Config{
		Level:  "info",
		Encode: "json",
		Output: "file",
		File:   FileConfig{Path: path, Encode: "json"},
	})
	t.Cleanup(func() { require.NoError(t, logger.Close()) })
	return logger
}

func readServerLogEntry(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entry map[string]string
	require.NoError(t, json.Unmarshal(data, &entry))
	return entry
}
