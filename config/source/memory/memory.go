// Package memory is a memory source
package memory

import (
	"bytes"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/fino-io/finokit/config/source"
)

type memory struct {
	ChangeSet *source.ChangeSet
	Watchers  map[string]*watcher
	sync.RWMutex
}

func (m *memory) Read() (*source.ChangeSet, error) {
	m.RLock()
	defer m.RUnlock()
	if m.ChangeSet == nil {
		return &source.ChangeSet{Source: m.String()}, nil
	}

	cs := &source.ChangeSet{
		Format:    m.ChangeSet.Format,
		Timestamp: m.ChangeSet.Timestamp,
		Data:      bytes.Clone(m.ChangeSet.Data),
		Checksum:  m.ChangeSet.Checksum,
		Source:    m.ChangeSet.Source,
	}
	return cs, nil
}

func (m *memory) Watch() (source.Watcher, error) {
	w := &watcher{
		ID:      uuid.New().String(),
		updates: make(chan *source.ChangeSet, 100),
		exit:    make(chan struct{}),
		source:  m,
	}

	m.Lock()
	m.Watchers[w.ID] = w
	m.Unlock()
	return w, nil
}

func (m *memory) Write(cs *source.ChangeSet) error {
	m.Update(cs)
	return nil
}

// Update allows manual updates of the config data.
func (m *memory) Update(c *source.ChangeSet) {
	// don't process nil
	if c == nil {
		return
	}

	// hash the file
	m.Lock()
	// update changeset
	m.ChangeSet = &source.ChangeSet{
		Data:      bytes.Clone(c.Data),
		Format:    c.Format,
		Source:    "memory",
		Timestamp: time.Now(),
	}
	m.ChangeSet.Checksum = m.ChangeSet.Sum()

	// update watchers
	for _, w := range m.Watchers {
		select {
		case w.updates <- m.ChangeSet:
		default:
		}
	}
	m.Unlock()
}

func (m *memory) String() string {
	return "memory"
}

func NewSource(opts ...source.Option) source.Source {
	options := source.NewOptions(opts...)

	s := &memory{
		Watchers: map[string]*watcher{},
		ChangeSet: &source.ChangeSet{
			Format:    options.Encoder.String(),
			Source:    "memory",
			Timestamp: time.Now(),
		},
	}

	c, ok := options.Context.Value(changeSetKey{}).(*source.ChangeSet)
	if ok {
		s.Update(c)
	}

	return s
}
