package project

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type Permission struct {
	Action  string          `json:"action"`
	Source  *Source         `json:"source,omitempty"`
	Version *ContentVersion `json:"version,omitempty"`
	Owner   *RetentionOwner `json:"owner,omitempty"`
	Access  *Access         `json:"access,omitempty"`
}

func (s *Service) AuthorizeArtifact(ctx context.Context, p Principal, permission Permission) error {
	if !p.Valid() {
		return ErrForbidden
	}
	switch permission.Action {
	case "write":
		source := permission.Source
		if source == nil || permission.Version != nil || permission.Owner != nil || permission.Access != nil {
			return ErrForbidden
		}
		if source.Kind == "asset_manifest" {
			return s.authorizeWorldWrite(ctx, p, *source)
		}
		if source.Kind != "user_edit" && source.Kind != "user_import" && source.Kind != "legacy_import" {
			return ErrForbidden
		}
		return writable(ctx, s.db, p, source.ProjectID)
	case "read", "delete":
		v := permission.Version
		a := permission.Access
		if v == nil || a == nil || permission.Source != nil || permission.Owner != nil || !v.ContentRef.Valid() || v.OrganizationID != p.OrganizationID {
			return ErrForbidden
		}
		if permission.Action == "delete" {
			if *a != (Access{}) {
				return ErrForbidden
			}
			member, e := role(ctx, s.db, p, v.Source.ProjectID)
			if e != nil {
				return e
			}
			if member != "owner" {
				return ErrForbidden
			}
			return nil
		}
		if a.AssetID != "" || a.AssetVersionID != "" {
			return s.authorizeAssetRead(ctx, p, *v, *a)
		}
		if *a == (Access{}) {
			_, e := role(ctx, s.db, p, v.Source.ProjectID)
			return e
		}
		if !ValidID(a.ProjectID) || !ValidID(a.RevisionID) {
			return ErrForbidden
		}
		if _, e := role(ctx, s.db, p, a.ProjectID); e != nil {
			return e
		}
		m, ref, e := revision(ctx, s.db, p.OrganizationID, a.ProjectID, a.RevisionID)
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
	case "retain", "retain_content", "inspect_retention", "release":
		o := permission.Owner
		if o != nil && o.Kind == "asset_version" {
			return s.authorizeWorldRetention(ctx, p, permission)
		}
		if o == nil || o.Kind != "project_revision" || !ValidID(o.ID) || !ValidID(o.CommitID) || permission.Source != nil || permission.Access != nil {
			return ErrForbidden
		}
		if (permission.Action == "retain_content") != (permission.Version != nil) {
			return ErrForbidden
		}
		var id string
		e := s.db.QueryRow(ctx, "SELECT project_id::text FROM studio.project_commits WHERE organization_id=$1 AND revision_id=$2 AND commit_id=$3", p.OrganizationID, o.ID, o.CommitID).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrForbidden
		}
		if e != nil {
			return e
		}
		member, e := role(ctx, s.db, p, id)
		if e != nil {
			return e
		}
		c, e := loadCommit(ctx, s.db, p.OrganizationID, id, o.CommitID)
		if e != nil {
			return e
		}
		if p.SubjectID != c.Actor && member != "owner" {
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
			e = s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM studio.project_revisions WHERE revision_id=$1)", o.ID).Scan(&exists)
			if e != nil {
				return e
			}
			if exists {
				return ErrForbidden
			}
			return nil
		}
		if c.State != "preparing" && c.State != "committed" {
			return ErrForbidden
		}
		if c.Plan.ManifestRef == nil || len(c.Plan.Refs) == 0 {
			return ErrForbidden
		}
		if permission.Action == "retain" {
			return nil
		}
		if permission.Version.OrganizationID != p.OrganizationID {
			return ErrForbidden
		}
		for _, ref := range c.Plan.Refs {
			if ref == permission.Version.ContentRef {
				// A preparation plan fixes the set; it is not itself a content
				// grant. Recheck the actual source or a prior published revision.
				if _, err := role(ctx, s.db, p, permission.Version.Source.ProjectID); err == nil {
					return nil
				} else if !errors.Is(err, ErrForbidden) {
					return err
				}
				if c.Expected != "" {
					a := Access{ProjectID: id, RevisionID: c.Expected}
					if err := s.AuthorizeArtifact(ctx, p, Permission{Action: "read", Version: permission.Version, Access: &a}); err == nil {
						return nil
					} else if !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrNotFound) {
						return err
					}
				}
				return s.authorizeDependencyContent(ctx, p, c.Plan.Manifest.AssetRefs, ref)
			}
		}
		return ErrForbidden
	}
	return ErrForbidden
}

// AuthorityHandler is an internal-only endpoint. Its service credential is
// independent from the Gateway credential; user headers are never accepted.
func (s *Service) AuthorityHandler(token string) (http.Handler, error) {
	if len(token) < 32 || strings.ContainsAny(token, " \r\n\t") {
		return nil, ErrInvalid
	}
	expected := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		deny := func(status int) { w.WriteHeader(status); _, _ = w.Write([]byte(`{"allowed":false}`)) }
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if r.Method != "POST" || r.URL.RawQuery != "" || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			deny(403)
			return
		}
		var req struct {
			Principal  Principal  `json:"principal"`
			Permission Permission `json:"permission"`
		}
		if strictjson.Decode(r.Body, 1<<20, &req) != nil {
			deny(400)
			return
		}
		e := s.AuthorizeArtifact(r.Context(), req.Principal, req.Permission)
		if e != nil {
			if errors.Is(e, ErrForbidden) || errors.Is(e, ErrNotFound) {
				deny(403)
			} else {
				deny(503)
			}
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Allowed bool `json:"allowed"`
		}{true})
	}), nil
}
