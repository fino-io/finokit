// Package config is an interface for dynamic configuration.
package config

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fino-io/finokit/config/loader"
	"github.com/fino-io/finokit/config/reader"
	"github.com/fino-io/finokit/config/source"
	"github.com/fino-io/finokit/config/source/file"
	"github.com/fino-io/finokit/config/source/flag"
)

// Config is an interface abstraction for dynamic configuration.
type Config interface {
	// provide the reader.Values interface
	reader.Values
	// Init the config
	Init(opts ...Option) error
	// Options in the config
	Options() Options
	// Close Stop the config loader/watcher
	Close() error
	// Load config sources
	Load(source ...source.Source) error
	// Sync Force a source changeset sync
	Sync() error
	// Watch a value for changes
	Watch(path ...string) (Watcher, error)
}

// Watcher is the config watcher.
type Watcher interface {
	Next() (reader.Value, error)
	Stop() error
}

type Options struct {
	Loader loader.Loader
	Reader reader.Reader
	// for alternative data
	Context context.Context

	Source []source.Source

	WithWatcherDisabled bool
}

type Option func(o *Options)

// Default Config Manager.
var DefaultConfig, _ = NewConfig()
var defaultConfigInited atomic.Bool
var defaultConfigMu sync.RWMutex

var ErrDefaultConfigUninitialized = errors.New("default config is uninitialized, call InitDefault or Load* explicitly first")
var ErrScanTargetNilPointer = errors.New("scan target must be a non-nil pointer")

// NewConfig returns new config.
func NewConfig(opts ...Option) (Config, error) {
	return newConfig(opts...)
}

// InitDefault reinitializes the package-level default config manager.
// This is explicit initialization and must be called by applications
// that rely on package-level helpers.
func InitDefault(opts ...Option) error {
	cfg, err := NewConfig(opts...)
	if err != nil {
		return err
	}

	defaultConfigMu.Lock()
	defer defaultConfigMu.Unlock()

	if DefaultConfig != nil {
		_ = DefaultConfig.Close()
	}
	DefaultConfig = cfg
	defaultConfigInited.Store(true)

	return nil
}

func IsDefaultInitialized() bool {
	return defaultConfigInited.Load()
}

// Bytes Return config as raw json.
func Bytes() []byte {
	if !defaultConfigInited.Load() {
		return nil
	}
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Bytes()
}

// Map Return config as a map.
func Map() map[string]interface{} {
	if !defaultConfigInited.Load() {
		return nil
	}
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Map()
}

// Scan values to a go type.
func Scan(v interface{}) error {
	if !defaultConfigInited.Load() {
		return ErrDefaultConfigUninitialized
	}
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Scan(v)
}

// ScanFrom scan from the specifier keys to a go type
func ScanFrom(v interface{}, key string, alternatives ...string) error {
	if !defaultConfigInited.Load() {
		return ErrDefaultConfigUninitialized
	}

	if v == nil {
		return ErrScanTargetNilPointer
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return ErrScanTargetNilPointer
	}

	keys := append([]string{key}, alternatives...)
	for i, k := range keys {
		val, err := Get(k)
		if err != nil {
			return err
		}

		if val.Null() && i < len(keys)-1 {
			continue
		}

		if err := val.Scan(v); err != nil {
			return fmt.Errorf("scan config key %q into %T: %w", k, v, err)
		}
		return nil
	}

	return nil
}

// Sync Force a source changeset sync.
func Sync() error {
	if !defaultConfigInited.Load() {
		return ErrDefaultConfigUninitialized
	}
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Sync()
}

// Get a value from the config.
func Get(path ...string) (reader.Value, error) {
	if !defaultConfigInited.Load() {
		return nil, ErrDefaultConfigUninitialized
	}
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Get(normalizePath(path...)...)
}

// Load config sources.
func Load(source ...source.Source) error {
	defaultConfigInited.Store(true)
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Load(source...)
}

// Watch a value for changes.
func Watch(path ...string) (Watcher, error) {
	if !defaultConfigInited.Load() {
		return nil, ErrDefaultConfigUninitialized
	}
	defaultConfigMu.RLock()
	defer defaultConfigMu.RUnlock()
	return DefaultConfig.Watch(normalizePath(path...)...)
}

// LoadFile is short hand for creating a file source and loading it.
func LoadFile(path string) error {
	return Load(file.NewSource(
		file.WithPath(path),
	))
}

func LoadPath(path string) error {
	return Load(newFileSources(path, "")...)
}

func LoadPathWithSuffix(path string, suffix string) error {
	return Load(newFileSources(path, suffix)...)
}

// LoadFlag load command-line parameters
func LoadFlag() error {
	return Load(flag.NewSource())
}

func normalizePath(path ...string) []string {
	var segments []string
	for _, p := range path {
		s := strings.Split(p, ".")
		segments = append(segments, s...)
	}
	return segments
}
