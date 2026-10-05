package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

// Check existing names too: a Linux copy must not create aliases that collide
// when moved to Windows. os.Root separately constrains all actual operations.
func (w *Workspace) safe(name string, parents bool) error {
	parts := strings.Split(name, "/")
	dir := "."
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ErrInvalid
		}
		f, e := w.root.Open(dir)
		if e != nil {
			return e
		}
		entries, e := f.ReadDir(-1)
		f.Close()
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if project.PathKey(entry.Name()) == project.PathKey(part) && entry.Name() != part {
				return ErrConflict
			}
		}
		next := path.Join(dir, part)
		info, e := w.root.Lstat(next)
		if errors.Is(e, os.ErrNotExist) {
			if i == len(parts)-1 {
				return nil
			}
			if !parents {
				return os.ErrNotExist
			}
			if e = w.root.Mkdir(next, 0700); e != nil {
				return e
			}
			info, e = w.root.Lstat(next)
		}
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return ErrConflict
		}
		dir = next
	}
	return nil
}

func (w *Workspace) read(name string, limit int64) ([]byte, error) {
	if e := w.safe(name, false); e != nil {
		return nil, e
	}
	f, e := w.root.Open(name)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, ErrInvalid
	}
	return b, nil
}

func (w *Workspace) decode(name string, out any) error {
	b, e := w.read(name, project.MaxManifestBytes)
	if e != nil {
		return e
	}
	if strictjson.Decode(bytes.NewReader(b), project.MaxManifestBytes, out) != nil {
		return ErrInvalid
	}
	return nil
}

func (w *Workspace) temporary() (string, *os.File, error) {
	name := ".vf/tmp/" + uuid.Must(uuid.NewV7()).String()
	if e := w.safe(name, false); e != nil {
		return "", nil, e
	}
	f, e := w.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	return name, f, e
}

func (w *Workspace) write(name string, b []byte, replace bool) error {
	if e := w.safe(name, false); e != nil {
		return e
	}
	tmp, f, e := w.temporary()
	if e != nil {
		return e
	}
	defer w.root.Remove(tmp)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = w.safe(name, false); e != nil {
		return e
	}
	if replace {
		e = w.root.Rename(tmp, name)
	} else {
		e = w.root.Link(tmp, name)
	}
	if e != nil {
		return e
	}
	return syncDirectory(w.root, path.Dir(name))
}

func (w *Workspace) writeJSON(name string, value any, replace bool) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil || len(b) > project.MaxManifestBytes {
		return ErrInvalid
	}
	return w.write(name, append(b, '\n'), replace)
}

func manifestEqual(a, b project.Manifest) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (w *Workspace) base(revision string) (project.Manifest, error) {
	if !project.ValidID(revision) {
		return project.Manifest{}, ErrInvalid
	}
	b, e := w.read(".vf/bases/"+revision+".json", project.MaxManifestBytes)
	if e != nil {
		return project.Manifest{}, e
	}
	m, e := project.Decode(b)
	if e != nil || validateManifest(m) != nil {
		return project.Manifest{}, ErrInvalid
	}
	return m, nil
}

func (w *Workspace) storeBase(revision string, m project.Manifest) error {
	if !project.ValidID(revision) || validateManifest(m) != nil {
		return ErrInvalid
	}
	old, e := w.base(revision)
	if e == nil {
		if !manifestEqual(old, m) {
			return ErrCorrupt
		}
		return nil
	}
	if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return w.writeJSON(".vf/bases/"+revision+".json", m, false)
}

func (w *Workspace) checksum(name string, max int64) (string, int64, error) {
	if e := w.safe(name, false); e != nil {
		return "", 0, e
	}
	f, e := w.root.Open(name)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return "", 0, ErrConflict
	}
	if info.Size() > max {
		return "", info.Size(), nil
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, max+1))
	if e != nil {
		return "", n, e
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
