package project_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/artifactclient"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

type localRemote struct {
	f          *fixture
	loseCommit bool
}

func (r *localRemote) Open(ctx context.Context, p, rev string) (project.OpenResult, error) {
	return r.f.s.Open(ctx, r.f.p, p, rev)
}
func (r *localRemote) Metadata(ctx context.Context, ref project.ContentRef, a project.Access) (project.ContentVersion, error) {
	return r.f.content.Metadata(ctx, r.f.p, ref, a)
}
func (r *localRemote) Download(ctx context.Context, ref project.ContentRef, a project.Access, dst io.Writer, limit int64) error {
	return r.f.content.Artifact.(*artifactclient.Client).Download(ctx, r.f.p, ref, a, dst, limit)
}
func (r *localRemote) Commit(ctx context.Context, req project.CommitRequest) (project.Result, error) {
	result, e := r.f.s.Commit(ctx, r.f.p, req)
	if e == nil && r.loseCommit {
		r.loseCommit = false
		return project.Result{}, project.ErrDependency
	}
	return result, e
}

func TestWorkingCopiesWithRealProjectAndArtifact(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	remote := &localRemote{f: f}
	created, e := f.s.Create(ctx, f.p, createRequest())
	if e != nil {
		t.Fatal(e)
	}
	media, e := f.content.Write(ctx, f.p, created.ProjectID, project.TextWrite{WriteID: id(), Text: strings.Repeat("synthetic-media", 400000), MIME: "application/octet-stream"})
	if e != nil {
		t.Fatal(e)
	}
	files := []project.FileUpdate{{Path: "media/reference.bin", Role: "reference", Content: &media.ContentRef}}
	runs := []project.RunRef{{ServiceID: "video", RunID: "native/job:fixed"}}
	created, e = f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: created.ProjectID, ExpectedRevisionID: created.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &files, RunRefs: &runs}})
	if e != nil {
		t.Fatal(e)
	}
	aDir, bDir := t.TempDir(), t.TempDir()
	a, e := workspace.Open(ctx, aDir, "service-fixture", created.ProjectID, remote)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := workspace.Open(ctx, bDir, "service-fixture", created.ProjectID, remote)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	entry := created.Manifest.EntryDocumentID
	for _, w := range []*workspace.Workspace{a, b} {
		if e = w.Fetch(ctx, entry, 1<<20); e != nil {
			t.Fatal(e)
		}
	}
	var mediaID string
	for _, file := range created.Manifest.Files {
		if file.Path == "media/reference.bin" {
			mediaID = file.ID
		}
	}
	if e = b.Fetch(ctx, mediaID, 8<<20); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(aDir, "media", "reference.bin")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("sparse checkout downloaded media")
	}
	if e = os.WriteFile(filepath.Join(aDir, "review.md"), []byte("# local submitted edit"), 0600); e != nil {
		t.Fatal(e)
	}
	remote.loseCommit = true
	if _, e = a.SaveTexts(ctx, []string{entry}); !errors.Is(e, project.ErrDependency) {
		t.Fatal("lost commit response not exposed", e)
	}
	committed, e := f.s.Open(ctx, f.p, created.ProjectID, "")
	if e != nil {
		t.Fatal(e)
	}
	if committed.RevisionID == created.RevisionID {
		t.Fatal("save did not commit")
	}
	if e = os.WriteFile(filepath.Join(aDir, "review.md"), []byte("# newer local draft"), 0600); e != nil {
		t.Fatal(e)
	}
	a.Close()
	f.restart()
	a, e = workspace.Open(ctx, aDir, "service-fixture", created.ProjectID, remote)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	recovered, e := a.Resume(ctx)
	if e != nil || recovered.RevisionID != committed.RevisionID {
		t.Fatal("lost response recovery changed identity", e)
	}
	status, e := a.Status(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var state string
	for _, file := range status {
		if file.FileID == entry {
			state = file.State
		}
	}
	if state != "modified" {
		t.Fatal("post-submit draft lost", state)
	}
	if recovered.Manifest.RunRefs[0].RunID != "native/job:fixed" {
		t.Fatal("task identity changed")
	}
	if e = os.WriteFile(filepath.Join(bDir, "review.md"), []byte("# concurrent local edit"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = b.SaveTexts(ctx, []string{entry}); !errors.Is(e, project.ErrConflict) {
		t.Fatal("stale working copy published", e)
	}
	if data, e := os.ReadFile(filepath.Join(bDir, "review.md")); e != nil || string(data) != "# concurrent local edit" {
		t.Fatal("conflict lost draft", e)
	}
	cDir := t.TempDir()
	c, e := workspace.Open(ctx, cDir, "service-fixture", created.ProjectID, remote)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.Fetch(ctx, entry, 1<<20); e != nil {
		t.Fatal(e)
	}
	if data, e := os.ReadFile(filepath.Join(cDir, "review.md")); e != nil || string(data) != "# local submitted edit" {
		t.Fatal("new computer did not read saved revision", e)
	}
	if e = os.Remove(filepath.Join(cDir, "review.md")); e != nil {
		t.Fatal(e)
	}
	if _, e = c.SaveTexts(ctx, []string{entry}); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("local missing file inferred removal", e)
	}
	latest, e := f.s.Open(ctx, f.p, created.ProjectID, "")
	if e != nil || len(latest.Manifest.Files) != 2 {
		t.Fatal("remote inventory changed", e)
	}
	if dir := os.Getenv("WORKSPACE_TEST_EVIDENCE_DIR"); dir != "" {
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		for _, name := range []string{"local.json", "project.json"} {
			data, e := os.ReadFile(filepath.Join(aDir, ".vf", name))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
}
