package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type ConflictFile struct {
	Index          int                `json:"index"`
	Pending        project.FileUpdate `json:"pending"`
	Base           *project.File      `json:"base,omitempty"`
	Head           *project.File      `json:"head,omitempty"`
	RemoteChanged  bool               `json:"remote_changed"`
	CanKeepPending bool               `json:"can_keep_pending"`
}

type ConflictPreview struct {
	ProjectID      string         `json:"project_id"`
	CommitID       string         `json:"commit_id"`
	RequestHash    string         `json:"request_sha256"`
	BaseRevisionID string         `json:"base_revision_id"`
	HeadRevisionID string         `json:"head_revision_id"`
	Files          []ConflictFile `json:"files"`
}

type ConflictChoice struct {
	Index  int     `json:"index"`
	Choice string  `json:"choice"` // head, pending, or text
	Text   *string `json:"text,omitempty"`
}

type ConflictResolution struct {
	CommitID       string           `json:"commit_id"`
	RequestHash    string           `json:"request_sha256"`
	HeadRevisionID string           `json:"head_revision_id"`
	Choices        []ConflictChoice `json:"choices"`
}

type resolutionJournal struct {
	From         string `json:"from_revision_id"`
	Head         string `json:"head_revision_id"`
	Original     string `json:"original_commit_id"`
	OriginalHash string `json:"original_request_sha256"`
	Next         string `json:"next_commit_id,omitempty"`
	NextHash     string `json:"next_request_sha256,omitempty"`
}

func requestHash(r project.CommitRequest) string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func archived(id, suffix string) string { return ".vf/conflicts/" + id + suffix }

func (w *Workspace) terminalConflict(ctx context.Context, r project.CommitRequest) error {
	remote, ok := w.remote.(ConflictRemote)
	if !ok {
		return project.ErrDependency
	}
	if e := w.validatePending(r); e != nil {
		return e
	}
	status, e := remote.CommitStatus(ctx, r.ProjectID, r.CommitID)
	if e != nil {
		return e
	}
	if status.RequestHash != requestHash(r) {
		return ErrCorrupt
	}
	if status.State != "conflict" || status.ErrorCode != project.ErrConflict.Error() || status.Result != nil {
		return ErrPending
	}
	return nil
}

// PreviewConflict is read-only. It never retries or substitutes an uncertain
// request, and it binds decisions to both the exact request and current head.
func (w *Workspace) PreviewConflict(ctx context.Context) (ConflictPreview, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, _, e := w.previewConflict(ctx)
	return p, e
}

func (w *Workspace) previewConflict(ctx context.Context) (ConflictPreview, project.OpenResult, error) {
	fail := func(e error) (ConflictPreview, project.OpenResult, error) {
		return ConflictPreview{}, project.OpenResult{}, e
	}
	if e := w.authorize(ctx); e != nil {
		return fail(e)
	}
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return fail(e)
	}
	if _, e := w.read(".vf/resolution.json", project.MaxManifestBytes); e == nil {
		return fail(ErrPending)
	} else if !errors.Is(e, os.ErrNotExist) {
		return fail(e)
	}
	var r project.CommitRequest
	if e := w.decode(".vf/pending.json", &r); e != nil {
		return fail(e)
	}
	if r.ExpectedRevisionID != w.state.BaseRevisionID {
		return fail(ErrConflict)
	}
	if e := w.terminalConflict(ctx, r); e != nil {
		return fail(e)
	}
	head, e := w.remote.Open(ctx, w.state.ProjectID, "")
	if e != nil {
		return fail(e)
	}
	if head.ProjectID != w.state.ProjectID || head.Manifest.ProjectID != head.ProjectID || !project.ValidID(head.RevisionID) || !head.ManifestRef.Valid() || validateManifest(head.Manifest) != nil {
		return fail(ErrCorrupt)
	}
	p := ConflictPreview{ProjectID: r.ProjectID, CommitID: r.CommitID, RequestHash: requestHash(r), BaseRevisionID: r.ExpectedRevisionID, HeadRevisionID: head.RevisionID, Files: []ConflictFile{}}
	baseFiles, headFiles := map[string]project.File{}, map[string]project.File{}
	for _, f := range w.manifest.Files {
		baseFiles[f.ID] = f
	}
	for _, f := range head.Manifest.Files {
		headFiles[f.ID] = f
	}
	for i, update := range *r.Changes.UpsertFiles {
		item := ConflictFile{Index: i, Pending: update}
		if base, ok := baseFiles[update.ID]; ok {
			item.Base = &base
		}
		if current, ok := headFiles[update.ID]; ok {
			item.Head = &current
		}
		item.RemoteChanged = item.Base == nil || item.Head == nil || *item.Base != *item.Head
		item.CanKeepPending = update.ID == "" || (item.Head != nil && item.Head.Path == update.Path && item.Head.Role == update.Role)
		for _, other := range head.Manifest.Files {
			if other.ID == update.ID {
				continue
			}
			a, b := project.PathKey(other.Path), project.PathKey(update.Path)
			if a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				item.CanKeepPending = false
			}
		}
		p.Files = append(p.Files, item)
	}
	return p, head, nil
}

// ResolveConflict preserves the original journals and all user bytes. Only
// explicitly selected updates enter a fresh CAS request against the previewed
// head. Any failure after preparing is recoverable by reopening or Resume.
func (w *Workspace) ResolveConflict(ctx context.Context, in ConflictResolution) (project.Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, head, e := w.previewConflict(ctx)
	if e != nil {
		return project.Result{}, e
	}
	if in.CommitID != p.CommitID || in.RequestHash != p.RequestHash || in.HeadRevisionID != p.HeadRevisionID {
		return project.Result{}, ErrConflict
	}
	if len(in.Choices) != len(p.Files) {
		return project.Result{}, ErrInvalid
	}
	seen := map[int]bool{}
	updates := []project.FileUpdate{}
	for _, c := range in.Choices {
		if c.Index < 0 || c.Index >= len(p.Files) || seen[c.Index] {
			return project.Result{}, ErrInvalid
		}
		seen[c.Index] = true
		item := p.Files[c.Index]
		switch c.Choice {
		case "head":
			if c.Text != nil {
				return project.Result{}, ErrInvalid
			}
			continue
		case "pending":
			if !item.CanKeepPending || c.Text != nil {
				return project.Result{}, ErrInvalid
			}
		case "text":
			if !item.CanKeepPending || item.Pending.Text == nil || c.Text == nil {
				return project.Result{}, ErrInvalid
			}
			item.Pending.Text = c.Text
		default:
			return project.Result{}, ErrInvalid
		}
		updates = append(updates, item.Pending)
	}
	if e = w.storeBase(head.RevisionID, head.Manifest); e != nil {
		return project.Result{}, e
	}
	j := resolutionJournal{From: p.BaseRevisionID, Head: p.HeadRevisionID, Original: p.CommitID, OriginalHash: p.RequestHash}
	var next project.CommitRequest
	if len(updates) > 0 {
		j.Next = uuid.Must(uuid.NewV7()).String()
		next = project.CommitRequest{ProjectID: p.ProjectID, ExpectedRevisionID: p.HeadRevisionID, CommitID: j.Next, Changes: &project.Changes{UpsertFiles: &updates}}
		if e = w.validatePending(next); e != nil {
			return project.Result{}, e
		}
		j.NextHash = requestHash(next)
	}
	if e = w.safe(".vf/conflicts/.check", true); e != nil {
		return project.Result{}, e
	}
	for _, suffix := range []string{".pending.json", ".upload.json"} {
		source := ".vf/pending.json"
		if suffix == ".upload.json" {
			source = ".vf/upload-journal.json"
		}
		b, readErr := w.read(source, project.MaxManifestBytes)
		if errors.Is(readErr, os.ErrNotExist) && suffix == ".upload.json" {
			continue
		}
		if readErr != nil {
			return project.Result{}, readErr
		}
		if suffix == ".upload.json" {
			var u uploadJournal
			if e = w.decode(source, &u); e != nil || u.CommitID != p.CommitID || u.ProjectID != p.ProjectID || u.ExpectedRevisionID != p.BaseRevisionID {
				return project.Result{}, ErrCorrupt
			}
		}
		if e = w.archive(archived(p.CommitID, suffix), b); e != nil {
			return project.Result{}, e
		}
	}
	if j.Next != "" {
		if e = w.writeJSON(archived(j.Next, ".pending.json"), next, false); e != nil {
			return project.Result{}, e
		}
	}
	if e = w.writeJSON(".vf/resolution.json", j, false); e != nil {
		return project.Result{}, e
	}
	if e = w.recoverResolution(ctx); e != nil {
		return project.Result{}, e
	}
	if j.Next == "" {
		return head.Result, nil
	}
	return w.resume(ctx)
}

func (w *Workspace) archive(name string, b []byte) error {
	old, e := w.read(name, project.MaxManifestBytes)
	if e == nil {
		if string(old) != string(b) {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return w.write(name, b, false)
}

func (w *Workspace) recoverResolution(ctx context.Context) error {
	var j resolutionJournal
	if e := w.decode(".vf/resolution.json", &j); errors.Is(e, os.ErrNotExist) {
		return nil
	} else if e != nil {
		return e
	}
	if !project.ValidID(j.From) || !project.ValidID(j.Head) || !project.ValidID(j.Original) || !hashPattern.MatchString(j.OriginalHash) || (j.Next != "" && (!project.ValidID(j.Next) || j.Next == j.Original || !hashPattern.MatchString(j.NextHash))) || (j.Next == "" && j.NextHash != "") {
		return ErrInvalid
	}
	if w.state.BaseRevisionID != j.From && w.state.BaseRevisionID != j.Head {
		return ErrConflict
	}
	var original project.CommitRequest
	if e := w.decode(archived(j.Original, ".pending.json"), &original); e != nil {
		return e
	}
	if original.CommitID != j.Original || original.ExpectedRevisionID != j.From || requestHash(original) != j.OriginalHash {
		return ErrCorrupt
	}
	if e := w.terminalConflict(ctx, original); e != nil {
		return e
	}
	head, e := w.remote.Open(ctx, w.state.ProjectID, j.Head)
	if e != nil {
		return e
	}
	base, e := w.base(j.Head)
	if e != nil || head.RevisionID != j.Head || head.ProjectID != w.state.ProjectID || !manifestEqual(base, head.Manifest) {
		return ErrCorrupt
	}
	var next project.CommitRequest
	if j.Next != "" {
		if e = w.decode(archived(j.Next, ".pending.json"), &next); e != nil {
			return e
		}
		if next.CommitID != j.Next || next.ExpectedRevisionID != j.Head || requestHash(next) != j.NextHash {
			return ErrCorrupt
		}
		if e = w.validatePending(next); e != nil {
			return e
		}
	}
	var pending project.CommitRequest
	e = w.decode(".vf/pending.json", &pending)
	if e == nil {
		if requestHash(pending) != j.OriginalHash && (j.Next == "" || requestHash(pending) != requestHash(next)) {
			return ErrConflict
		}
	} else if !errors.Is(e, os.ErrNotExist) || j.Next != "" {
		return ErrConflict
	}
	// Validate a remaining upload journal before any state transition. It is
	// evidence of the old operation, never permission to remove another upload.
	u, e := w.read(".vf/upload-journal.json", project.MaxManifestBytes)
	if e == nil {
		prior, readErr := w.read(archived(j.Original, ".upload.json"), project.MaxManifestBytes)
		if readErr != nil || string(prior) != string(u) {
			return ErrConflict
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	state := w.state
	state.BaseRevisionID = j.Head
	state.Materialized = []string{}
	for _, id := range w.state.Materialized {
		for _, f := range head.Manifest.Files {
			if f.ID == id {
				state.Materialized = append(state.Materialized, id)
				break
			}
		}
	}
	if e = w.install(state, head.Manifest); e != nil {
		return e
	}
	if j.Next != "" {
		if e = w.writeJSON(".vf/pending.json", next, true); e != nil {
			return e
		}
	} else if e = w.root.Remove(".vf/pending.json"); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if e = w.removeUploadJournal(); e != nil {
		return e
	}
	if e = syncDirectory(w.root, ".vf"); e != nil {
		return e
	}
	if e = w.root.Remove(".vf/resolution.json"); e != nil {
		return e
	}
	return syncDirectory(w.root, ".vf")
}
