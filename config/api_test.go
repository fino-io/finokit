package config

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	loaderpkg "github.com/fino-io/finokit/config/loader"
	loadermemory "github.com/fino-io/finokit/config/loader/memory"
	readerpkg "github.com/fino-io/finokit/config/reader"
	readerjson "github.com/fino-io/finokit/config/reader/json"
	"github.com/fino-io/finokit/config/source"
	sourcememory "github.com/fino-io/finokit/config/source/memory"
)

func isolateDefaultConfig(t *testing.T) {
	t.Helper()

	prev := DefaultConfig
	prevInited := defaultConfigInited.Load()

	t.Cleanup(func() {
		if DefaultConfig != nil && DefaultConfig != prev {
			_ = DefaultConfig.Close()
		}
		DefaultConfig = prev
		defaultConfigInited.Store(prevInited)
	})
}

func TestPackageAPIRequiresInitialization(t *testing.T) {
	isolateDefaultConfig(t)
	defaultConfigInited.Store(false)

	require.Nil(t, Bytes())
	require.Nil(t, Map())

	var target map[string]any
	require.ErrorIs(t, Scan(&target), ErrDefaultConfigUninitialized)
	require.ErrorIs(t, ScanFrom(&target, "x"), ErrDefaultConfigUninitialized)
	require.ErrorIs(t, Sync(), ErrDefaultConfigUninitialized)

	v, err := Get("x")
	require.Nil(t, v)
	require.ErrorIs(t, err, ErrDefaultConfigUninitialized)

	w, err := Watch("x")
	require.Nil(t, w)
	require.ErrorIs(t, err, ErrDefaultConfigUninitialized)
}

func TestInitDefaultAndGlobalAccessors(t *testing.T) {
	isolateDefaultConfig(t)

	err := InitDefault(
		WithWatcherDisabled(),
		WithSource(sourcememory.NewSource(sourcememory.WithJSON([]byte(`{
			"service": {"name": "fino"},
			"backup": {"name": "fallback"}
		}`)))),
	)
	require.NoError(t, err)
	require.True(t, IsDefaultInitialized())

	v1, err := Get("service.name")
	require.NoError(t, err)
	require.Equal(t, "fino", v1.String(""))

	v2, err := Get("service", "name")
	require.NoError(t, err)
	require.Equal(t, "fino", v2.String(""))

	var picked string
	err = ScanFrom(&picked, "service.missing", "backup.name", "service.name")
	require.NoError(t, err)
	require.Equal(t, "fallback", picked)

	var snapshot map[string]map[string]string
	require.NoError(t, Scan(&snapshot))
	require.Equal(t, "fino", snapshot["service"]["name"])

	require.NoError(t, Sync())
	require.NotEmpty(t, Bytes())
	require.Contains(t, Map(), "service")
}

func TestScanFromParsesDurationFields(t *testing.T) {
	isolateDefaultConfig(t)

	err := InitDefault(
		WithWatcherDisabled(),
		WithSource(sourcememory.NewSource(sourcememory.WithJSON([]byte(`{
			"server": {"ttl": "10m"}
		}`)))),
	)
	require.NoError(t, err)

	var cfg struct {
		TTL time.Duration `json:"ttl"`
	}
	require.NoError(t, ScanFrom(&cfg, "server"))
	require.Equal(t, 10*time.Minute, cfg.TTL)
}

func TestScanFromReturnsDurationParseError(t *testing.T) {
	isolateDefaultConfig(t)

	err := InitDefault(
		WithWatcherDisabled(),
		WithSource(sourcememory.NewSource(sourcememory.WithJSON([]byte(`{
			"server": {"ttl": "invalid"}
		}`)))),
	)
	require.NoError(t, err)

	var ttl time.Duration
	err = ScanFrom(&ttl, "server.ttl")
	require.Error(t, err)
	require.ErrorContains(t, err, `scan config key "server.ttl" into *time.Duration`)
	require.ErrorContains(t, err, "invalid duration")
}

func TestScanFromRequiresPointerTarget(t *testing.T) {
	isolateDefaultConfig(t)
	require.NoError(t, InitDefault(WithWatcherDisabled()))

	require.EqualError(t, ScanFrom(nil, "x"), "scan target must be a non-nil pointer")
	require.EqualError(t, ScanFrom(struct{}{}, "x"), "scan target must be a non-nil pointer")
}

func TestLoadHelpers(t *testing.T) {
	t.Run("LoadFile", func(t *testing.T) {
		isolateDefaultConfig(t)
		require.NoError(t, InitDefault(WithWatcherDisabled()))

		path := writeTempConfigFile(t, `{"from":"file"}`)
		require.NoError(t, LoadFile(path))

		v, err := Get("from")
		require.NoError(t, err)
		require.Equal(t, "file", v.String(""))
	})

	t.Run("LoadPath", func(t *testing.T) {
		isolateDefaultConfig(t)
		require.NoError(t, InitDefault(WithWatcherDisabled()))

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.yaml"), "app:\n  name: fino\n")
		writeFile(t, filepath.Join(dir, "nested", "b.json"), `{"db":{"port":5432}}`)
		writeFile(t, filepath.Join(dir, "ignored.txt"), "ignore")

		require.NoError(t, LoadPath(dir))

		appName, err := Get("app.name")
		require.NoError(t, err)
		require.Equal(t, "fino", appName.String(""))

		dbPort, err := Get("db.port")
		require.NoError(t, err)
		require.Equal(t, 5432, dbPort.Int(0))
	})

	t.Run("LoadPathWithSuffix", func(t *testing.T) {
		isolateDefaultConfig(t)
		require.NoError(t, InitDefault(WithWatcherDisabled()))

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "app.dev.yaml"), "env: dev\n")
		writeFile(t, filepath.Join(dir, "app.prod.yaml"), "env: prod\n")

		require.NoError(t, LoadPathWithSuffix(dir, "dev"))

		envVal, err := Get("env")
		require.NoError(t, err)
		require.Equal(t, "dev", envVal.String(""))
	})

	t.Run("LoadFlag", func(t *testing.T) {
		isolateDefaultConfig(t)
		if !flag.Parsed() {
			require.NoError(t, flag.CommandLine.Parse([]string{}))
		}

		require.NoError(t, InitDefault(WithWatcherDisabled()))
		require.NoError(t, LoadFlag())
	})
}

func TestNormalizePath(t *testing.T) {
	require.Equal(t, []string{"a", "b", "c", "d"}, normalizePath("a.b", "c", "d"))
	require.Nil(t, normalizePath())
}

func TestOptionsHelpers(t *testing.T) {
	var opts Options

	loader := loadermemory.NewLoader()
	reader := readerjson.NewReader()
	src := sourcememory.NewSource(sourcememory.WithJSON([]byte(`{"ok":true}`)))

	WithLoader(loader)(&opts)
	WithReader(reader)(&opts)
	WithSource(src)(&opts)
	WithWatcherDisabled()(&opts)

	require.Equal(t, loader, opts.Loader)
	require.Equal(t, reader, opts.Reader)
	require.Len(t, opts.Source, 1)
	require.True(t, opts.WithWatcherDisabled)
}

func TestDefaultConfigProvider(t *testing.T) {
	isolateDefaultConfig(t)
	require.NoError(t, InitDefault(
		WithWatcherDisabled(),
		WithSource(sourcememory.NewSource(sourcememory.WithJSON([]byte(`{"project":{"name":"fino"}}`)))),
	))

	p := NewDefaultConfigProvider()

	gotAny, err := p.Get("project.name")
	require.NoError(t, err)
	gotVal, ok := gotAny.(readerpkg.Value)
	require.True(t, ok)
	require.Equal(t, "fino", gotVal.String(""))

	var data struct {
		Project struct {
			Name string `json:"name"`
		} `json:"project"`
	}
	require.NoError(t, p.Scan(&data))
	require.Equal(t, "fino", data.Project.Name)

	var name string
	require.NoError(t, p.ScanFrom(&name, "project.missing", "project.name"))
	require.Equal(t, "fino", name)
}

func TestValueDefaults(t *testing.T) {
	v := newValue()

	require.True(t, v.Null())
	require.False(t, v.Bool(true))
	require.Zero(t, v.Int(123))
	require.Equal(t, "", v.String("fallback"))
	require.Zero(t, v.Float64(1.2))
	require.Zero(t, v.Duration(time.Second))
	require.Nil(t, v.StringSlice([]string{"x"}))
	require.Equal(t, map[string]string{}, v.StringMap(map[string]string{"k": "v"}))
	require.NoError(t, v.Scan(new(struct{})))
	require.Nil(t, v.Bytes())
}

func TestConfigMethodsWithNilValues(t *testing.T) {
	c := &config{}

	require.Nil(t, c.Map())
	require.NoError(t, c.Scan(new(any)))

	got, err := c.Get("missing")
	require.NoError(t, err)
	require.True(t, got.Null())

	c.Set("v", "k")
	c.Del("k")

	require.Equal(t, []byte{}, c.Bytes())
	require.Equal(t, "config", c.String())
}

func TestWatcherNextSkipsUnchangedValue(t *testing.T) {
	lw := &stubLoaderWatcher{
		events: []stubWatchEvent{
			{snap: &loaderpkg.Snapshot{ChangeSet: &source.ChangeSet{Data: []byte("same")}}},
			{snap: &loaderpkg.Snapshot{ChangeSet: &source.ChangeSet{Data: []byte("next")}}},
		},
	}

	w := &watcher{
		lw:    lw,
		rd:    &stubReader{},
		value: &stubValue{raw: []byte("same")},
	}

	got, err := w.Next()
	require.NoError(t, err)
	require.Equal(t, "next", got.String(""))

	require.NoError(t, w.Stop())
	require.True(t, lw.stopped)
}

func TestWatcherNextReturnsLoaderError(t *testing.T) {
	boom := errors.New("boom")
	w := &watcher{
		lw: &stubLoaderWatcher{
			events: []stubWatchEvent{{err: boom}},
		},
		rd:    &stubReader{},
		value: &stubValue{raw: []byte("same")},
	}

	_, err := w.Next()
	require.ErrorIs(t, err, boom)
}

func TestSnapshotNewerVersion(t *testing.T) {
	cases := []struct {
		next    string
		current string
		expect  bool
	}{
		{next: "2", current: "1", expect: true},
		{next: "1", current: "2", expect: false},
		{next: "a2", current: "a1", expect: true},
		{next: "a1", current: "a2", expect: false},
	}

	for _, tc := range cases {
		require.Equal(t, tc.expect, snapshotNewerVersion(tc.next, tc.current))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

type stubWatchEvent struct {
	snap *loaderpkg.Snapshot
	err  error
}

type stubLoaderWatcher struct {
	events  []stubWatchEvent
	idx     int
	stopped bool
}

func (w *stubLoaderWatcher) Next() (*loaderpkg.Snapshot, error) {
	if w.idx >= len(w.events) {
		return nil, io.EOF
	}

	ev := w.events[w.idx]
	w.idx++
	return ev.snap, ev.err
}

func (w *stubLoaderWatcher) Stop() error {
	w.stopped = true
	return nil
}

type stubReader struct{}

func (r *stubReader) Merge(ch ...*source.ChangeSet) (*source.ChangeSet, error) {
	if len(ch) == 0 {
		return &source.ChangeSet{}, nil
	}
	return ch[len(ch)-1], nil
}

func (r *stubReader) Values(ch *source.ChangeSet) (readerpkg.Values, error) {
	data := []byte(nil)
	if ch != nil {
		data = append([]byte(nil), ch.Data...)
	}
	return &stubValues{raw: data}, nil
}

func (r *stubReader) String() string {
	return "stub"
}

type stubValues struct {
	raw []byte
}

func (v *stubValues) Bytes() []byte {
	return append([]byte(nil), v.raw...)
}

func (v *stubValues) Get(path ...string) (readerpkg.Value, error) {
	return &stubValue{raw: append([]byte(nil), v.raw...)}, nil
}

func (v *stubValues) Set(val interface{}, path ...string) {}

func (v *stubValues) Del(path ...string) {}

func (v *stubValues) Map() map[string]interface{} {
	return map[string]interface{}{}
}

func (v *stubValues) Scan(any) error {
	return nil
}

type stubValue struct {
	raw []byte
}

func (v *stubValue) Null() bool {
	return len(v.raw) == 0
}

func (v *stubValue) Bool(def bool) bool {
	return def
}

func (v *stubValue) Int(def int) int {
	return def
}

func (v *stubValue) String(def string) string {
	if len(v.raw) == 0 {
		return def
	}
	return string(v.raw)
}

func (v *stubValue) Float64(def float64) float64 {
	return def
}

func (v *stubValue) Duration(def time.Duration) time.Duration {
	return def
}

func (v *stubValue) StringSlice(def []string) []string {
	return def
}

func (v *stubValue) StringMap(def map[string]string) map[string]string {
	return def
}

func (v *stubValue) Scan(any) error {
	return nil
}

func (v *stubValue) Bytes() []byte {
	return append([]byte(nil), v.raw...)
}
