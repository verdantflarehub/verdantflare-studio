// Package contentcache stores rebuildable, immutable bytes. Authorization is
// deliberately the caller's responsibility and must run before every lookup.
package contentcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrInvalid = errors.New("invalid cached content")

type call struct {
	done chan struct{}
	err  error
}
type Cache struct {
	Dir                             string
	Budget                          int64
	mu                              sync.Mutex
	disk                            sync.Mutex
	flights                         map[string]*call
	Hits, Misses, Writes, Evictions atomic.Uint64
}

func New(dir string, budget int64) *Cache {
	return &Cache{Dir: dir, Budget: budget, flights: map[string]*call{}}
}
func DefaultDir(name string) string {
	root, err := os.UserCacheDir()
	if err != nil {
		root = os.TempDir()
	}
	return filepath.Join(root, "verdantflare", name)
}
func (c *Cache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(c.Dir, hex.EncodeToString(h[:])+".cache")
}

// Each file starts with a digest of its payload. An incomplete, corrupt, or
// oversized file is a miss. Paths are hashes of keys, never client paths.
func (c *Cache) Lookup(key string, max int64) (*os.File, int64, bool) {
	path := c.path(key)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false
	}
	stat, err := f.Stat()
	if err != nil || stat.Size() < 32 || stat.Size()-32 > max {
		f.Close()
		return nil, 0, false
	}
	var digest [32]byte
	h := sha256.New()
	if _, err = io.ReadFull(f, digest[:]); err == nil {
		_, err = io.Copy(h, f)
	}
	if err != nil || hex.EncodeToString(h.Sum(nil)) != hex.EncodeToString(digest[:]) {
		f.Close()
		_ = os.Remove(path)
		return nil, 0, false
	}
	_, err = f.Seek(32, io.SeekStart)
	if err != nil {
		f.Close()
		return nil, 0, false
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	c.Hits.Add(1)
	return f, stat.Size() - 32, true
}
func (c *Cache) File(ctx context.Context, key string, max int64, load func(io.Writer) error) (*os.File, int64, error) {
	for {
		if f, size, ok := c.Lookup(key, max); ok {
			return f, size, nil
		}
		c.mu.Lock()
		if pending := c.flights[key]; pending != nil {
			c.mu.Unlock()
			select {
			case <-pending.done:
				if pending.err != nil {
					return nil, 0, pending.err
				}
				continue
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			}
		}
		// A completed writer may have published between the first lookup and
		// acquiring the flight lock. Recheck before becoming another writer.
		if f, size, ok := c.Lookup(key, max); ok {
			c.mu.Unlock()
			return f, size, nil
		}
		pending := &call{done: make(chan struct{})}
		c.flights[key] = pending
		c.mu.Unlock()
		c.Misses.Add(1)
		err := c.write(ctx, key, max, load)
		c.mu.Lock()
		pending.err = err
		delete(c.flights, key)
		close(pending.done)
		c.mu.Unlock()
		if err != nil {
			return nil, 0, err
		}
		if f, size, ok := c.Lookup(key, max); ok {
			return f, size, nil
		}
		return nil, 0, ErrInvalid
	}
}

type limitedWriter struct {
	target io.Writer
	left   int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.left {
		return 0, ErrInvalid
	}
	n, e := w.target.Write(p)
	w.left -= int64(n)
	return n, e
}
func (c *Cache) write(ctx context.Context, key string, max int64, load func(io.Writer) error) error {
	if max < 0 || c.Budget <= max {
		return ErrInvalid
	}
	if err := os.MkdirAll(c.Dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(c.Dir, ".partial-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(make([]byte, 32)); err != nil {
		return err
	}
	h := sha256.New()
	w := &limitedWriter{target: io.MultiWriter(f, h), left: max}
	if err = load(w); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = f.WriteAt(h.Sum(nil), 0); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	c.disk.Lock()
	defer c.disk.Unlock()
	if err = c.prune(stat.Size()); err != nil {
		return err
	}
	if err = os.Rename(name, c.path(key)); err != nil {
		return err
	}
	c.Writes.Add(1)
	slog.Info("immutable_content_cache", "event", "write", "layer", filepath.Base(c.Dir), "bytes", stat.Size()-32)
	return nil
}
func (c *Cache) prune(incoming int64) error {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return err
	}
	type item struct {
		path string
		size int64
		time time.Time
	}
	var items []item
	var total int64
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".partial-") {
			if stat, err := e.Info(); err == nil && time.Since(stat.ModTime()) > time.Hour {
				_ = os.Remove(filepath.Join(c.Dir, e.Name()))
			}
		}
		if filepath.Ext(e.Name()) != ".cache" {
			continue
		}
		s, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{filepath.Join(c.Dir, e.Name()), s.Size(), s.ModTime()})
		total += s.Size()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].time.Before(items[j].time) })
	for _, v := range items {
		if total+incoming <= c.Budget {
			break
		}
		if os.Remove(v.path) == nil {
			total -= v.size
			c.Evictions.Add(1)
		}
	}
	if total+incoming > c.Budget {
		return ErrInvalid
	}
	return nil
}
func (c *Cache) Get(ctx context.Context, key string, max int64, load func() ([]byte, error)) ([]byte, error) {
	f, size, err := c.File(ctx, key, max, func(w io.Writer) error {
		b, e := load()
		if e != nil {
			return e
		}
		_, e = w.Write(b)
		return e
	})
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.NewSectionReader(f, 32, size))
}
