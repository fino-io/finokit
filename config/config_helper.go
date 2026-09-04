package config

import (
	"errors"
	"fmt"
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

// LoadServiceConfig loads configuration for one service from the working
// directory. It prefers a service-specific directory and also supports a
// directory that is already mounted as that service's config root.
func LoadServiceConfig(service string) error {
	sources, err := serviceSources(service)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("no config sources found for service %q", service)
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

	dirs := configRoots(workDir)

	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		dirs = append([]string{configPath}, dirs...)
	}

	return firstFileSources(dirs, os.Getenv("DEPLOY_ENV"), true), nil
}

func serviceSources(service string) ([]source.Source, error) {
	service = strings.TrimSpace(service)
	if service == "" || service == "." || service == ".." ||
		filepath.IsAbs(service) || filepath.Base(service) != service {
		return nil, errors.New("service name must be a single path segment")
	}

	workDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	env := os.Getenv("DEPLOY_ENV")
	roots := configRoots(workDir)
	serviceDirs := make([]string, 0, len(roots)+1)
	directDirs := make([]string, 0, len(roots)+1)
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		serviceDirs = append(serviceDirs, filepath.Join(configPath, service))
		directDirs = append(directDirs, configPath)
	}

	for _, root := range roots {
		serviceDirs = append(serviceDirs, filepath.Join(root, service))
		directDirs = append(directDirs, root)
	}

	if sources := firstFileSources(serviceDirs, env, true); len(sources) > 0 {
		return sources, nil
	}

	return firstFileSources(directDirs, env, false), nil
}

func configRoots(workDir string) []string {
	roots := []string{
		filepath.Join(workDir, "conf"),
		filepath.Join(workDir, "config"),
		filepath.Join(workDir, "configs"),
	}
	if strings.Contains(workDir, "/cmd/") || strings.HasSuffix(workDir, "/cmd") {
		roots = append(roots,
			filepath.Join(workDir, "..", "configs"),
			filepath.Join(workDir, "..", "..", "configs"),
		)
	}
	return roots
}

func firstFileSources(dirs []string, env string, recursive bool) []source.Source {
	for _, dir := range dirs {
		if sources := collectFileSources(dir, env, recursive); len(sources) > 0 {
			return sources
		}
	}
	return nil
}

func newFileSources(dir string, env string) []source.Source {
	return collectFileSources(dir, env, true)
}

func collectFileSources(dir string, env string, recursive bool) []source.Source {
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
			if recursive {
				sources = append(sources, collectFileSources(filepath.Join(dir, entry.Name()), env, true)...)
			}
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
