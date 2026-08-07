package memory

import (
	"testing"
	"time"

	"github.com/fino-io/finokit/config/source"
)

func TestMemoryRead_EmptySource(t *testing.T) {
	src := NewSource()
	cs, err := src.Read()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if cs == nil {
		t.Fatal("changeset is nil")
	}
}

func TestMemoryWatcher_StopUnblocksNext(t *testing.T) {
	src := NewSource(WithJSON([]byte(`{"foo":"bar"}`)))
	w, err := src.Watch()
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, nextErr := w.Next()
		result <- nextErr
	}()

	time.Sleep(20 * time.Millisecond)
	if err = w.Stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	select {
	case nextErr := <-result:
		if nextErr != source.ErrWatcherStopped {
			t.Fatalf("expected %v, got %v", source.ErrWatcherStopped, nextErr)
		}
	case <-time.After(time.Second):
		t.Fatal("watcher.Next did not unblock after Stop")
	}
}
