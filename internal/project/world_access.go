package project

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// References resolves the exact dependency closure without reading or granting
// access to source-project manifests. Retention verifies physical bytes later.
func (s *Service) References(ctx context.Context, p Principal, a AssetRef) ([]ContentRef, error) {
	if !p.Valid() || !ValidID(a.AssetID) || !ValidID(a.VersionID) || !validText(a.Purpose) {
		return nil, ErrInvalid
	}
	type node struct {
		ref   AssetRef
		depth int
	}
	queue := []node{{a, 0}}
	seen := map[WorldGetRequest]bool{}
	refs := []ContentRef{}
	for len(queue) > 0 {
		n := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		key := WorldGetRequest{AssetID: n.ref.AssetID, VersionID: n.ref.VersionID}
		if n.depth > 32 {
			return nil, ErrInvalid
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(seen) > 10000 {
			return nil, ErrInvalid
		}
		if _, e := assetRole(ctx, s.db, p, key.AssetID); e != nil {
			return nil, e
		}
		m, ref, e := assetVersion(ctx, s.db, p.OrganizationID, key.AssetID, key.VersionID)
		if e != nil {
			return nil, e
		}
		if m.Validate() != nil || !ref.Valid() {
			return nil, ErrNotReady
		}
		refs = append(refs, ref)
		for _, f := range m.Files {
			refs = append(refs, f.Content)
		}
		refs = uniqueRefs(refs)
		if len(refs) > MaxFiles {
			return nil, ErrInvalid
		}
		for _, d := range m.DependsOn {
			queue = append(queue, node{d, n.depth + 1})
		}
	}
	return refs, nil
}
func (s *Service) authorizeAssetRead(ctx context.Context, p Principal, v ContentVersion, a Access) error {
	if a.ProjectID != "" || a.RevisionID != "" || !ValidID(a.AssetID) || !ValidID(a.AssetVersionID) || v.OrganizationID != p.OrganizationID || !v.ContentRef.Valid() {
		return ErrForbidden
	}
	if _, e := assetRole(ctx, s.db, p, a.AssetID); e != nil {
		return e
	}
	m, ref, e := assetVersion(ctx, s.db, p.OrganizationID, a.AssetID, a.AssetVersionID)
	if e != nil {
		return e
	}
	if ref == v.ContentRef {
		return nil
	}
	for _, f := range m.Files {
		if f.Content == v.ContentRef {
			return nil
		}
	}
	return ErrForbidden
}
func (s *Service) authorizeWorldWrite(ctx context.Context, p Principal, source Source) error {
	if source.Kind != "asset_manifest" || !ValidID(source.AssetID) || !ValidID(source.AssetVersionID) || source.ProjectID != "" || source.ServiceID != "" || source.RunID != "" || source.TaskID != "" || source.Original != nil {
		return ErrForbidden
	}
	if e := assetWritable(ctx, s.db, p, source.AssetID); e != nil {
		return e
	}
	var commit string
	e := s.db.QueryRow(ctx, "SELECT commit_id::text FROM studio.world_commits WHERE organization_id=$1 AND asset_id=$2 AND version_id=$3", p.OrganizationID, source.AssetID, source.AssetVersionID).Scan(&commit)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if e != nil {
		return e
	}
	c, e := loadWorldCommit(ctx, s.db, p.OrganizationID, source.AssetID, commit)
	if e != nil {
		return e
	}
	if c.Actor != p.SubjectID || (c.State != "preparing" && c.State != "committed") {
		return ErrForbidden
	}
	_, e = role(ctx, s.db, p, c.Plan.Manifest.Source.ProjectID)
	return e
}
func (s *Service) authorizeWorldRetention(ctx context.Context, p Principal, permission Permission) error {
	o := permission.Owner
	if o == nil || o.Kind != "asset_version" || !ValidID(o.ID) || !ValidID(o.CommitID) || permission.Source != nil || permission.Access != nil || (permission.Action == "retain_content") != (permission.Version != nil) {
		return ErrForbidden
	}
	var asset string
	e := s.db.QueryRow(ctx, "SELECT asset_id::text FROM studio.world_commits WHERE organization_id=$1 AND version_id=$2 AND commit_id=$3", p.OrganizationID, o.ID, o.CommitID).Scan(&asset)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if e != nil {
		return e
	}
	member, e := assetRole(ctx, s.db, p, asset)
	if e != nil {
		return e
	}
	c, e := loadWorldCommit(ctx, s.db, p.OrganizationID, asset, o.CommitID)
	if e != nil {
		return e
	}
	if c.Actor != p.SubjectID && member != "owner" {
		return ErrForbidden
	}
	if permission.Action == "inspect_retention" {
		return nil
	}
	if member != "owner" && member != "editor" {
		return ErrForbidden
	}
	if permission.Action == "release" {
		if c.State != "failed" && c.State != "conflict" {
			return ErrForbidden
		}
		var exists bool
		e = s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM studio.world_asset_versions WHERE version_id=$1)", o.ID).Scan(&exists)
		if e != nil {
			return e
		}
		if exists {
			return ErrForbidden
		}
		return nil
	}
	if (c.State != "preparing" && c.State != "committed") || c.Plan.ManifestRef == nil || len(c.Plan.Refs) == 0 {
		return ErrForbidden
	}
	if permission.Action == "retain" {
		return nil
	}
	if permission.Action != "retain_content" || permission.Version.OrganizationID != p.OrganizationID {
		return ErrForbidden
	}
	ref := permission.Version.ContentRef
	found := false
	for _, r := range c.Plan.Refs {
		if r == ref {
			found = true
			break
		}
	}
	if !found {
		return ErrForbidden
	}
	if ref == *c.Plan.ManifestRef {
		source := permission.Version.Source
		if source.Kind != "asset_manifest" || source.AssetID != asset || source.AssetVersionID != c.Version {
			return ErrForbidden
		}
		return nil
	}
	isSource := ref == c.Plan.SourceRef
	for _, f := range c.Plan.Manifest.Files {
		if f.Content == ref {
			isSource = true
		}
	}
	if isSource {
		a := Access{ProjectID: c.Plan.Manifest.Source.ProjectID, RevisionID: c.Plan.Manifest.Source.RevisionID}
		return s.AuthorizeArtifact(ctx, p, Permission{Action: "read", Version: permission.Version, Access: &a})
	}
	return s.authorizeDependencyContent(ctx, p, c.Plan.Manifest.DependsOn, ref)
}
func (s *Service) authorizeDependencyContent(ctx context.Context, p Principal, assets []AssetRef, ref ContentRef) error {
	found := false
	for _, a := range assets {
		refs, e := s.References(ctx, p, a)
		if e != nil {
			return e
		}
		for _, r := range refs {
			if r == ref {
				found = true
			}
		}
	}
	if found {
		return nil
	}
	return ErrForbidden
}
func (s *Service) WorldGrant(ctx context.Context, p Principal, r WorldGrantRequest) (WorldGrantResult, error) {
	if r.Role != "reader" && r.Role != "editor" {
		return WorldGrantResult{}, ErrInvalid
	}
	return s.setWorldGrant(ctx, p, r.AssetID, r.SubjectID, r.Role)
}
func (s *Service) WorldRevoke(ctx context.Context, p Principal, r WorldRevokeRequest) (WorldGrantResult, error) {
	return s.setWorldGrant(ctx, p, r.AssetID, r.SubjectID, "none")
}
func (s *Service) setWorldGrant(ctx context.Context, p Principal, asset, subject, wanted string) (WorldGrantResult, error) {
	result := WorldGrantResult{AssetID: asset, SubjectID: subject, Role: wanted}
	if !p.Valid() || !ValidID(asset) || !ValidID(subject) {
		return result, ErrInvalid
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(context.Background())
	var actor string
	e = tx.QueryRow(ctx, "SELECT role FROM studio.world_asset_grants WHERE organization_id=$1 AND asset_id=$2 AND subject_id=$3 FOR SHARE", p.OrganizationID, asset, p.SubjectID).Scan(&actor)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && actor != "owner" {
		return result, ErrForbidden
	}
	if e != nil {
		return result, e
	}
	var owner, head string
	e = tx.QueryRow(ctx, "SELECT owner_id::text,COALESCE(head_version_id::text,'') FROM studio.world_assets WHERE organization_id=$1 AND asset_id=$2", p.OrganizationID, asset).Scan(&owner, &head)
	if e != nil {
		return result, e
	}
	if owner == subject {
		return result, ErrInvalid
	}
	if head == "" {
		return result, ErrNotReady
	}
	if e = advisory(ctx, tx, "asset-grant:"+p.OrganizationID+":"+asset+":"+subject); e != nil {
		return result, e
	}
	var previous string
	e = tx.QueryRow(ctx, "SELECT role FROM studio.world_asset_grants WHERE organization_id=$1 AND asset_id=$2 AND subject_id=$3", p.OrganizationID, asset, subject).Scan(&previous)
	if errors.Is(e, pgx.ErrNoRows) {
		previous = "none"
	} else if e != nil {
		return result, e
	}
	if previous == wanted {
		return result, tx.Commit(ctx)
	}
	if wanted == "none" {
		_, e = tx.Exec(ctx, "DELETE FROM studio.world_asset_grants WHERE organization_id=$1 AND asset_id=$2 AND subject_id=$3", p.OrganizationID, asset, subject)
	} else {
		_, e = tx.Exec(ctx, "INSERT INTO studio.world_asset_grants(organization_id,asset_id,subject_id,role) VALUES($1,$2,$3,$4) ON CONFLICT(organization_id,asset_id,subject_id) DO UPDATE SET role=EXCLUDED.role", p.OrganizationID, asset, subject, wanted)
	}
	if e != nil {
		return result, e
	}
	if e = audit(ctx, tx, p, "world.grant."+wanted, asset); e != nil {
		return result, e
	}
	return result, tx.Commit(ctx)
}
func (s *Service) UseAsset(ctx context.Context, p Principal, r UseAssetRequest) (Result, error) {
	if !ValidID(r.AssetID) || !ValidID(r.VersionID) || !ValidID(r.ExpectedRevisionID) || !ValidID(r.CommitID) || !validText(r.Purpose) {
		return Result{}, ErrInvalid
	}
	if e := writable(ctx, s.db, p, r.ProjectID); e != nil {
		return Result{}, e
	}
	previous, _, e := revision(ctx, s.db, p.OrganizationID, r.ProjectID, r.ExpectedRevisionID)
	if e != nil {
		return Result{}, e
	}
	refs := []AssetRef{}
	for _, a := range previous.AssetRefs {
		if a.AssetID == r.AssetID && a.Purpose == r.Purpose {
			continue
		}
		refs = append(refs, a)
	}
	refs = append(refs, AssetRef{AssetID: r.AssetID, VersionID: r.VersionID, Purpose: r.Purpose})
	return s.Commit(ctx, p, CommitRequest{ProjectID: r.ProjectID, ExpectedRevisionID: r.ExpectedRevisionID, CommitID: r.CommitID, Changes: &Changes{AssetRefs: &refs}})
}
