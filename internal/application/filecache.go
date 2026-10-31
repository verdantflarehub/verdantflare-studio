package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/verdantflarehub/verdantflare-studio/internal/navigation"
)

// fileSignature identifies registry contents by metadata only. Kubernetes
// ConfigMap updates swap the mounted symlink target, which changes both the
// file and directory modification times, so cached reads follow releases
// without reading bytes on every request.
func fileSignature(paths ...string) string {
	var b strings.Builder
	for _, path := range paths {
		if path == "" {
			b.WriteString("-|")
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			b.WriteString("!|")
			continue
		}
		fmt.Fprintf(&b, "%d:%d:%t|", info.Size(), info.ModTime().UnixNano(), info.IsDir())
	}
	return b.String()
}

// fileCached keeps the last successful load until a watched path changes.
// Failures are never retained. Cached values are shared and read-only.
type fileCached[T any] struct {
	paths []string
	mu    sync.Mutex
	sig   string
	value T
	ok    bool
}

func (c *fileCached[T]) get(load func() (T, error)) (T, error) {
	sig := fileSignature(c.paths...)
	c.mu.Lock()
	if c.ok && c.sig == sig {
		v := c.value
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()
	v, err := load()
	if err == nil {
		c.mu.Lock()
		c.sig, c.value, c.ok = sig, v, true
		c.mu.Unlock()
	}
	return v, err
}

// cachedCatalogSource serves the local navigation registry from memory until
// the registry or the Dashboard registry it consults changes.
type cachedCatalogSource struct {
	inner navigation.Source
	cache fileCached[navigation.Catalog]
}

func (s *cachedCatalogSource) Load(ctx context.Context) (navigation.Catalog, error) {
	return s.cache.get(func() (navigation.Catalog, error) { return s.inner.Load(ctx) })
}

func cachedArtifacts(load func() (map[string]string, error), path string) func() (map[string]string, error) {
	cache := &fileCached[map[string]string]{paths: []string{path}}
	return func() (map[string]string, error) { return cache.get(load) }
}

func dirOf(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}
