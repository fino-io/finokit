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
)

var supportedFileSuffixes = map[string]bool{
	"json": true,
	// "toml": true,
	"xml":  true,
	"yaml": true,
}

// LoadDefaultSources discovers and loads config sources from conventional
// directories. This must be called explicitly by applications.
func LoadDefaultSources() error {
	return Load(defaultSources()...)
}

func defaultSources() []source.Source {
	workDir, _ := os.Getwd()
	dirs := []string{
		filepath.Join(workDir, "conf"),
		filepath.Join(workDir, "config"),
		filepath.Join(workDir, "configs"),
	}

	if configPath := os.Getenv("CONFIG_PATH"); len(configPath) > 0 {
		dirs = append([]string{configPath}, dirs...)
	}

	if strings.Contains(workDir, "/cmd/") || strings.HasSuffix(workDir, "/cmd") {
		dirs = append(dirs, []string{"../configs", "../../configs"}...)
	}

	var sources []source.Source
	for _, dir := range dirs {
		if sources = newFileSources(dir, os.Getenv("DEPLOY_ENV")); len(sources) > 0 {
			break
		}
	}

	// sources = append(sources, env.NewSource())
	return sources
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

	for _, f := range files {
		if f.IsDir() {
			ss := newFileSources(filepath.Join(dir, f.Name()), env)
			sources = append(sources, ss...)
		} else {
			segments := strings.Split(f.Name(), ".")
			suffix := strings.TrimPrefix(strings.ToLower(filepath.Ext(f.Name())), ".")
			if !supportedFileSuffixes[suffix] {
				continue
			}
			p := filepath.Join(dir, f.Name())
			if len(env) > 0 {
				name := strings.Join(segments[:len(segments)-1], ".")
				if strings.HasSuffix(name, env) {
					sources = append(sources, file.NewSource(file.WithPath(p)))
				}
			} else {
				sources = append(sources, file.NewSource(file.WithPath(p)))
			}
		}
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
