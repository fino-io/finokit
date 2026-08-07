// Package file is a file source. Expected format is json
package file

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"

	"github.com/fino-io/finokit/config/source"
)

type file struct {
	opts source.Options
	fs   fs.FS
	path string
}

var DefaultPath = "config.json"

const (
	defaultMaxConfigSizeBytes = 16 << 20 // 16MB
	maxConfigSizeEnvKey       = "fino_CONFIG_FILE_MAX_BYTES"
)

func (f *file) Read() (*source.ChangeSet, error) {
	var fh fs.File
	var err error

	if f.fs != nil {
		fh, err = f.fs.Open(f.path)
	} else {
		fh, err = os.Open(f.path)
	}

	if err != nil {
		return nil, err
	}
	defer func() {
		_ = fh.Close()
	}()

	info, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	maxSize := maxConfigSizeBytes()
	if info.Size() > maxSize {
		return nil, fmt.Errorf("config file %q exceeds max allowed size (%d bytes)", f.path, maxSize)
	}

	b, err := io.ReadAll(fh)
	if err != nil {
		return nil, err
	}

	cs := &source.ChangeSet{
		Format:    format(f.path, f.opts.Encoder),
		Source:    f.String(),
		Timestamp: info.ModTime(),
		Data:      b,
	}
	cs.Checksum = cs.Sum()

	return cs, nil
}

func (f *file) String() string {
	return "file"
}

func (f *file) Watch() (source.Watcher, error) {
	// do not watch if fs.FS instance is provided
	if f.fs != nil {
		return source.NewNoopWatcher()
	}

	if _, err := os.Stat(f.path); err != nil {
		return nil, err
	}
	return newWatcher(f)
}

func (f *file) Write(cs *source.ChangeSet) error {
	return nil
}

func NewSource(opts ...source.Option) source.Source {
	options := source.NewOptions(opts...)

	fs, _ := options.Context.Value(fsKey{}).(fs.FS)

	path := DefaultPath
	f, ok := options.Context.Value(filePathKey{}).(string)
	if ok {
		path = f
	}
	return &file{opts: options, fs: fs, path: path}
}

func maxConfigSizeBytes() int64 {
	raw := os.Getenv(maxConfigSizeEnvKey)
	if raw == "" {
		return defaultMaxConfigSizeBytes
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return defaultMaxConfigSizeBytes
	}

	return n
}
