package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"os"
	"sort"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

// SaveTexts snapshots only the explicitly selected, already inventoried texts.
// A retry after uncertain delivery must call Resume with the persisted request.
func (w *Workspace) SaveTexts(ctx context.Context, ids []string) (project.Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(ids) == 0 || len(ids) > project.MaxFiles {
		return project.Result{}, ErrInvalid
	}
	if e := w.authorize(ctx); e != nil {
		return project.Result{}, e
	}
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return project.Result{}, e
	}
	if _, e := w.read(".vf/pending.json", project.MaxManifestBytes); e == nil {
		return project.Result{}, ErrPending
	} else if !errors.Is(e, os.ErrNotExist) {
		return project.Result{}, e
	}
	ids = append([]string{}, ids...)
	sort.Strings(ids)
	updates := []project.FileUpdate{}
	for i, id := range ids {
		if i > 0 && ids[i-1] == id {
			return project.Result{}, ErrInvalid
		}
		f, e := w.file(id)
		if e != nil {
			return project.Result{}, e
		}
		v, e := w.metadata(ctx, f)
		if e != nil {
			return project.Result{}, e
		}
		mt, _, e := mime.ParseMediaType(v.MIME)
		if e != nil || (mt != "text/markdown" && mt != "text/plain" && mt != "application/json") {
			return project.Result{}, ErrInvalid
		}
		b, e := w.read(f.Path, 1<<20)
		if e != nil {
			return project.Result{}, e
		}
		if !utf8.Valid(b) || (mt == "application/json" && !json.Valid(b)) {
			return project.Result{}, ErrInvalid
		}
		text := string(b)
		updates = append(updates, project.FileUpdate{ID: id, Path: f.Path, Role: f.Role, Text: &text, MIME: v.MIME})
	}
	r := project.CommitRequest{ProjectID: w.state.ProjectID, ExpectedRevisionID: w.state.BaseRevisionID, CommitID: uuid.Must(uuid.NewV7()).String(), Changes: &project.Changes{UpsertFiles: &updates}}
	if e := ctx.Err(); e != nil {
		return project.Result{}, e
	}
	if e := w.writeJSON(".vf/pending.json", r, false); e != nil {
		return project.Result{}, e
	}
	return w.resume(ctx)
}

func (w *Workspace) Resume(ctx context.Context) (project.Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.authorize(ctx); e != nil {
		return project.Result{}, e
	}
	return w.resume(ctx)
}

func (w *Workspace) resume(ctx context.Context) (project.Result, error) {
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return project.Result{}, e
	}
	var r project.CommitRequest
	if e := w.decode(".vf/pending.json", &r); e != nil {
		return project.Result{}, e
	}
	if r.ProjectID != w.state.ProjectID || !project.ValidID(r.CommitID) || !project.ValidID(r.ExpectedRevisionID) || r.Changes == nil || r.Changes.UpsertFiles == nil || r.Manifest != nil || len(r.Reselected) != 0 {
		return project.Result{}, ErrInvalid
	}
	// A local recovery file is data, not authority to execute arbitrary changes.
	if r.Changes.Metadata != nil || r.Changes.RemoveFileIDs != nil || r.Changes.Selections != nil || r.Changes.AssetRefs != nil || r.Changes.RunRefs != nil || r.Changes.DomainDocuments != nil {
		return project.Result{}, ErrInvalid
	}
	base, e := w.base(r.ExpectedRevisionID)
	if e != nil || base.ProjectID != r.ProjectID {
		return project.Result{}, ErrInvalid
	}
	known := map[string]project.File{}
	for _, f := range base.Files {
		known[f.ID] = f
	}
	seen := map[string]bool{}
	paths := map[string]bool{}
	for _, file := range base.Files {
		paths[project.PathKey(file.Path)] = true
	}
	for _, f := range *r.Changes.UpsertFiles {
		old, ok := known[f.ID]
		if f.ID != "" && (!ok || seen[f.ID] || f.Path != old.Path || f.Role != old.Role) {
			return project.Result{}, ErrInvalid
		}
		if f.ID == "" && paths[project.PathKey(f.Path)] {
			return project.Result{}, ErrInvalid
		}
		if f.ID != "" {
			seen[f.ID] = true
		}
		if (f.Text == nil) == (f.Content == nil) {
			return project.Result{}, ErrInvalid
		}
		if f.Content != nil {
			if f.MIME != "" || !f.Content.Valid() {
				return project.Result{}, ErrInvalid
			}
			paths[project.PathKey(f.Path)] = true
			continue
		}
		mt, _, err := mime.ParseMediaType(f.MIME)
		if f.ID == "" || len(*f.Text) > 1<<20 || !utf8.ValidString(*f.Text) || err != nil || (mt != "text/plain" && mt != "text/markdown" && mt != "application/json") || (mt == "application/json" && !json.Valid([]byte(*f.Text))) {
			return project.Result{}, ErrInvalid
		}
	}
	if len(seen) == 0 && len(*r.Changes.UpsertFiles) == 0 {
		return project.Result{}, ErrInvalid
	}
	result, e := w.remote.Commit(ctx, r)
	if e != nil {
		return project.Result{}, e
	}
	if result.ProjectID != w.state.ProjectID || result.Manifest.ProjectID != result.ProjectID || !project.ValidID(result.RevisionID) || !result.ManifestRef.Valid() || validateManifest(result.Manifest) != nil {
		return project.Result{}, ErrCorrupt
	}
	if w.state.BaseRevisionID == result.RevisionID {
		if !manifestEqual(w.manifest, result.Manifest) {
			return project.Result{}, ErrCorrupt
		}
	} else {
		if r.ExpectedRevisionID != w.state.BaseRevisionID {
			return project.Result{}, ErrConflict
		}
		next := w.state
		next.BaseRevisionID = result.RevisionID
		if e = w.install(next, result.Manifest); e != nil {
			return project.Result{}, e
		}
	}
	if e = w.root.Remove(".vf/pending.json"); e != nil {
		return project.Result{}, e
	}
	if e = syncDirectory(w.root, ".vf"); e != nil {
		return project.Result{}, e
	}
	return result, nil
}
