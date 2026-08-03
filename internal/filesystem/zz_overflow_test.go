package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type ovHandler struct{ n atomic.Int32 }

func (h *ovHandler) OnBasePathAdded(string) {}
func (h *ovHandler) OnCreate(string)        { h.n.Add(1); time.Sleep(120 * time.Millisecond) }
func (h *ovHandler) OnUpdate(string)        {}
func (h *ovHandler) OnRemove(string)        {}
func (h *ovHandler) Filter(s string) bool   { return filepath.Ext(s) == ".yaml" }

// The test floods a watched directory with 2000 files while using a deliberately slow handler,
// which overflows the kernel's inotify queue and triggers an fsnotify error that nothing consumes.
// It then writes a single "canary" file and waits 8 seconds, if that file is never reported,
// the watcher has stopped delivering events permanently, and the test fails.
func TestOverflowKillsWatcher(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	h := &ovHandler{}
	w.Add(dir, h)
	stop := make(chan struct{})
	w.Start(stop)
	defer close(stop)
	time.Sleep(1500 * time.Millisecond)

	const burst = 2000
	for i := 0; i < burst; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("b-%04d.yaml", i)), []byte("x"), 0644)
	}
	time.Sleep(10 * time.Second)
	afterBurst := h.n.Load()
	t.Logf("phase 1: OnCreate fired %d / %d", afterBurst, burst)

	before := h.n.Load()
	os.WriteFile(filepath.Join(dir, "canary.yaml"), []byte("x"), 0644)
	time.Sleep(8 * time.Second)
	delta := h.n.Load() - before

	t.Logf("phase 2: canary file -> OnCreate delta = %d", delta)
	if delta == 0 {
		t.Errorf("WATCHER IS DEAD: canary create was never reported (phase1 delivered %d/%d)",
			afterBurst, burst)
	}
}
