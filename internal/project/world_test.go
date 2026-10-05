package project_test

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestWorldVoiceDependencyProvenanceAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	request := createRequest()
	request.Name, request.Category = "Training source", "music"
	training, e := f.s.Create(ctx, f.p, request)
	if e != nil {
		t.Fatal(e)
	}
	// Opaque synthetic bytes only: this verifies provenance, not model execution.
	weights, e := f.content.Write(ctx, f.p, training.ProjectID, project.TextWrite{WriteID: id(), Text: "synthetic-model-weights", MIME: "application/octet-stream"})
	if e != nil {
		t.Fatal(e)
	}
	files := []project.FileUpdate{{Path: "voice/model.bin", Role: "voice-model-weights", Content: &weights.ContentRef}}
	training, e = f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: training.ProjectID, ExpectedRevisionID: training.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &files}})
	if e != nil {
		t.Fatal(e)
	}
	var modelFile project.File
	for _, file := range training.Manifest.Files {
		if file.Path == "voice/model.bin" {
			modelFile = file
		}
	}
	modelRequest := registration(training, modelFile)
	modelRequest.Name, modelRequest.AssetType = "Voice fixture", "voice-model"
	model, e := f.s.WorldRegister(ctx, f.p, modelRequest)
	if e != nil {
		t.Fatal(e)
	}
	request.CommitID, request.Name = id(), "Song using existing model"
	song, e := f.s.Create(ctx, f.p, request)
	if e != nil {
		t.Fatal(e)
	}
	song, e = f.s.UseAsset(ctx, f.p, project.UseAssetRequest{ProjectID: song.ProjectID, ExpectedRevisionID: song.RevisionID, CommitID: id(), AssetID: model.AssetID, VersionID: model.VersionID, Purpose: "singing-voice"})
	if e != nil {
		t.Fatal(e)
	}
	// Reusing an existing ContentRef must not invent a new training origin.
	song, e = f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: song.ProjectID, ExpectedRevisionID: song.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &files}})
	if e != nil {
		t.Fatal(e)
	}
	var entry, reused project.File
	for _, file := range song.Manifest.Files {
		if file.ID == song.Manifest.EntryDocumentID {
			entry = file
		}
		if file.Path == "voice/model.bin" {
			reused = file
		}
	}
	falseOrigin := registration(song, reused)
	falseOrigin.AssetType = "voice-model"
	if _, e = f.s.WorldRegister(ctx, f.p, falseOrigin); !errors.Is(e, project.ErrInvalid) {
		t.Fatal("invented training origin accepted", e)
	}
	falseOrigin.CommitID, falseOrigin.Relation = id(), "curated_in"
	curated, e := f.s.WorldRegister(ctx, f.p, falseOrigin)
	if e != nil {
		t.Fatal(e)
	}
	metadata, e := f.content.Metadata(ctx, f.p, curated.Manifest.Files[0].Content, project.Access{AssetID: curated.AssetID, AssetVersionID: curated.VersionID})
	if e != nil || metadata.Source.ProjectID != training.ProjectID || metadata.ContentRef != weights.ContentRef {
		t.Fatal("curation rewrote training source", e)
	}
	packageRequest := registration(song, entry)
	packageRequest.AssetType = "song-reference"
	packageRequest.DependsOn = &song.Manifest.AssetRefs
	pack, e := f.s.WorldRegister(ctx, f.p, packageRequest)
	if e != nil {
		t.Fatal(e)
	}
	refs, e := f.s.References(ctx, f.p, project.AssetRef{AssetID: pack.AssetID, VersionID: pack.VersionID, Purpose: "reuse"})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, ref := range refs {
		if ref == weights.ContentRef {
			found = true
		}
		if ref == training.ManifestRef || ref == song.ManifestRef {
			t.Fatal("dependency exposed private source manifest")
		}
	}
	if !found {
		t.Fatal("dependency closure omitted model weights")
	}
	guest := f.p
	guest.SubjectID = id()
	grant := func(asset string) {
		t.Helper()
		if _, e := f.s.WorldGrant(ctx, f.p, project.WorldGrantRequest{AssetID: asset, SubjectID: guest.SubjectID, Role: "reader"}); e != nil {
			t.Fatal(e)
		}
	}
	grant(pack.AssetID)
	if _, e = f.s.References(ctx, guest, project.AssetRef{AssetID: pack.AssetID, VersionID: pack.VersionID, Purpose: "reuse"}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("parent grant authorized private dependency", e)
	}
	grant(model.AssetID)
	if _, e = f.content.Metadata(ctx, guest, weights.ContentRef, project.Access{AssetID: pack.AssetID, AssetVersionID: pack.VersionID}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("parent scope exposed dependency bytes", e)
	}
	if _, e = f.content.Metadata(ctx, guest, weights.ContentRef, project.Access{AssetID: model.AssetID, AssetVersionID: model.VersionID}); e != nil {
		t.Fatal(e)
	}
	request.CommitID, request.Name = id(), "Consumer"
	target, e := f.s.Create(ctx, guest, request)
	if e != nil {
		t.Fatal(e)
	}
	use := project.UseAssetRequest{ProjectID: target.ProjectID, ExpectedRevisionID: target.RevisionID, CommitID: id(), AssetID: pack.AssetID, VersionID: pack.VersionID, Purpose: "reference"}
	// Revoke after Artifact retention succeeds, before the Studio publish transaction.
	f.content.afterRetain = func() {
		if _, e := f.s.WorldRevoke(ctx, f.p, project.WorldRevokeRequest{AssetID: model.AssetID, SubjectID: guest.SubjectID}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = f.s.UseAsset(ctx, guest, use); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("late revoked dependency published", e)
	}
	opened, e := f.s.Open(ctx, guest, target.ProjectID, "")
	if e != nil || opened.RevisionID != target.RevisionID {
		t.Fatal("rejected use changed head", e)
	}
	grant(model.AssetID)
	used, e := f.s.UseAsset(ctx, guest, use)
	if e != nil {
		t.Fatal("same commit could not recover", e)
	}
	var targetEntry project.File
	for _, file := range used.Manifest.Files {
		if file.ID == used.Manifest.EntryDocumentID {
			targetEntry = file
		}
	}
	nestedRequest := registration(used, targetEntry)
	nestedRequest.AssetType = "reference-document"
	nestedRequest.DependsOn = &used.Manifest.AssetRefs
	before := count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions")
	f.content.afterRetain = func() {
		if _, e := f.s.WorldRevoke(ctx, f.p, project.WorldRevokeRequest{AssetID: model.AssetID, SubjectID: guest.SubjectID}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = f.s.WorldRegister(ctx, guest, nestedRequest); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("late revoked nested dependency published to World", e)
	}
	if count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions") != before {
		t.Fatal("failed nested publication left a version")
	}
	grant(model.AssetID)
	if _, e = f.s.WorldRegister(ctx, guest, nestedRequest); e != nil {
		t.Fatal("World dependency grant recovery failed", e)
	}
}

func worldSource(t *testing.T, f *fixture) (project.Result, project.File, project.File) {
	t.Helper()
	ctx := context.Background()
	r := createRequest()
	r.Name = "Character source"
	source, e := f.s.Create(ctx, f.p, r)
	if e != nil {
		t.Fatal(e)
	}
	png, e := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jx1sAAAAASUVORK5CYII=")
	if e != nil {
		t.Fatal(e)
	}
	updates := []project.FileUpdate{}
	for _, name := range []string{"selected.png", "rejected.png"} {
		v, e := f.content.Write(ctx, f.p, source.ProjectID, project.TextWrite{WriteID: id(), Text: string(png), MIME: "image/png"})
		if e != nil {
			t.Fatal(e)
		}
		updates = append(updates, project.FileUpdate{Path: name, Role: "character-reference", Content: &v.ContentRef})
	}
	source, e = f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: source.ProjectID, ExpectedRevisionID: source.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &updates}})
	if e != nil {
		t.Fatal(e)
	}
	var selected, rejected project.File
	for _, file := range source.Manifest.Files {
		if file.Path == "selected.png" {
			selected = file
		}
		if file.Path == "rejected.png" {
			rejected = file
		}
	}
	return source, selected, rejected
}
func registration(source project.Result, file project.File) project.WorldRegisterRequest {
	return project.WorldRegisterRequest{CommitID: id(), SourceProjectID: source.ProjectID, SourceRevisionID: source.RevisionID, FileIDs: []string{file.ID}, Name: "Character reference", AssetType: "character-image", Subjects: []string{"Character"}, Relation: "produced_in", ReviewFileID: source.Manifest.EntryDocumentID}
}

func TestWorldIndependentGrantsAndTwoPinnedProjects(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source, selected, rejected := worldSource(t, f)
	r := registration(source, selected)
	asset, e := f.s.WorldRegister(ctx, f.p, r)
	if e != nil {
		t.Fatal(e)
	}
	otherOrganization := f.p
	otherOrganization.OrganizationID = id()
	if _, e = f.s.WorldGet(ctx, otherOrganization, project.WorldGetRequest{AssetID: asset.AssetID, VersionID: asset.VersionID}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("World access crossed organization boundary", e)
	}
	if asset.Manifest.Files[0].Content != selected.Content || asset.Manifest.Source.ProjectID != source.ProjectID {
		t.Fatal("World moved or rebound original content")
	}
	guest := f.p
	guest.SubjectID = id()
	if list, e := f.s.WorldList(ctx, guest, project.WorldListRequest{}); e != nil || len(list.Items) != 0 {
		t.Fatal("ungranted asset listed", e)
	}
	if _, e = f.s.WorldGrant(ctx, f.p, project.WorldGrantRequest{AssetID: asset.AssetID, SubjectID: guest.SubjectID, Role: "reader"}); e != nil {
		t.Fatal(e)
	}
	got, e := f.s.WorldGet(ctx, guest, project.WorldGetRequest{AssetID: asset.AssetID, VersionID: asset.VersionID})
	if e != nil || got.ManifestRef != asset.ManifestRef {
		t.Fatal("asset reader requires source membership", e)
	}
	access := project.Access{AssetID: asset.AssetID, AssetVersionID: asset.VersionID}
	if _, e = f.content.Read(ctx, guest, selected.Content, access, 1<<20); e != nil {
		t.Fatal(e)
	}
	for _, ref := range []project.ContentRef{rejected.Content, source.Manifest.Files[0].Content, source.ManifestRef} {
		if _, e = f.content.Metadata(ctx, guest, ref, access); !errors.Is(e, project.ErrForbidden) {
			t.Fatal("asset grant exposed private source content", e)
		}
	}
	if _, e = f.content.Metadata(ctx, guest, selected.Content, project.Access{}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("content ref became a bearer capability", e)
	}
	projects := []project.Result{}
	for _, name := range []string{"Dance A", "Dance B"} {
		req := createRequest()
		req.Name = name
		req.Category = "video"
		p, e := f.s.Create(ctx, guest, req)
		if e != nil {
			t.Fatal(e)
		}
		p, e = f.s.UseAsset(ctx, guest, project.UseAssetRequest{ProjectID: p.ProjectID, ExpectedRevisionID: p.RevisionID, CommitID: id(), AssetID: asset.AssetID, VersionID: asset.VersionID, Purpose: "performer"})
		if e != nil {
			t.Fatal(e)
		}
		projects = append(projects, p)
	}
	update := r
	update.CommitID = id()
	update.AssetID = asset.AssetID
	update.ExpectedVersionID = asset.VersionID
	update.Name = "Character reference v2"
	newer, e := f.s.WorldRegister(ctx, f.p, update)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range projects {
		opened, e := f.s.Open(ctx, guest, p.ProjectID, "")
		if e != nil || opened.Manifest.AssetRefs[0].VersionID != asset.VersionID {
			t.Fatal("asset head silently upgraded a project", e)
		}
	}
	upgraded, e := f.s.UseAsset(ctx, guest, project.UseAssetRequest{ProjectID: projects[0].ProjectID, ExpectedRevisionID: projects[0].RevisionID, CommitID: id(), AssetID: asset.AssetID, VersionID: newer.VersionID, Purpose: "performer"})
	if e != nil || len(upgraded.Manifest.AssetRefs) != 1 || upgraded.Manifest.AssetRefs[0].VersionID != newer.VersionID {
		t.Fatal("explicit upgrade failed", e)
	}
	if _, e = f.s.WorldRevoke(ctx, f.p, project.WorldRevokeRequest{AssetID: asset.AssetID, SubjectID: guest.SubjectID}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.content.Metadata(ctx, guest, selected.Content, access); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("revoked asset reader accessed bytes", e)
	}
	if _, e = f.content.Metadata(ctx, guest, selected.Content, project.Access{ProjectID: projects[1].ProjectID, RevisionID: projects[1].RevisionID}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("Project asset_refs bypassed independent grants", e)
	}
	if _, e = f.s.WorldGrant(ctx, f.p, project.WorldGrantRequest{AssetID: asset.AssetID, SubjectID: guest.SubjectID, Role: "reader"}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(ctx, "DELETE FROM studio.project_members WHERE organization_id=$1 AND project_id=$2", f.p.OrganizationID, source.ProjectID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.WorldGet(ctx, guest, project.WorldGetRequest{AssetID: asset.AssetID, VersionID: asset.VersionID}); e != nil {
		t.Fatal("source membership controlled independent asset grant", e)
	}
	if _, e = f.s.Open(ctx, guest, source.ProjectID, ""); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("World grant created project membership")
	}
	if n := count(t, f.artDB, "SELECT count(*) FROM station.artifact_retention_owners WHERE owner_kind='asset_version' AND released_at IS NULL"); n != 2 {
		t.Fatal("missing durable asset retention", n)
	}
	if _, e = f.db.Exec(ctx, "DELETE FROM studio.world_asset_versions"); e == nil {
		t.Fatal("published asset versions mutable")
	}
}

func TestWorldReaderPublicationAndRecoverableCommit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source, selected, _ := worldSource(t, f)
	publisher := f.p
	publisher.SubjectID = id()
	if _, e := f.db.Exec(ctx, "INSERT INTO studio.project_members(organization_id,project_id,subject_id,role) VALUES($1,$2,$3,'reader')", f.p.OrganizationID, source.ProjectID, publisher.SubjectID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.content.Write(ctx, publisher, source.ProjectID, project.TextWrite{WriteID: id(), Text: "cannot edit source", MIME: "text/plain"}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("source reader was granted project writes", e)
	}
	r := registration(source, selected)
	f.content.loseWrite.Store(true)
	if _, e := f.s.WorldRegister(ctx, publisher, r); !errors.Is(e, project.ErrDependency) {
		t.Fatal("lost World manifest result not surfaced", e)
	}
	if list, e := f.s.WorldList(ctx, publisher, project.WorldListRequest{}); e != nil || len(list.Items) != 0 {
		t.Fatal("unpublished asset listed", e)
	}
	f.restart()
	f.content.loseRetain.Store(true)
	if _, e := f.s.WorldRegister(ctx, publisher, r); !errors.Is(e, project.ErrDependency) {
		t.Fatal("lost retain result not recoverable", e)
	}
	if count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions") != 0 {
		t.Fatal("published incomplete asset")
	}
	asset, e := f.s.WorldRegister(ctx, publisher, r)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.s.WorldRegister(ctx, publisher, r)
	if e != nil || again.VersionID != asset.VersionID || again.ManifestRef != asset.ManifestRef {
		t.Fatal("retry changed World identity", e)
	}
	metadata, e := f.content.Metadata(ctx, publisher, asset.ManifestRef, project.Access{AssetID: asset.AssetID, AssetVersionID: asset.VersionID})
	if e != nil || metadata.Source.Kind != "asset_manifest" || metadata.Source.ProjectID != "" || metadata.Source.AssetVersionID != asset.VersionID {
		t.Fatal("manifest faked source-project edit", e)
	}
	r.Name = "changed same request"
	if _, e = f.s.WorldRegister(ctx, publisher, r); !errors.Is(e, project.ErrIdempotency) {
		t.Fatal("changed World declaration accepted", e)
	}
	owner := project.RetentionOwner{Kind: "asset_version", ID: asset.VersionID, CommitID: r.CommitID}
	if e = f.s.AuthorizeArtifact(ctx, publisher, project.Permission{Action: "release", Owner: &owner}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("live asset retention releasable")
	}
	if _, e = f.s.WorldGrant(ctx, f.p, project.WorldGrantRequest{AssetID: asset.AssetID, SubjectID: id(), Role: "editor"}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("source owner controlled World grants")
	}
}

func TestWorldFaultRollbackAndConcurrentVersions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source, selected, _ := worldSource(t, f)
	r := registration(source, selected)
	_, e := f.db.Exec(ctx, `CREATE FUNCTION studio.test_world_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='world.publish' THEN RAISE EXCEPTION 'injected failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_world_failure BEFORE INSERT ON studio.studio_events FOR EACH ROW EXECUTE FUNCTION studio.test_world_failure()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.WorldRegister(ctx, f.p, r); e == nil {
		t.Fatal("audit failure did not roll back")
	}
	if count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions") != 0 {
		t.Fatal("World revision survived failed publication")
	}
	if _, e = f.db.Exec(ctx, "DROP TRIGGER test_world_failure ON studio.studio_events"); e != nil {
		t.Fatal(e)
	}
	base, e := f.s.WorldRegister(ctx, f.p, r)
	if e != nil {
		t.Fatal(e)
	}
	type outcome struct {
		r project.WorldRegisterRequest
		e error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"asset update A", "asset update B"} {
		next := r
		next.CommitID = id()
		next.AssetID = base.AssetID
		next.ExpectedVersionID = base.VersionID
		next.Name = name
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.WorldRegister(ctx, f.p, next); results <- outcome{next, e} }()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for o := range results {
		if o.e == nil {
			wins++
		} else if errors.Is(o.e, project.ErrConflict) {
			conflicts++
			state, e := f.s.WorldCommitStatus(ctx, f.p, project.WorldStatusRequest{AssetID: base.AssetID, CommitID: o.r.CommitID})
			if e != nil || state.State != "conflict" || state.CurrentVersionID == "" {
				t.Fatal("conflict not inspectable", e)
			}
		} else {
			t.Fatal(o.e)
		}
	}
	if wins != 1 || conflicts != 1 || count(t, f.db, "SELECT count(*) FROM studio.world_asset_versions") != 2 {
		t.Fatal("World CAS did not serialize publication")
	}
}
