package project_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

// Runs against two isolated PostgreSQL databases and the real Artifact process.
// It must not be reported as passing when either integration prerequisite is absent.
func TestDramaVersionCommitRecoveryAndRejectedHead(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	request := createRequest()
	request.Category = "ai_short_drama"
	head, err := f.s.Create(ctx, f.p, request)
	if err != nil {
		t.Fatal(err)
	}
	base := `{"schema_version":1,"kind":"project-content","category":"ai_short_drama","ai_short_drama":{"globals":[{"key":"hall","kind":"location","name":"Synthetic Hall","states":[{"key":"night","name":"Rainy night, lamp off","definition_refs":[],"marks":[]}]}],"episodes":[]}}`
	light := "Synthetic complete lighting record: rain, night, lamp off, ambient cool, direction from window; not a creative approval."
	output := "Synthetic lighting proposal V1"
	updates := []project.FileUpdate{{Path: "project-content.json", Role: "domain", Text: &base, MIME: "application/json"}, {Path: "lighting.md", Role: "record", Text: &light, MIME: "text/markdown"}, {Path: "proposal.md", Role: "output", Text: &output, MIME: "text/markdown"}}
	head, err = f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &updates}})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]project.File{}
	for _, file := range head.Manifest.Files {
		files[file.Path] = file
	}
	domains := []project.DomainDocument{{Type: "project-content", FileID: files["project-content.json"].ID}}
	head, err = f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &project.Changes{DomainDocuments: &domains}})
	if err != nil {
		t.Fatal(err)
	}
	basis := head.RevisionID
	var root map[string]any
	json.Unmarshal([]byte(base), &root)
	drama := root["ai_short_drama"].(map[string]any)
	global := drama["globals"].([]any)[0].(map[string]any)
	global["versions"] = []any{map[string]any{"key": "v1", "label": "V1", "state_key": "night", "basis_revision_id": basis, "change_note": "Initial synthetic proposal", "generation_record_file_ids": []string{files["lighting.md"].ID}, "lighting_record_file_id": files["lighting.md"].ID, "review": map[string]any{"state": "unreviewed"}, "file_refs": []any{map[string]any{"file_id": files["proposal.md"].ID, "content_ref": files["proposal.md"].Content, "purpose": "lighting"}}}}
	global["selected_versions"] = []any{map[string]any{"state_key": "night", "purpose": "lighting", "version_key": "v1"}}
	encode := func() string {
		b, e := json.Marshal(root)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	payload := encode()
	domain := files["project-content.json"]
	updates = []project.FileUpdate{{ID: domain.ID, Path: domain.Path, Role: domain.Role, Text: &payload, MIME: "application/json"}}
	selections := []project.Selection{{Purpose: "drama/globals/hall/night/lighting", FileIDs: []string{files["proposal.md"].ID}}}
	commit := project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &updates, Selections: &selections}}
	f.content.loseRetain.Store(true)
	if _, e := f.s.Commit(ctx, f.p, commit); !errors.Is(e, project.ErrDependency) {
		t.Fatal("expected interrupted retention", e)
	}
	f.restart()
	head, err = f.s.Commit(ctx, f.p, commit)
	if err != nil {
		t.Fatal("resume after Artifact restart", err)
	}
	retry, e := f.s.Commit(ctx, f.p, commit)
	if e != nil || retry.RevisionID != head.RevisionID {
		t.Fatal("retry created a new revision", e)
	}
	opened, e := f.s.Open(ctx, f.p, head.ProjectID, head.RevisionID)
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range opened.Manifest.Files {
		if file.ID == domain.ID {
			actual, e := f.content.Read(ctx, f.p, file.Content, project.Access{ProjectID: head.ProjectID, RevisionID: head.RevisionID}, project.MaxManifestBytes)
			if e != nil || string(actual) != payload {
				t.Fatal("versioned content changed in storage", e)
			}
		}
	}
	assertRejected := func(change project.Changes) {
		t.Helper()
		_, e := f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &change})
		if !errors.Is(e, project.ErrInvalid) {
			t.Fatal("invalid content was not rejected", e)
		}
		current, e := f.s.Open(ctx, f.p, head.ProjectID, "")
		if e != nil || current.RevisionID != head.RevisionID {
			t.Fatal("invalid commit changed head", e)
		}
	}
	global["selected_versions"].([]any)[0].(map[string]any)["state_key"] = "day"
	bad := encode()
	badUpdate := []project.FileUpdate{{ID: domain.ID, Path: domain.Path, Role: domain.Role, Text: &bad, MIME: "application/json"}}
	assertRejected(project.Changes{UpsertFiles: &badUpdate})
	changed := "Silently replaced lighting facts"
	lightingUpdate := []project.FileUpdate{{ID: files["lighting.md"].ID, Path: "lighting.md", Role: "record", Text: &changed, MIME: "text/markdown"}}
	assertRejected(project.Changes{UpsertFiles: &lightingUpdate})
	removed := []project.DomainDocument{}
	assertRejected(project.Changes{DomainDocuments: &removed})
	stranger := f.p
	stranger.SubjectID = id()
	if _, e = f.s.Commit(ctx, stranger, project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &updates}}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("ungranted writer accepted", e)
	}
}
