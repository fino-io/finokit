package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/fino-io/finokit/config/reader"
	"github.com/fino-io/finokit/config/source"
	"github.com/fino-io/finokit/config/source/file"
	"github.com/joho/godotenv"
)

var supportedFileSuffixes = map[string]bool{
	"json": true,
	// "toml": true,
	"xml":  true,
	"yaml": true,
}

// LoadDefaultSources loads .env and config sources from the working directory.
func LoadDefaultSources() error {
	sources, err := defaultSources()
	if err != nil {
		return err
	}
	return Load(sources...)
}

func defaultSources() ([]source.Source, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	dirs := []string{
		filepath.Join(workDir, "conf"),
		filepath.Join(workDir, "config"),
		filepath.Join(workDir, "configs"),
	}

	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		dirs = append([]string{configPath}, dirs...)
	}

	if strings.Contains(workDir, "/cmd/") || strings.HasSuffix(workDir, "/cmd") {
		dirs = append(dirs, "../configs", "../../configs")
	}

	var sources []source.Source
	env := os.Getenv("DEPLOY_ENV")
	for _, dir := range dirs {
		if sources = newFileSources(dir, env); len(sources) > 0 {
			break
		}
	}

	return sources, nil
}

func newFileSources(dir string, env string) []source.Source {
	var sources []source.Source

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, entry := range files {
		if entry.IsDir() {
			sources = append(sources, newFileSources(filepath.Join(dir, entry.Name()), env)...)
			continue
		}

		ext := filepath.Ext(entry.Name())
		if !supportedFileSuffixes[strings.TrimPrefix(strings.ToLower(ext), ".")] {
			continue
		}
		if env != "" && !strings.HasSuffix(strings.TrimSuffix(entry.Name(), ext), env) {
			continue
		}
		sources = append(sources, file.NewSource(file.WithPath(filepath.Join(dir, entry.Name()))))
	}

	return sources
}

type watchCloser struct {
	exit chan struct{}
	once sync.Once
}

func (w *watchCloser) Close() error {
	w.once.Do(func() {
		close(w.exit)
	})
	return nil
}

func WatchFunc(handle func(reader.Value), paths ...string) (io.Closer, error) {
	_path := make([]string, 0, len(paths))
	for _, v := range paths {
		_path = append(_path, strings.Split(v, ".")...)
	}

	exit := make(chan struct{})
	w, err := Watch(_path...)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			v, err := w.Next()
			if err != nil {
				if errors.Is(err, source.ErrWatcherStopped) {
					return
				}
				continue
			}

			if handle != nil {
				handle(v)
			}
		}
	}()

	go func() {
		<-exit
		_ = w.Stop()
	}()

	return &watchCloser{exit: exit}, nil
}
