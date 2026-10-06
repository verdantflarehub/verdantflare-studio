package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func id() string { return uuid.Must(uuid.NewV7()).String() }

type fakeRemote struct {
	head          project.OpenResult
	versions      map[string]project.OpenResult
	contents      map[project.ContentRef][]byte
	results       map[string]project.Result
	lose          bool
	corrupt       string
	afterDownload func()
	afterCommit   func()
	denied        bool
	downloaded    int
	uploads       map[string]project.ContentVersion
	uploadCalls   map[string]int
	loseUpload    bool
}

func fixture() *fakeRemote {
	f := &fakeRemote{versions: map[string]project.OpenResult{}, contents: map[project.ContentRef][]byte{}, results: map[string]project.Result{}, uploads: map[string]project.ContentVersion{}, uploadCalls: map[string]int{}}
	text, binary := project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}, project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}
	f.contents[text], f.contents[binary] = []byte("# Review\n"), bytes.Repeat([]byte("synthetic-media"), 400000)
	m := project.Manifest{SchemaVersion: 1, Kind: "project", ProjectID: id(), Name: "fixture", Category: "image", Status: "draft", Files: []project.File{{ID: id(), Path: "review.md", Role: "review", Content: text}, {ID: id(), Path: "images/reference.bin", Role: "reference", Content: binary}}, Selections: []project.Selection{}, AssetRefs: []project.AssetRef{}, RunRefs: []project.RunRef{{ServiceID: "video", RunID: "native/job:42"}}, DomainDocuments: []project.DomainDocument{}}
	m.EntryDocumentID = m.Files[0].ID
	f.head = project.OpenResult{Result: project.Result{ProjectID: m.ProjectID, RevisionID: id(), ManifestRef: project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}, Manifest: m, Invalidated: []string{}}}
	f.head.HeadRevisionID = f.head.RevisionID
	f.versions[f.head.RevisionID] = f.head
	return f
}
func (f *fakeRemote) Open(_ context.Context, p, rev string) (project.OpenResult, error) {
	if f.denied {
		return project.OpenResult{}, project.ErrForbidden
	}
	if p != f.head.ProjectID {
		return project.OpenResult{}, project.ErrNotFound
	}
	if rev == "" {
		rev = f.head.RevisionID
	}
	o, ok := f.versions[rev]
	if !ok {
		return o, project.ErrNotFound
	}
	o.HeadRevisionID = f.head.RevisionID
	return o, nil
}
func (f *fakeRemote) Metadata(_ context.Context, ref project.ContentRef, _ project.Access) (project.ContentVersion, error) {
	if f.denied {
		return project.ContentVersion{}, project.ErrForbidden
	}
	b, ok := f.contents[ref]
	if !ok {
		return project.ContentVersion{}, project.ErrNotFound
	}
	h := sha256.Sum256(b)
	return project.ContentVersion{ContentRef: ref, Size: int64(len(b)), SHA256: hex.EncodeToString(h[:]), MIME: "text/markdown"}, nil
}
func (f *fakeRemote) Download(ctx context.Context, ref project.ContentRef, _ project.Access, dst io.Writer, _ int64) error {
	if f.denied {
		return project.ErrForbidden
	}
	f.downloaded++
	b := f.contents[ref]
	switch f.corrupt {
	case "short":
		b = b[:len(b)-1]
	case "long":
		b = append(append([]byte{}, b...), 1)
	case "digest":
		b = bytes.Repeat([]byte("x"), len(b))
	case "error":
		_, _ = dst.Write(b[:1])
		return io.ErrUnexpectedEOF
	}
	for len(b) > 0 {
		if e := ctx.Err(); e != nil {
			return e
		}
		n := min(len(b), 32768)
		if _, e := dst.Write(b[:n]); e != nil {
			return e
		}
		b = b[n:]
	}
	if f.afterDownload != nil {
		f.afterDownload()
	}
	return nil
}
func (f *fakeRemote) Upload(_ context.Context, req UploadRequest, src io.Reader) (project.ContentVersion, error) {
	f.uploadCalls[req.WriteID]++
	if prior, ok := f.uploads[req.WriteID]; ok {
		return prior, nil
	}
	b, e := io.ReadAll(src)
	if e != nil || int64(len(b)) != req.Size {
		return project.ContentVersion{}, project.ErrDependency
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != req.SHA256 {
		return project.ContentVersion{}, project.ErrNotReady
	}
	ref := project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}
	f.contents[ref] = b
	v := project.ContentVersion{SchemaVersion: 2, ContentRef: ref, Size: req.Size, SHA256: req.SHA256, MIME: req.MIME, Source: req.Source}
	f.uploads[req.WriteID] = v
	if f.loseUpload {
		f.loseUpload = false
		return project.ContentVersion{}, project.ErrDependency
	}
	return v, nil
}
func (f *fakeRemote) Commit(_ context.Context, r project.CommitRequest) (project.Result, error) {
	if f.denied {
		return project.Result{}, project.ErrForbidden
	}
	if prior, ok := f.results[r.CommitID]; ok {
		return prior, nil
	}
	if r.ExpectedRevisionID != f.head.RevisionID {
		return project.Result{}, &project.ConflictError{CurrentRevisionID: f.head.RevisionID}
	}
	result := f.head.Result
	result.Manifest.Files = append([]project.File{}, result.Manifest.Files...)
	for _, update := range *r.Changes.UpsertFiles {
		if update.ID == "" {
			if update.Content == nil {
				continue
			}
			result.Manifest.Files = append(result.Manifest.Files, project.File{ID: id(), Path: update.Path, Role: update.Role, Content: *update.Content})
			continue
		}
		for i, file := range result.Manifest.Files {
			if file.ID == update.ID {
				if update.Content != nil {
					result.Manifest.Files[i].Content = *update.Content
					continue
				}
				ref := project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}
				f.contents[ref] = []byte(*update.Text)
				result.Manifest.Files[i].Content = ref
			}
		}
	}
	result.RevisionID = id()
	result.ManifestRef = project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}
	f.head = project.OpenResult{Result: result, HeadRevisionID: result.RevisionID}
	f.versions[result.RevisionID] = f.head
	f.results[r.CommitID] = result
	if f.afterCommit != nil {
		f.afterCommit()
	}
	if f.lose {
		f.lose = false
		return project.Result{}, project.ErrDependency
	}
	return result, nil
}
func openFixture(t *testing.T, f *fakeRemote) (*Workspace, string) {
	t.Helper()
	dir := t.TempDir()
	w, e := Open(context.Background(), dir, "local-test", f.head.ProjectID, f)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { w.Close() })
	return w, dir
}

func TestSparseCheckoutLargeFetchAndMissingIsNotDelete(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	ctx := context.Background()
	entry, media := f.head.Manifest.Files[0], f.head.Manifest.Files[1]
	if e := w.Fetch(ctx, entry.ID, 1<<20); e != nil {
		t.Fatal(e)
	}
	s, e := w.Status(ctx)
	if e != nil || s[0].State != "clean" || s[1].State != "not_materialized" || f.downloaded != 1 {
		t.Fatal(s, e)
	}
	if e = w.Fetch(ctx, media.ID, 8<<20); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(dir, "review.md")); e != nil {
		t.Fatal(e)
	}
	s, e = w.Status(ctx)
	if e != nil || s[0].State != "missing" || len(f.head.Manifest.Files) != 2 {
		t.Fatal(s, e)
	}
	if e = w.Fetch(ctx, entry.ID, 1<<20); e != nil {
		t.Fatal(e)
	}
	if f.head.Manifest.RunRefs[0].RunID != "native/job:42" {
		t.Fatal("native run changed")
	}
	second, _ := openFixture(t, f)
	if e = second.Fetch(ctx, entry.ID, 1<<20); e != nil {
		t.Fatal(e)
	}
	f.denied = true
	if e = w.Fetch(ctx, entry.ID, 1<<20); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("cached file bypassed live authorization", e)
	}
}

func TestDownloadFailureNeverExposesPartialOrOverwritesRace(t *testing.T) {
	for _, mode := range []string{"short", "long", "digest", "error", "race"} {
		t.Run(mode, func(t *testing.T) {
			f := fixture()
			w, dir := openFixture(t, f)
			f.corrupt = mode
			if mode == "race" {
				f.afterDownload = func() {
					if e := os.WriteFile(filepath.Join(dir, "review.md"), []byte("local edit"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := w.Fetch(context.Background(), f.head.Manifest.EntryDocumentID, 1<<20); e == nil {
				t.Fatal("bad download succeeded")
			}
			b, e := os.ReadFile(filepath.Join(dir, "review.md"))
			if mode == "race" {
				if e != nil || string(b) != "local edit" {
					t.Fatal("overwrote racing edit", e)
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				t.Fatal("partial visible", e)
			}
			files, e := os.ReadDir(filepath.Join(dir, ".vf", "tmp"))
			if e != nil || len(files) != 0 {
				t.Fatal("temporary bytes retained", e)
			}
		})
	}
}

func TestSaveLostResponsePinsRequestAndPreservesLaterEdit(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	ctx := context.Background()
	entry := f.head.Manifest.EntryDocumentID
	if e := w.Fetch(ctx, entry, 1<<20); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "review.md"), []byte("# submitted"), 0600); e != nil {
		t.Fatal(e)
	}
	f.lose = true
	if _, e := w.SaveTexts(ctx, []string{entry}); !errors.Is(e, project.ErrDependency) {
		t.Fatal(e)
	}
	committed := f.head.RevisionID
	if e := os.WriteFile(filepath.Join(dir, "review.md"), []byte("# later unsaved edit"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := w.SaveTexts(ctx, []string{entry}); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
	w.Close()
	resumed, e := Open(ctx, dir, "local-test", f.head.ProjectID, f)
	if e != nil {
		t.Fatal(e)
	}
	defer resumed.Close()
	r, e := resumed.Resume(ctx)
	if e != nil || r.RevisionID != committed || len(f.results) != 1 {
		t.Fatal("retry created another revision", e)
	}
	s, e := resumed.Status(ctx)
	if e != nil || s[0].State != "modified" {
		t.Fatal("later local edit lost", s, e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "review.md"))
	if e != nil || string(b) != "# later unsaved edit" {
		t.Fatal(e)
	}
	if _, e = resumed.SaveTexts(ctx, []string{entry}); e != nil {
		t.Fatal(e)
	}
	s, e = resumed.Status(ctx)
	if e != nil || s[0].State != "clean" {
		t.Fatal(s, e)
	}
}

func TestSaveFilesUploadsExplicitNewAndExistingContent(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	defer w.Close()
	ctx := context.Background()
	media := f.head.Manifest.Files[1]
	if e := os.MkdirAll(filepath.Join(dir, "images"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "images", "reference.bin"), []byte("new-media"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := w.SaveFiles(ctx, []FileInput{{FileID: media.ID, Path: media.Path, Role: media.Role, MIME: "application/octet-stream"}}); e != nil {
		t.Fatal("existing media replacement failed", e)
	}
	if e := os.WriteFile(filepath.Join(dir, "new-reference.png"), []byte("new-file"), 0600); e != nil {
		t.Fatal(e)
	}
	result, e := w.SaveFiles(ctx, []FileInput{{Path: "new-reference.png", Role: "reference", MIME: "image/png"}})
	if e != nil {
		t.Fatal("new media import failed", e)
	}
	if len(result.Manifest.Files) != 3 {
		t.Fatalf("new file not committed: %d files", len(result.Manifest.Files))
	}
	status, e := w.Status(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range status {
		if file.Path == "new-reference.png" && file.State != "clean" {
			t.Fatalf("new file status %s", file.State)
		}
	}
	if _, e := w.SaveFiles(ctx, []FileInput{{Path: "review.md", Role: "review", MIME: "text/markdown"}}); !errors.Is(e, ErrConflict) {
		t.Fatalf("path-only replacement was accepted: %v", e)
	}
}

func TestSaveFilesUploadJournalResumesAfterLostResponse(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	if e := os.WriteFile(filepath.Join(dir, "new-reference.png"), []byte("new-file"), 0600); e != nil {
		t.Fatal(e)
	}
	f.loseUpload = true
	if _, e := w.SaveFiles(context.Background(), []FileInput{{Path: "new-reference.png", Role: "reference", MIME: "image/png"}}); !errors.Is(e, project.ErrDependency) {
		t.Fatalf("lost upload response was not surfaced: %v", e)
	}
	if _, e := os.Stat(filepath.Join(dir, ".vf", "upload-journal.json")); e != nil {
		t.Fatalf("upload journal missing: %v", e)
	}
	if _, e := os.Stat(filepath.Join(dir, ".vf", "pending.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("pending commit written before upload recovery: %v", e)
	}
	w.Close()
	resumed, e := Open(context.Background(), dir, "local-test", f.head.ProjectID, f)
	if e != nil {
		t.Fatal(e)
	}
	defer resumed.Close()
	result, e := resumed.Resume(context.Background())
	if e != nil || len(result.Manifest.Files) != 3 {
		t.Fatalf("upload journal did not resume: %v", e)
	}
	if len(f.uploads) != 1 || len(f.results) != 1 {
		t.Fatalf("resume created duplicate remote work: uploads=%d commits=%d", len(f.uploads), len(f.results))
	}
	for writeID, calls := range f.uploadCalls {
		if calls != 2 || !project.ValidID(writeID) {
			t.Fatalf("unstable upload retry: write_id=%s calls=%d", writeID, calls)
		}
	}
	if _, e = os.Stat(filepath.Join(dir, ".vf", "upload-journal.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("upload journal retained after recovery: %v", e)
	}
}

func TestImportFilesCopiesOnlyExplicitSources(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	defer w.Close()
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "candidate.wav")
	if e := os.WriteFile(source, []byte("external-media"), 0600); e != nil {
		t.Fatal(e)
	}
	result, e := w.ImportFiles(context.Background(), []ImportInput{{SourcePath: source, Path: "audio/candidate.wav", Role: "reference", MIME: "audio/wav"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(result.Manifest.Files) != 3 {
		t.Fatalf("import did not add one file: %d", len(result.Manifest.Files))
	}
	data, e := os.ReadFile(filepath.Join(dir, "audio", "candidate.wav"))
	if e != nil || string(data) != "external-media" {
		t.Fatalf("imported bytes unavailable: %v", e)
	}
	if _, e = w.ImportFiles(context.Background(), []ImportInput{{SourcePath: source, Path: "audio/candidate.wav", Role: "reference", MIME: "audio/wav"}}); !errors.Is(e, ErrConflict) {
		t.Fatalf("existing target was overwritten: %v", e)
	}
}

func TestRecoverInterruptedMetadataAndProtectManualManifest(t *testing.T) {
	for _, phase := range []string{"journal", "projection", "state", "edited"} {
		t.Run(phase, func(t *testing.T) {
			f := fixture()
			w, dir := openFixture(t, f)
			old := w.state
			next := old
			next.BaseRevisionID = id()
			m := f.head.Manifest
			m.Name = "new revision"
			opened := f.head
			opened.Manifest = m
			opened.RevisionID = next.BaseRevisionID
			opened.HeadRevisionID = next.BaseRevisionID
			f.versions[next.BaseRevisionID] = opened
			f.head = opened
			if e := w.storeBase(next.BaseRevisionID, m); e != nil {
				t.Fatal(e)
			}
			if e := w.writeJSON(".vf/update.json", transition{From: old.BaseRevisionID, State: next}, false); e != nil {
				t.Fatal(e)
			}
			if phase != "journal" {
				if e := w.writeJSON(".vf/project.json", m, true); e != nil {
					t.Fatal(e)
				}
			}
			if phase == "state" {
				if e := w.writeJSON(".vf/local.json", next, true); e != nil {
					t.Fatal(e)
				}
			}
			if phase == "edited" {
				edited := m
				edited.Name = "user structural edit"
				if e := w.writeJSON(".vf/project.json", edited, true); e != nil {
					t.Fatal(e)
				}
			}
			w.Close()
			got, e := Open(context.Background(), dir, "local-test", f.head.ProjectID, f)
			if phase == "edited" {
				if !errors.Is(e, ErrConflict) {
					t.Fatal("manual projection overwritten", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer got.Close()
			if got.state.BaseRevisionID != next.BaseRevisionID || got.manifest.Name != "new revision" {
				t.Fatal("journal not recovered")
			}
		})
	}
}

func TestConflictsPathsAndExistingData(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	ctx := context.Background()
	if e := os.WriteFile(filepath.Join(dir, "review.md"), []byte("keep me"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := w.Fetch(ctx, f.head.Manifest.EntryDocumentID, 1<<20); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(dir, "Images"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := w.Fetch(ctx, f.head.Manifest.Files[1].ID, 8<<20); !errors.Is(e, ErrConflict) {
		t.Fatal("case-colliding directory accepted", e)
	}
	if other, e := Open(ctx, dir, "local-test", f.head.ProjectID, f); !errors.Is(e, ErrBusy) {
		if other != nil {
			other.Close()
		}
		t.Fatal("concurrent copy opened", e)
	}
	m := f.head.Manifest
	m.Files = append(append([]project.File{}, m.Files...), project.File{ID: id(), Path: "images", Role: "input", Content: project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}})
	if validateManifest(m) == nil {
		t.Fatal("file/directory prefix accepted")
	}
	// Real server conflict: the local base must stay pinned and bytes unchanged.
	base := f.head
	newer := base
	newer.RevisionID = id()
	newer.HeadRevisionID = newer.RevisionID
	f.head = newer
	f.versions[newer.RevisionID] = newer
	if _, e := w.SaveTexts(ctx, []string{base.Manifest.EntryDocumentID}); !errors.Is(e, project.ErrConflict) {
		t.Fatal(e)
	}
	if w.state.BaseRevisionID != base.RevisionID {
		t.Fatal("conflict advanced base")
	}
	b, e := os.ReadFile(filepath.Join(dir, "review.md"))
	if e != nil || string(b) != "keep me" {
		t.Fatal(e)
	}
}

func TestSymlinkParentDoesNotEscape(t *testing.T) {
	f := fixture()
	w, dir := openFixture(t, f)
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(dir, "images")); e != nil {
		t.Skipf("OS symlink privilege unavailable: %v", e)
	}
	if e := w.Fetch(context.Background(), f.head.Manifest.Files[1].ID, 8<<20); e == nil {
		t.Fatal("followed symlink")
	}
	entries, e := os.ReadDir(outside)
	if e != nil || len(entries) != 0 {
		t.Fatal("wrote outside root", e)
	}
}

func TestWorkspaceLockProcess(t *testing.T) {
	if os.Getenv("VF_WORKSPACE_LOCK_HELPER") == "1" {
		r, e := os.OpenRoot(os.Getenv("VF_WORKSPACE_LOCK_ROOT"))
		if e != nil {
			os.Exit(2)
		}
		defer r.Close()
		f, e := r.OpenFile(".vf/lock", os.O_RDWR, 0600)
		if e != nil {
			os.Exit(3)
		}
		defer f.Close()
		if lockFile(f) == nil {
			os.Exit(4)
		}
		return
	}
	f := fixture()
	w, dir := openFixture(t, f)
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkspaceLockProcess$")
	cmd.Env = append(os.Environ(), "VF_WORKSPACE_LOCK_HELPER=1", "VF_WORKSPACE_LOCK_ROOT="+dir)
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("process lock: %v %s", e, b)
	}
	w.Close()
	other, e := Open(context.Background(), dir, "local-test", f.head.ProjectID, f)
	if e != nil {
		t.Fatal("closed process lock not released", e)
	}
	other.Close()
}
