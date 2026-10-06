package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"sync"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type Workspace struct {
	root     *os.Root
	lock     *os.File
	remote   Remote
	state    State
	manifest project.Manifest
	mu       sync.Mutex
}

// Snapshot returns an isolated copy for the host UI/agent. Mutating the returned
// values cannot edit the pinned baseline or local materialization state.
func (w *Workspace) Snapshot(ctx context.Context) (State, project.Manifest, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.authorize(ctx); e != nil {
		return State{}, project.Manifest{}, e
	}
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return State{}, project.Manifest{}, e
	}
	data, e := json.Marshal(w.manifest)
	if e != nil {
		return State{}, project.Manifest{}, e
	}
	m, e := project.Decode(data)
	state := w.state
	state.Materialized = append([]string{}, state.Materialized...)
	return state, m, e
}

// Open opens a user-selected existing directory. A new copy starts at the
// current service head; an existing copy stays pinned to its saved base.
func Open(ctx context.Context, directory, alias, projectID string, remote Remote) (*Workspace, error) {
	if remote == nil || !aliasPattern.MatchString(alias) || !project.ValidID(projectID) {
		return nil, ErrInvalid
	}
	info, e := os.Lstat(directory)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConflict
	}
	r, e := os.OpenRoot(directory)
	if e != nil {
		return nil, e
	}
	w := &Workspace{root: r, remote: remote}
	ok := false
	defer func() {
		if !ok {
			w.Close()
		}
	}()
	// safe treats its final component as a file, so check a sentinel inside each
	// directory before creation. This rejects links and case aliases as parents.
	for _, dir := range []string{".vf", ".vf/tmp", ".vf/bases"} {
		if e = w.safe(dir+"/.check", true); e != nil {
			return nil, e
		}
	}
	if e = w.safe(".vf/lock", false); e != nil {
		return nil, e
	}
	w.lock, e = r.OpenFile(".vf/lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockFile(w.lock); e != nil {
		return nil, ErrBusy
	}
	var update transition
	if e = w.decode(".vf/update.json", &update); e == nil {
		if update.State.ConnectionAlias != alias || update.State.ProjectID != projectID {
			return nil, ErrConflict
		}
		if e = w.recover(update); e != nil {
			return nil, e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	e = w.decode(".vf/local.json", &w.state)
	if e == nil {
		if w.state.ConnectionAlias != alias || w.state.ProjectID != projectID {
			return nil, ErrConflict
		}
		w.manifest, e = w.base(w.state.BaseRevisionID)
		if e != nil || validateState(w.state, w.manifest) != nil {
			return nil, ErrInvalid
		}
		if e = w.checkProjection(w.manifest, w.manifest); e != nil {
			return nil, e
		}
	} else if errors.Is(e, os.ErrNotExist) {
		if _, e = w.read(".vf/project.json", project.MaxManifestBytes); !errors.Is(e, os.ErrNotExist) {
			return nil, ErrConflict
		}
		opened, e := remote.Open(ctx, projectID, "")
		if e != nil {
			return nil, e
		}
		if opened.ProjectID != projectID || opened.Manifest.ProjectID != projectID || !project.ValidID(opened.RevisionID) || !opened.ManifestRef.Valid() || validateManifest(opened.Manifest) != nil {
			return nil, ErrInvalid
		}
		state := State{1, "local-workspace", alias, projectID, opened.RevisionID, []string{}}
		if e = w.install(state, opened.Manifest); e != nil {
			return nil, e
		}
	} else {
		return nil, e
	}
	if e = w.authorize(ctx); e != nil {
		return nil, e
	}
	if e = w.recoverResolution(ctx); e != nil {
		return nil, e
	}
	ok = true
	return w, nil
}

func (w *Workspace) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lock != nil {
		_ = w.lock.Close()
		w.lock = nil
	}
	if w.root != nil {
		e := w.root.Close()
		w.root = nil
		return e
	}
	return nil
}

func (w *Workspace) authorize(ctx context.Context) error {
	if w.root == nil {
		return ErrInvalid
	}
	opened, e := w.remote.Open(ctx, w.state.ProjectID, w.state.BaseRevisionID)
	if e != nil {
		return e
	}
	if opened.ProjectID != w.state.ProjectID || opened.RevisionID != w.state.BaseRevisionID || !manifestEqual(opened.Manifest, w.manifest) {
		return ErrCorrupt
	}
	return nil
}

func (w *Workspace) checkProjection(old, next project.Manifest) error {
	b, e := w.read(".vf/project.json", project.MaxManifestBytes)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	m, e := project.Decode(b)
	if e != nil || (!manifestEqual(m, old) && !manifestEqual(m, next)) {
		return ErrConflict
	}
	return nil
}

func (w *Workspace) install(state State, m project.Manifest) error {
	if validateState(state, m) != nil {
		return ErrInvalid
	}
	if e := w.storeBase(state.BaseRevisionID, m); e != nil {
		return e
	}
	u := transition{From: w.state.BaseRevisionID, State: state}
	if e := w.writeJSON(".vf/update.json", u, false); e != nil {
		return e
	}
	return w.recover(u)
}

func (w *Workspace) recover(u transition) error {
	next, e := w.base(u.State.BaseRevisionID)
	if e != nil || validateState(u.State, next) != nil {
		return ErrInvalid
	}
	old := next
	if u.From != "" {
		old, e = w.base(u.From)
		if e != nil || old.ProjectID != next.ProjectID {
			return ErrInvalid
		}
	}
	var current State
	e = w.decode(".vf/local.json", &current)
	if e == nil {
		if current.ProjectID != u.State.ProjectID || current.ConnectionAlias != u.State.ConnectionAlias || (current.BaseRevisionID != u.From && current.BaseRevisionID != u.State.BaseRevisionID) {
			return ErrConflict
		}
	} else if !errors.Is(e, os.ErrNotExist) || u.From != "" {
		return ErrInvalid
	}
	if e = w.checkProjection(old, next); e != nil {
		return e
	}
	if e = w.writeJSON(".vf/project.json", next, true); e != nil {
		return e
	}
	if e = w.writeJSON(".vf/local.json", u.State, true); e != nil {
		return e
	}
	if e = w.root.Remove(".vf/update.json"); e != nil {
		return e
	}
	if e = syncDirectory(w.root, ".vf"); e != nil {
		return e
	}
	w.state, w.manifest = u.State, next
	return nil
}

func (w *Workspace) file(id string) (project.File, error) {
	for _, f := range w.manifest.Files {
		if f.ID == id {
			return f, nil
		}
	}
	return project.File{}, ErrInvalid
}
func (w *Workspace) access() project.Access {
	return project.Access{ProjectID: w.state.ProjectID, RevisionID: w.state.BaseRevisionID}
}
func (w *Workspace) metadata(ctx context.Context, f project.File) (project.ContentVersion, error) {
	v, e := w.remote.Metadata(ctx, f.Content, w.access())
	if e != nil {
		return v, e
	}
	if v.ContentRef != f.Content || v.Size < 0 || v.Size > 1<<50 || !hashPattern.MatchString(v.SHA256) {
		return v, ErrCorrupt
	}
	return v, nil
}

func (w *Workspace) remember(id string) error {
	for _, v := range w.state.Materialized {
		if v == id {
			return nil
		}
	}
	next := w.state
	next.Materialized = append(append([]string{}, next.Materialized...), id)
	sort.Strings(next.Materialized)
	if e := w.writeJSON(".vf/local.json", next, true); e != nil {
		return e
	}
	w.state = next
	return nil
}

// Fetch is deliberately non-overwriting even when a local file looks clean.
func (w *Workspace) Fetch(ctx context.Context, id string, maxBytes int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if maxBytes < 0 || maxBytes > 1<<50 {
		return ErrInvalid
	}
	if e := w.authorize(ctx); e != nil {
		return e
	}
	f, e := w.file(id)
	if e != nil {
		return e
	}
	v, e := w.metadata(ctx, f)
	if e != nil {
		return e
	}
	if v.Size > maxBytes {
		return ErrInvalid
	}
	if e = w.safe(f.Path, true); e != nil {
		return e
	}
	hash, size, e := w.checksum(f.Path, v.Size)
	if e == nil {
		if size != v.Size || hash != v.SHA256 {
			return ErrConflict
		}
		return w.remember(id)
	}
	if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	tmp, out, e := w.temporary()
	if e != nil {
		return e
	}
	defer w.root.Remove(tmp)
	h := sha256.New()
	limited := &boundedWriter{dst: io.MultiWriter(out, h), remaining: v.Size}
	e = w.remote.Download(ctx, f.Content, w.access(), limited, maxBytes)
	if e == nil && (limited.overflow || limited.remaining != 0 || hex.EncodeToString(h.Sum(nil)) != v.SHA256) {
		e = ErrCorrupt
	}
	if e == nil {
		e = out.Sync()
	}
	closed := out.Close()
	if e == nil {
		e = closed
	}
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = w.safe(f.Path, true); e != nil {
		return e
	}
	if e = w.root.Link(tmp, f.Path); e != nil {
		if errors.Is(e, os.ErrExist) {
			return ErrConflict
		}
		return e
	}
	if e = syncDirectory(w.root, path.Dir(f.Path)); e != nil {
		return e
	}
	return w.remember(id)
}

type boundedWriter struct {
	dst       io.Writer
	remaining int64
	overflow  bool
}

func (w *boundedWriter) Write(b []byte) (int, error) {
	if int64(len(b)) > w.remaining {
		w.overflow = true
		return 0, ErrCorrupt
	}
	n, e := w.dst.Write(b)
	w.remaining -= int64(n)
	return n, e
}

func (w *Workspace) Status(ctx context.Context) ([]FileStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.authorize(ctx); e != nil {
		return nil, e
	}
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return nil, e
	}
	return w.statusLocked(ctx)
}

// statusLocked inspects the pinned revision without acquiring w.mu. Callers
// must have already authorized the workspace and checked its projection.
func (w *Workspace) statusLocked(ctx context.Context) ([]FileStatus, error) {
	known := map[string]bool{}
	for _, id := range w.state.Materialized {
		known[id] = true
	}
	result := []FileStatus{}
	for _, f := range w.manifest.Files {
		v, e := w.metadata(ctx, f)
		if e != nil {
			return nil, e
		}
		hash, size, e := w.checksum(f.Path, v.Size)
		status := "clean"
		switch {
		case errors.Is(e, os.ErrNotExist):
			status = "not_materialized"
			if known[f.ID] {
				status = "missing"
			}
		case errors.Is(e, ErrConflict):
			status = "conflict"
		case e != nil:
			return nil, e
		case size != v.Size || hash != v.SHA256:
			status = "modified"
		}
		result = append(result, FileStatus{f.ID, f.Path, status})
	}
	return result, nil
}

// SwitchToHead explicitly repins a clean local workspace to the current
// service head. It never merges, overwrites, or deletes local files. A local
// modification or an unfinished commit must be resolved by the user first;
// files from the old revision remain in place and are reclassified against
// the new baseline by the next Status call.
func (w *Workspace) SwitchToHead(ctx context.Context) (State, project.Manifest, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.authorize(ctx); e != nil {
		return State{}, project.Manifest{}, e
	}
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return State{}, project.Manifest{}, e
	}
	for _, name := range []string{".vf/pending.json", ".vf/upload-journal.json"} {
		if _, e := w.read(name, project.MaxManifestBytes); e == nil {
			return State{}, project.Manifest{}, ErrPending
		} else if !errors.Is(e, os.ErrNotExist) {
			return State{}, project.Manifest{}, e
		}
	}
	statuses, e := w.statusLocked(ctx)
	if e != nil {
		return State{}, project.Manifest{}, e
	}
	for _, item := range statuses {
		if item.State == "modified" || item.State == "conflict" {
			return State{}, project.Manifest{}, ErrConflict
		}
	}
	opened, e := w.remote.Open(ctx, w.state.ProjectID, "")
	if e != nil {
		return State{}, project.Manifest{}, e
	}
	if opened.ProjectID != w.state.ProjectID || opened.Manifest.ProjectID != w.state.ProjectID || !project.ValidID(opened.RevisionID) || !opened.ManifestRef.Valid() || validateManifest(opened.Manifest) != nil {
		return State{}, project.Manifest{}, ErrInvalid
	}
	if opened.RevisionID == w.state.BaseRevisionID {
		return w.state, w.manifest, nil
	}
	known := map[string]bool{}
	for _, file := range opened.Manifest.Files {
		known[file.ID] = true
	}
	next := w.state
	next.BaseRevisionID = opened.RevisionID
	next.Materialized = make([]string, 0, len(w.state.Materialized))
	for _, id := range w.state.Materialized {
		if known[id] {
			next.Materialized = append(next.Materialized, id)
		}
	}
	sort.Strings(next.Materialized)
	if e = w.storeBase(next.BaseRevisionID, opened.Manifest); e != nil {
		return State{}, project.Manifest{}, e
	}
	transition := transition{From: w.state.BaseRevisionID, State: next}
	if e = w.writeJSON(".vf/update.json", transition, false); e != nil {
		return State{}, project.Manifest{}, e
	}
	if e = w.recover(transition); e != nil {
		return State{}, project.Manifest{}, e
	}
	return w.state, w.manifest, nil
}
