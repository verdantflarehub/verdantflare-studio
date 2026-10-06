package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func (f *fakeRemote) CommitStatus(_ context.Context, _, commit string) (project.CommitStatus, error) {
	if f.denied {
		return project.CommitStatus{}, project.ErrForbidden
	}
	r, ok := f.requests[commit]
	if !ok {
		return project.CommitStatus{}, project.ErrNotFound
	}
	s := project.CommitStatus{State: "conflict", RequestHash: requestHash(r), ErrorCode: project.ErrConflict.Error(), CurrentRevisionID: f.head.RevisionID}
	if result, ok := f.results[commit]; ok {
		s.State = "committed"
		s.ErrorCode = ""
		s.Result = &result
	}
	return s, nil
}

func conflicted(t *testing.T) (*Workspace, *fakeRemote, string, ConflictPreview) {
	t.Helper()
	ctx := context.Background()
	f := fixture()
	dir := t.TempDir()
	w, e := Open(ctx, dir, "test", f.head.ProjectID, f)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { w.Close() })
	fid := f.head.Manifest.Files[0].ID
	if e = w.Fetch(ctx, fid, 1<<20); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "review.md"), []byte("local draft"), 0600); e != nil {
		t.Fatal(e)
	}
	text := "remote edit"
	files := []project.FileUpdate{{ID: fid, Path: "review.md", Role: "review", Text: &text, MIME: "text/markdown"}}
	if _, e = f.Commit(ctx, project.CommitRequest{ProjectID: f.head.ProjectID, ExpectedRevisionID: f.head.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &files}}); e != nil {
		t.Fatal(e)
	}
	if _, e = w.SaveTexts(ctx, []string{fid}); !errors.Is(e, project.ErrConflict) {
		t.Fatal(e)
	}
	p, e := w.PreviewConflict(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !p.Files[0].RemoteChanged || !p.Files[0].CanKeepPending {
		t.Fatal(p)
	}
	return w, f, dir, p
}

func decision(p ConflictPreview, choice string) ConflictResolution {
	return ConflictResolution{CommitID: p.CommitID, RequestHash: p.RequestHash, HeadRevisionID: p.HeadRevisionID, Choices: []ConflictChoice{{Index: 0, Choice: choice}}}
}

func TestResolveConflictPreservesDraftAndOriginalRequest(t *testing.T) {
	for _, choice := range []string{"pending", "head", "text"} {
		t.Run(choice, func(t *testing.T) {
			w, f, dir, p := conflicted(t)
			ctx := context.Background()
			before, _ := os.ReadFile(filepath.Join(dir, ".vf/pending.json"))
			in := decision(p, choice)
			merged := "reviewed merge"
			if choice == "text" {
				in.Choices[0].Text = &merged
			}
			result, e := w.ResolveConflict(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			want := "local draft"
			if choice == "head" {
				want = "remote edit"
				if result.RevisionID != p.HeadRevisionID {
					t.Fatal("unnecessary revision")
				}
			}
			if choice == "text" {
				want = merged
			}
			if got := string(f.contents[result.Manifest.Files[0].Content]); got != want {
				t.Fatal(got)
			}
			if string(mustRead(t, filepath.Join(dir, "review.md"))) != "local draft" {
				t.Fatal("user bytes overwritten")
			}
			if got := mustRead(t, filepath.Join(dir, archived(p.CommitID, ".pending.json"))); string(got) != string(before) {
				t.Fatal("original request lost")
			}
			for _, name := range []string{"pending.json", "resolution.json"} {
				if _, e = os.Stat(filepath.Join(dir, ".vf", name)); !errors.Is(e, os.ErrNotExist) {
					t.Fatal(name, e)
				}
			}
		})
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestResolveConflictRejectsStalePreviewAndChangedRequest(t *testing.T) {
	w, f, dir, p := conflicted(t)
	ctx := context.Background()
	original := mustRead(t, filepath.Join(dir, ".vf/pending.json"))
	text := "newer"
	files := []project.FileUpdate{{ID: f.head.Manifest.EntryDocumentID, Path: "review.md", Role: "review", Text: &text, MIME: "text/markdown"}}
	_, e := f.Commit(ctx, project.CommitRequest{ProjectID: f.head.ProjectID, ExpectedRevisionID: f.head.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &files}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.ResolveConflict(ctx, decision(p, "pending")); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if string(mustRead(t, filepath.Join(dir, ".vf/pending.json"))) != string(original) {
		t.Fatal("pending changed")
	}
	var r project.CommitRequest
	if e = w.decode(".vf/pending.json", &r); e != nil {
		t.Fatal(e)
	}
	changed := "changed after request"
	(*r.Changes.UpsertFiles)[0].Text = &changed
	if e = w.writeJSON(".vf/pending.json", r, true); e != nil {
		t.Fatal(e)
	}
	if _, e = w.PreviewConflict(ctx); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}

func TestResolvedCommitResponseLossUsesSameNewID(t *testing.T) {
	w, f, dir, p := conflicted(t)
	f.lose = true
	if _, e := w.ResolveConflict(context.Background(), decision(p, "pending")); !errors.Is(e, project.ErrDependency) {
		t.Fatal(e)
	}
	var pending project.CommitRequest
	if e := w.decode(".vf/pending.json", &pending); e != nil {
		t.Fatal(e)
	}
	if pending.CommitID == p.CommitID {
		t.Fatal("old ID reused for new request")
	}
	count := len(f.results)
	if _, e := w.Resume(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(f.results) != count {
		t.Fatal("duplicate result")
	}
	if string(mustRead(t, filepath.Join(dir, "review.md"))) != "local draft" {
		t.Fatal("draft changed")
	}
}

func TestResolutionRecoversAtEachLocalTransition(t *testing.T) {
	for _, stage := range []string{"prepared", "installed", "replaced"} {
		t.Run(stage, func(t *testing.T) {
			w, f, dir, p := conflicted(t)
			ctx := context.Background()
			var original project.CommitRequest
			if e := w.decode(".vf/pending.json", &original); e != nil {
				t.Fatal(e)
			}
			next := original
			next.CommitID = id()
			next.ExpectedRevisionID = p.HeadRevisionID
			j := resolutionJournal{From: p.BaseRevisionID, Head: p.HeadRevisionID, Original: p.CommitID, OriginalHash: p.RequestHash, Next: next.CommitID, NextHash: requestHash(next)}
			if e := w.safe(".vf/conflicts/.check", true); e != nil {
				t.Fatal(e)
			}
			if e := w.storeBase(p.HeadRevisionID, f.head.Manifest); e != nil {
				t.Fatal(e)
			}
			for name, value := range map[string]any{archived(p.CommitID, ".pending.json"): original, archived(next.CommitID, ".pending.json"): next, ".vf/resolution.json": j} {
				if e := w.writeJSON(name, value, false); e != nil {
					t.Fatal(e)
				}
			}
			if stage != "prepared" {
				state := w.state
				state.BaseRevisionID = p.HeadRevisionID
				if e := w.install(state, f.head.Manifest); e != nil {
					t.Fatal(e)
				}
			}
			if stage == "replaced" {
				if e := w.writeJSON(".vf/pending.json", next, true); e != nil {
					t.Fatal(e)
				}
			}
			w.Close()
			reopened, e := Open(ctx, dir, "test", p.ProjectID, f)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			var got project.CommitRequest
			if e = reopened.decode(".vf/pending.json", &got); e != nil || got.CommitID != next.CommitID {
				t.Fatal(e, got.CommitID)
			}
			if _, e = reopened.Resume(ctx); e != nil {
				t.Fatal(e)
			}
			if string(mustRead(t, filepath.Join(dir, "review.md"))) != "local draft" {
				t.Fatal("draft changed")
			}
		})
	}
}
