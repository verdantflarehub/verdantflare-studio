package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/contentcache"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

const MediaCacheControl = "private, max-age=2592000, immutable"

type mediaRecord struct {
	Scope, Resource, Representation, InstallationID string
	Access                                          project.Access
	Version                                         project.ContentVersion
}
type MediaFile struct {
	File        *os.File
	Size        int64
	MIME, ETag  string
	CacheStatus string
}

func mediaHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (d *Dashboards) mediaCache() *contentcache.Cache {
	d.mediaOnce.Do(func() {
		dir := os.Getenv("STUDIO_MEDIA_CACHE_DIR")
		if dir == "" {
			dir = contentcache.DefaultDir("studio-media")
		}
		budget := int64(4 << 30)
		if n, e := strconv.ParseInt(os.Getenv("STUDIO_MEDIA_CACHE_BYTES"), 10, 64); e == nil && n >= 2*DashboardContentLimit {
			budget = n
		}
		d.media = contentcache.New(filepath.Join(dir, "source"), budget*3/4)
		d.mediaDerived = contentcache.New(filepath.Join(dir, "derived"), budget/4)
		d.mediaIndex = contentcache.New(filepath.Join(dir, "index"), 64<<20)
	})
	return d.media
}
func (d *Dashboards) mediaIndexCache() *contentcache.Cache { d.mediaCache(); return d.mediaIndex }
func mediaScope(ctx context.Context) string {
	v, _ := ctx.Value(dashboardCredentialsKey{}).(dashboardCredentials)
	return v.mediaScope
}
func mediaIndexKey(scope, resource, representation string) string {
	return "index:" + scope + ":" + resource + ":" + representation
}

// Stable media descriptors reuse artifact.read's existing grant. The optional
// representation is consumed here and is never forwarded to Artifact's API.
func (d *Dashboards) DescribeMedia(ctx context.Context, token, sessionID string, in DashboardContentInput, g DashboardContentGateway) Result {
	ctx, actor, launch, out := d.contentBegin(ctx, token, sessionID)
	if out.Status != 200 {
		return out
	}
	if g == nil {
		return contentFailure(503, out.RequestID)
	}
	scope := mediaScope(ctx)
	resource := mediaHash([]any{in.Ref, in.Access.ProjectID, in.Access.AssetID, in.Access.WorldDraftID})
	key := mediaIndexKey(scope, resource, in.Representation)
	data, err := d.mediaIndexCache().Get(ctx, key, 128<<10, func() ([]byte, error) {
		plain := in
		plain.Representation = ""
		plain.Mode = "metadata"
		result := d.ReadContent(ctx, token, sessionID, plain, g)
		if result.Status != 200 {
			return nil, mediaError{result}
		}
		var value struct {
			Version project.ContentVersion `json:"version"`
		}
		if json.Unmarshal(result.Data, &value) != nil || value.Version.Size > DashboardContentLimit {
			return nil, contentcache.ErrInvalid
		}
		record := mediaRecord{Scope: scope, Resource: resource, Representation: in.Representation, InstallationID: launch.InstallationID, Access: in.Access, Version: value.Version}
		if _, err := d.authorizeMedia(ctx, actor, record, g, out.RequestID); err != nil {
			return nil, err
		}
		return json.Marshal(record)
	})
	if err != nil {
		return mediaFailure(err, out.RequestID)
	}
	var record mediaRecord
	if json.Unmarshal(data, &record) != nil {
		return contentFailure(503, out.RequestID)
	}
	if _, err = d.authorizeMedia(ctx, actor, record, g, out.RequestID); err != nil {
		return mediaFailure(err, out.RequestID)
	}
	out.Data, _ = json.Marshal(map[string]any{"mode": "download", "version": record.Version, "content_path": "/studio/media/" + scope + "/" + resource + "/" + in.Representation})
	return out
}

type mediaError struct{ result Result }

func (e mediaError) Error() string { return "media unavailable" }
func mediaFailure(err error, requestID string) Result {
	if e, ok := err.(mediaError); ok {
		return e.result
	}
	return contentFailure(503, requestID)
}
func mediaCall(ctx context.Context, g DashboardContentGateway, p project.Principal, name string, args map[string]any, scope string) (json.RawMessage, error) {
	value, status, err := g.CallVerified(ctx, name, args, p, scope)
	if err != nil || status != 200 {
		return nil, mediaError{contentFailure(status, p.RequestID)}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		IsError bool            `json:"isError"`
		Content json.RawMessage `json:"structuredContent"`
	}
	if json.Unmarshal(data, &wrapper) != nil || len(wrapper.Content) == 0 {
		return nil, contentcache.ErrInvalid
	}
	if wrapper.IsError {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(wrapper.Content, &failure)
		statuses := map[string]int{"INVALID_ARGUMENT": 400, "PERMISSION_DENIED": 403, "NOT_FOUND": 404, "CONTENT_NOT_READY": 409, "UNSUPPORTED_MEDIA": 415, "TOO_LARGE": 413, "RATE_LIMITED": 429}
		return nil, mediaError{contentFailure(statuses[failure.Code], p.RequestID)}
	}
	return wrapper.Content, nil
}

// Membership and fixed-revision containment are checked on every network read.
// The Project service caches immutable manifests, not its current membership.
func (d *Dashboards) authorizeMedia(ctx context.Context, actor dashboard.Actor, r mediaRecord, g DashboardContentGateway, requestID string) (string, error) {
	if r.Scope != mediaScope(ctx) || r.Version.OrganizationID != actor.OrganizationID {
		return "", mediaError{contentFailure(403, requestID)}
	}
	p := project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: requestID}
	if r.Access.ProjectID != "" {
		data, err := mediaCall(ctx, g, p, "project.open", map[string]any{"project_id": r.Access.ProjectID, "revision_id": r.Access.RevisionID}, r.Access.ProjectID)
		if err != nil {
			return "", err
		}
		var opened struct {
			ProjectID  string `json:"project_id"`
			RevisionID string `json:"revision_id"`
			Manifest   struct {
				Files []project.File `json:"files"`
			} `json:"manifest"`
		}
		if json.Unmarshal(data, &opened) != nil || opened.ProjectID != r.Access.ProjectID || opened.RevisionID != r.Access.RevisionID {
			return "", contentcache.ErrInvalid
		}
		for _, f := range opened.Manifest.Files {
			if f.Content == r.Version.ContentRef {
				return f.ID, nil
			}
		}
		return "", mediaError{contentFailure(403, requestID)}
	}
	// Other fixed Artifact scopes retain the existing authority's current check.
	args := map[string]any{"mode": "metadata", "content_ref": r.Version.ContentRef, "access": r.Access}
	data, err := mediaCall(ctx, g, p, "artifact.read", args, "")
	if err != nil {
		return "", err
	}
	var v struct {
		Version project.ContentVersion `json:"version"`
	}
	if json.Unmarshal(data, &v) != nil || v.Version.ContentRef != r.Version.ContentRef || v.Version.SHA256 != r.Version.SHA256 || v.Version.OrganizationID != actor.OrganizationID {
		return "", contentcache.ErrInvalid
	}
	return "", nil
}

func (d *Dashboards) DownloadMedia(ctx context.Context, token, scope, resource, representation string, g DashboardContentGateway) (MediaFile, Result) {
	ctx, actor, out := d.begin(ctx, token)
	if out.Status != 200 {
		return MediaFile{}, out
	}
	if scope != mediaScope(ctx) || !contentHash.MatchString(scope) || !contentHash.MatchString(resource) || g == nil {
		return MediaFile{}, contentFailure(403, out.RequestID)
	}
	index, size, ok := d.mediaIndexCache().Lookup(mediaIndexKey(scope, resource, representation), 128<<10)
	if !ok {
		return MediaFile{}, contentFailure(404, out.RequestID)
	}
	data, err := io.ReadAll(io.NewSectionReader(index, 32, size))
	index.Close()
	var record mediaRecord
	if err != nil || json.Unmarshal(data, &record) != nil || record.Scope != scope || record.Resource != resource || record.Representation != representation {
		return MediaFile{}, contentFailure(503, out.RequestID)
	}
	if err = d.resolver.Authorize(ctx, actor, record.InstallationID, "artifact.read"); err != nil {
		return MediaFile{}, dashboardError(err, out.RequestID)
	}
	fileID, err := d.authorizeMedia(ctx, actor, record, g, out.RequestID)
	if err != nil {
		return MediaFile{}, mediaFailure(err, out.RequestID)
	}
	v := record.Version
	mt, _, _ := mime.ParseMediaType(v.MIME)
	thumbnail := strings.HasPrefix(representation, "thumbnail-") || strings.HasPrefix(representation, "poster-")
	max := DashboardContentLimit
	if thumbnail {
		max = 4 << 20
		mt = "image/webp"
	}
	contentKey := "body:" + mediaHash([]any{actor.OrganizationID, v.ContentRef, v.SHA256, representation})
	cache := d.mediaCache()
	if thumbnail {
		cache = d.mediaDerived
	}
	load := func(w io.Writer) error {
		p := project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: out.RequestID}
		if thumbnail {
			if record.Access.ProjectID == "" {
				if !strings.HasPrefix(representation, "thumbnail-") || v.Size > 32<<20 {
					return mediaError{contentFailure(415, out.RequestID)}
				}
				original := record
				original.Representation = "original-v1"
				_, err := d.mediaIndexCache().Get(ctx, mediaIndexKey(scope, resource, "original-v1"), 128<<10, func() ([]byte, error) { return json.Marshal(original) })
				if err != nil {
					return err
				}
				source, result := d.DownloadMedia(ctx, token, scope, resource, "original-v1", g)
				if result.Status != 200 {
					return mediaError{result}
				}
				defer source.File.Close()
				edge := 320
				if strings.Contains(representation, "640") {
					edge = 640
				}
				return encodeMediaThumbnail(ctx, io.NewSectionReader(source.File, 32, source.Size), edge, w)
			}
			if fileID == "" {
				return mediaError{contentFailure(415, out.RequestID)}
			}
			edge := 320
			if strings.Contains(representation, "640") {
				edge = 640
			}
			args := map[string]any{"project_id": record.Access.ProjectID, "revision_id": record.Access.RevisionID, "file_id": fileID, "edge": edge}
			if strings.HasPrefix(representation, "poster-") {
				args["preview"] = "poster"
			}
			data, err := mediaCall(ctx, g, p, "project.thumbnail", args, record.Access.ProjectID)
			if err != nil {
				return err
			}
			var thumb struct {
				MIME string `json:"mime"`
				Data string `json:"data"`
			}
			if json.Unmarshal(data, &thumb) != nil || thumb.MIME != "image/webp" || len(thumb.Data) > 6<<20 {
				return contentcache.ErrInvalid
			}
			b, err := base64.StdEncoding.DecodeString(thumb.Data)
			if err != nil || len(b) == 0 {
				return contentcache.ErrInvalid
			}
			_, err = w.Write(b)
			return err
		}
		q := url.Values{"store_id": {v.StoreID}, "artifact_id": {v.ArtifactID}}
		a := record.Access
		for k, value := range map[string]string{"project_id": a.ProjectID, "project_revision_id": a.RevisionID, "asset_id": a.AssetID, "asset_version_id": a.AssetVersionID, "world_draft_id": a.WorldDraftID, "world_draft_revision_id": a.WorldDraftRevisionID} {
			if value != "" {
				q.Set(k, value)
			}
		}
		resp, status, err := g.Transfer(ctx, p, "GET", "/v2/artifacts/"+v.VersionID+"/content", q.Encode(), nil, 0)
		if err != nil {
			return mediaError{contentFailure(status, out.RequestID)}
		}
		defer resp.Body.Close()
		actual, _, parseErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if status != 200 || resp.ContentLength != v.Size || actual != mt || parseErr != nil || resp.Header.Get("Content-Encoding") != "" {
			return contentcache.ErrInvalid
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(resp.Body, v.Size+1))
		if err != nil {
			return err
		}
		if n != v.Size || hex.EncodeToString(h.Sum(nil)) != v.SHA256 {
			return contentcache.ErrInvalid
		}
		return nil
	}
	f, length, hit := cache.Lookup(contentKey, max)
	if !hit {
		f, length, err = cache.File(ctx, contentKey, max, load)
	}

	if err != nil {
		return MediaFile{}, mediaFailure(err, out.RequestID)
	}
	var digest [32]byte
	if _, err = f.ReadAt(digest[:], 0); err != nil {
		f.Close()
		return MediaFile{}, contentFailure(503, out.RequestID)
	}
	// Original bytes were verified at insertion; cache files verify their own
	// digest on each lookup, including after a process restart.
	if !thumbnail && hex.EncodeToString(digest[:]) != v.SHA256 {
		f.Close()
		return MediaFile{}, contentFailure(503, out.RequestID)
	}
	cacheStatus := "miss"
	if hit {
		cacheStatus = "hit"
	}
	return MediaFile{File: f, Size: length, MIME: mt, CacheStatus: cacheStatus, ETag: "\"" + hex.EncodeToString(digest[:]) + "\""}, out
}

func validRepresentation(value string) bool {
	return slices.Contains([]string{"original-v1", "thumbnail-320-v1", "thumbnail-640-v1", "poster-320-v1", "poster-640-v1"}, value)
}

// Login refresh does not invalidate byte identities; permission policy changes do.
func mediaIdentityScope(identity identityContext) string {
	roles, scopes := slices.Clone(identity.Roles), slices.Clone(identity.Scopes)
	sort.Strings(roles)
	sort.Strings(scopes)
	return mediaHash([]any{identity.UserID, identity.OrganizationID, identity.StationID, identity.RevocationVersion, identity.PolicyVersion, slices.Compact(roles), slices.Compact(scopes)})
}
