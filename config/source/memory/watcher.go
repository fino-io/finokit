package memory

import (
	"sync"

	"github.com/fino-io/finokit/config/source"
)

type watcher struct {
	ID      string
	updates chan *source.ChangeSet
	exit    chan struct{}
	source  *memory
	once    sync.Once
}

func (w *watcher) Next() (*source.ChangeSet, error) {
	select {
	case cs := <-w.updates:
		return cs, nil
	case <-w.exit:
		return nil, source.ErrWatcherStopped
	}
}

func (w *watcher) Stop() error {
	w.once.Do(func() {
		close(w.exit)
		w.source.Lock()
		delete(w.source.Watchers, w.ID)
		w.source.Unlock()
	})

	return nil
}
