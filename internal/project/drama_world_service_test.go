package project_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

// Calls the real MCP/HTTP handler and business service. setup requires disposable
// databases on the configured PostgreSQL and the actual Artifact executable.
func TestDramaMCPFixedPublicationAndAssociationRecovery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	encode := func(value any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	call := func(p project.Principal, name string, args, result any, wantStatus int, wantCode string) {
		t.Helper()
		handler, err := f.s.MCPHandler(strings.Repeat("s", 32), strings.Repeat("a", 32))
		if err != nil {
			t.Fatal(err)
		}
		rpcID := id()
		body := encode(map[string]any{"jsonrpc": "2.0", "id": rpcID, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		req := httptest.NewRequest("POST", "/mcp", bytes.NewBufferString(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+strings.Repeat("s", 32))
		req.Header.Set("X-Organization-Id", p.OrganizationID)
		req.Header.Set("X-User-Id", p.SubjectID)
		req.Header.Set("X-Request-Id", id())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		var envelope struct {
			ID     string `json:"id"`
			Result struct {
				Data    json.RawMessage `json:"structuredContent"`
				IsError bool            `json:"isError"`
			} `json:"result"`
		}
		if response.Code != wantStatus || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.ID != rpcID || envelope.Result.IsError != (wantStatus >= 400) {
			t.Fatalf("%s: status/envelope mismatch (%d): %s", name, response.Code, response.Body.String())
		}
		if wantCode != "" {
			var failure struct {
				Code string `json:"code"`
			}
			if json.Unmarshal(envelope.Result.Data, &failure) != nil || failure.Code != wantCode {
				t.Fatalf("%s: wrong domain error: %s", name, envelope.Result.Data)
			}
		} else if result != nil {
			if err := json.Unmarshal(envelope.Result.Data, result); err != nil {
				t.Fatal(name, err)
			}
		}
	}
	var head project.Result
	create := createRequest()
	create.Category = "ai_short_drama"
	create.Name = "Synthetic lighting publication integration"
	call(f.p, "project.create", create, &head, 200, "")
	root := map[string]any{"schema_version": 1, "kind": "project-content", "category": "ai_short_drama"}
	global := map[string]any{"key": "hall", "kind": "location", "name": "Synthetic hall", "states": []any{map[string]any{"key": "night", "name": "Rainy night, lamp off", "definition_refs": []any{}, "marks": []any{}}}}
	drama := map[string]any{"globals": []any{global}, "episodes": []any{}}
	root["ai_short_drama"] = drama
	updates := []project.FileUpdate{}
	for _, doc := range []struct{ path, text string }{
		{"project-content.json", encode(root)},
		{"lighting-l1.md", "Synthetic L1: night, rain, lamp off; cool window light from left, low intensity, hard shadows, weak ambient light, fixed exposure."},
		{"output-l1.md", "Synthetic L1 lighting output"},
		{"lighting-l2.md", "Synthetic L2: softer shadows, brighter ambient light; other settings unchanged."},
		{"output-l2.md", "Synthetic L2 lighting output"},
		{"publication-review.md", "Synthetic approval fixture only, not human creative approval."},
	} {
		mime := "text/markdown"
		if doc.path == "project-content.json" {
			mime = "application/json"
		}
		text := doc.text
		updates = append(updates, project.FileUpdate{Path: doc.path, Role: "document", Text: &text, MIME: mime})
	}
	commit := func(changes project.Changes) {
		t.Helper()
		call(f.p, "project.commit", project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &changes}, &head, 200, "")
	}
	commit(project.Changes{UpsertFiles: &updates})
	files := map[string]project.File{}
	for _, file := range head.Manifest.Files {
		files[file.Path] = file
	}
	domain := files["project-content.json"]
	domains := []project.DomainDocument{{Type: "project-content", FileID: domain.ID}}
	commit(project.Changes{DomainDocuments: &domains})
	basis := head.RevisionID
	fileRef := func(path, purpose string) map[string]any {
		file := files[path]
		return map[string]any{"file_id": file.ID, "content_ref": file.Content, "purpose": purpose}
	}
	versions := []any{}
	for _, key := range []string{"l1", "l2"} {
		version := map[string]any{"key": key, "label": strings.ToUpper(key), "state_key": "night", "basis_revision_id": basis, "change_note": "Synthetic " + key + " lighting change", "file_refs": []any{fileRef("output-"+key+".md", "lighting")}, "lighting_record_file_id": files["lighting-"+key+".md"].ID, "generation_record_file_ids": []any{}, "review": map[string]any{"state": "unreviewed"}}
		if key == "l2" {
			version["parent_version_key"] = "l1"
		}
		versions = append(versions, version)
	}
	global["versions"] = versions
	global["selected_versions"] = []any{map[string]any{"state_key": "night", "purpose": "lighting", "version_key": "l2"}}
	locator := map[string]any{"kind": "global", "key": "hall"}
	packageText := encode(map[string]any{"schema_version": 1, "kind": "short-drama-publication", "source": locator, "versions": []any{map[string]any{"version_key": "l1", "file_refs": []any{fileRef("output-l1.md", "lighting")}}}, "supporting_file_refs": []any{fileRef("lighting-l1.md", "lighting_record")}})
	updates = []project.FileUpdate{{Path: "publication.json", Role: "document", Text: &packageText, MIME: "application/json"}}
	commit(project.Changes{UpsertFiles: &updates})
	for _, file := range head.Manifest.Files {
		files[file.Path] = file
	}
	candidate := map[string]any{"key": "hall-package", "name": "Synthetic L1 package", "source": locator, "publication_record_file_id": files["publication.json"].ID, "review": map[string]any{"state": "approved", "record_file_id": files["publication-review.md"].ID}}
	drama["world_candidates"] = []any{candidate}
	contentChange := func() project.Changes {
		text := encode(root)
		upserts := []project.FileUpdate{{ID: domain.ID, Path: domain.Path, Role: domain.Role, Text: &text, MIME: "application/json"}}
		return project.Changes{UpsertFiles: &upserts}
	}
	changes := contentChange()
	selections := []project.Selection{{Purpose: "drama/globals/hall/night/lighting", FileIDs: []string{files["output-l2.md"].ID}}}
	changes.Selections = &selections
	commit(changes)
	source := head
	request := project.WorldRegisterRequest{CommitID: id(), SourceProjectID: source.ProjectID, SourceRevisionID: source.RevisionID, Name: "Synthetic L1 package", AssetType: "location", Subjects: []string{"Synthetic hall"}, Relation: "produced_in", ReviewFileID: files["publication-review.md"].ID, FileIDs: []string{files["publication.json"].ID, files["output-l1.md"].ID, files["lighting-l1.md"].ID}}
	candidate["registration_request"] = request
	commit(contentChange())
	assertHead := func(expected string) {
		t.Helper()
		var current project.OpenResult
		call(f.p, "project.open", project.OpenRequest{ProjectID: head.ProjectID}, &current, 200, "")
		if current.RevisionID != expected {
			t.Fatal("unexpected source head change")
		}
	}
	// A World request cannot quietly include the project-selected newer output.
	wrong := request
	wrong.CommitID = id()
	wrong.FileIDs = append(append([]string{}, request.FileIDs...), files["output-l2.md"].ID)
	call(f.p, "world.register", wrong, nil, 400, project.ErrInvalid.Error())
	assertHead(head.RevisionID)
	stranger := f.p
	stranger.SubjectID = id()
	call(stranger, "world.register", request, nil, 403, project.ErrForbidden.Error())
	// Interrupt after real Artifact retention, then replace both service instances.
	f.content.loseRetain.Store(true)
	call(f.p, "world.register", request, nil, 503, project.ErrDependency.Error())
	if count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions") != 0 {
		t.Fatal("interrupted publication exposed a World version")
	}
	f.restart()
	var err error
	f.s, err = project.NewService(ctx, f.db, f.content, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.active.Store(f.s)
	var published, retry project.WorldResult
	call(f.p, "world.register", request, &published, 200, "")
	call(f.p, "world.register", request, &retry, 200, "")
	if !reflect.DeepEqual(published, retry) || count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions") != 1 {
		t.Fatal("recovery created more than one fixed asset version")
	}
	if published.Manifest.Source.ProjectID != source.ProjectID || published.Manifest.Source.RevisionID != source.RevisionID || len(published.Manifest.Files) != 3 {
		t.Fatal("publication lost its original project/revision/scope")
	}
	for _, file := range published.Manifest.Files {
		if !containsID(request.FileIDs, file.ID) || file.Content != files[file.Path].Content {
			t.Fatal("published an unselected or substituted file")
		}
	}
	var status project.WorldStatus
	call(f.p, "world.commit_status", project.WorldStatusRequest{AssetID: published.AssetID, CommitID: request.CommitID}, &status, 200, "")
	if status.State != "committed" || status.Result == nil || !reflect.DeepEqual(*status.Result, published) {
		t.Fatal("authoritative publication status mismatch")
	}
	forged := status
	forged.RequestHash = strings.Repeat("0", 64)
	candidate["registration_status"] = forged
	badChanges := contentChange()
	call(f.p, "project.commit", project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &badChanges}, nil, 400, project.ErrInvalid.Error())
	assertHead(head.RevisionID)
	candidate["registration_status"] = status
	association := contentChange()
	associate := project.CommitRequest{ProjectID: head.ProjectID, ExpectedRevisionID: head.RevisionID, CommitID: id(), Changes: &association}
	f.content.loseRetain.Store(true)
	call(f.p, "project.commit", associate, nil, 503, project.ErrDependency.Error())
	assertHead(head.RevisionID)
	f.restart()
	call(f.p, "project.commit", associate, &head, 200, "")
	var saved project.Result
	call(f.p, "project.commit", associate, &saved, 200, "")
	if saved.RevisionID != head.RevisionID || !reflect.DeepEqual(head.Manifest.Selections, selections) {
		t.Fatal("association changed current L2 selection or duplicated its revision")
	}
	for _, file := range head.Manifest.Files {
		if file.ID == domain.ID {
			data, err := f.content.Read(ctx, f.p, file.Content, project.Access{ProjectID: head.ProjectID, RevisionID: head.RevisionID}, project.MaxManifestBytes)
			if err != nil || string(data) != encode(root) {
				t.Fatal("stored project association differs from authoritative result", err)
			}
		}
	}
	// The fixed World version remains usable by another project.
	var other project.Result
	create.CommitID = id()
	call(f.p, "project.create", create, &other, 200, "")
	call(f.p, "project.use_asset", project.UseAssetRequest{ProjectID: other.ProjectID, ExpectedRevisionID: other.RevisionID, CommitID: id(), AssetID: published.AssetID, VersionID: published.VersionID, Purpose: "hall-lighting"}, &other, 200, "")
	if len(other.Manifest.AssetRefs) != 1 || other.Manifest.AssetRefs[0].VersionID != published.VersionID {
		t.Fatal("reuse did not pin the published L1 World version")
	}
}

func containsID(ids []string, wanted string) bool {
	for _, value := range ids {
		if value == wanted {
			return true
		}
	}
	return false
}
