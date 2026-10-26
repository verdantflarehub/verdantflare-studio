package contentcache

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheCoalescesColdReadsPersistsAndRecoversCorruption(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 4096)
	var loads atomic.Int32
	load := func() ([]byte, error) { loads.Add(1); return []byte("fixed bytes"), nil }
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, e := c.Get(context.Background(), "org/ref/original-v1", 128, load)
			if e != nil || string(b) != "fixed bytes" {
				t.Errorf("bad cache result: %v", e)
			}
		}()
	}
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatalf("source loaded %d times", loads.Load())
	}
	restarted := New(dir, 4096)
	if _, e := restarted.Get(context.Background(), "org/ref/original-v1", 128, load); e != nil || loads.Load() != 1 {
		t.Fatal("restart lost cache", e)
	}
	if e := os.WriteFile(c.path("org/ref/original-v1"), bytes.Repeat([]byte{0}, 64), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := restarted.Get(context.Background(), "org/ref/original-v1", 128, load); e != nil || string(b) != "fixed bytes" || loads.Load() != 2 {
		t.Fatal("corrupt bytes escaped or were not rebuilt", e)
	}
	if _, e := c.Get(context.Background(), "other-org/ref/original-v1", 128, load); e != nil || loads.Load() != 3 {
		t.Fatal("organization cache key collided", e)
	}
}
func TestCacheFailedPartialAndOversizeAreNeverPublished(t *testing.T) {
	c := New(t.TempDir(), 1024)
	for _, load := range []func(io.Writer) error{
		func(w io.Writer) error { _, _ = w.Write([]byte("partial")); return errors.New("upstream failed") },
		func(w io.Writer) error { _, e := w.Write(bytes.Repeat([]byte{1}, 65)); return e },
	} {
		if f, _, e := c.File(context.Background(), "failed", 64, load); e == nil {
			f.Close()
			t.Fatal("failed load was published")
		}
		if f, _, ok := c.Lookup("failed", 64); ok {
			f.Close()
			t.Fatal("partial was visible")
		}
	}
	if c.Writes.Load() != 0 {
		t.Fatal("failed writes counted as successes")
	}
}
func TestCacheBudgetEvictionAndCancelledWait(t *testing.T) {
	c := New(t.TempDir(), 220)
	for _, key := range []string{"one", "two", "three"} {
		if _, e := c.Get(context.Background(), key, 64, func() ([]byte, error) { return bytes.Repeat([]byte(key[:1]), 64), nil }); e != nil {
			t.Fatal(e)
		}
	}
	if c.Evictions.Load() == 0 {
		t.Fatal("budget did not evict")
	}
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Get(context.Background(), "waiting", 64, func() ([]byte, error) { close(started); <-release; return []byte("ready"), nil })
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Get(ctx, "waiting", 64, func() ([]byte, error) { t.Error("duplicate loader"); return nil, nil }); !errors.Is(e, context.Canceled) {
		t.Fatal("wait did not cancel", e)
	}
	close(release)
	<-done
}

func TestCacheReclaimsOnlyAbandonedPartialFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, ".partial-crashed")
	live := filepath.Join(dir, ".partial-live")
	if err := os.WriteFile(old, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("writing"), 0600); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-2 * time.Hour)
	os.Chtimes(old, before, before)
	c := New(dir, 4096)
	if _, err := c.Get(context.Background(), "new", 128, func() ([]byte, error) { return []byte("ready"), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old partial survived", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("active writer was removed", err)
	}
}
