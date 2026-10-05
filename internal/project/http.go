package project

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type OpenRequest struct {
	ProjectID  string `json:"project_id"`
	RevisionID string `json:"revision_id,omitempty"`
}
type StatusRequest struct {
	ProjectID string `json:"project_id"`
	CommitID  string `json:"commit_id"`
}

// HTTPHandler exposes only the internal service protocol, not a public identity
// boundary. The Gateway must supply principals from a verified session.
func (s *Service) HTTPHandler(serviceToken, authorityToken string) (http.Handler, error) {
	if len(serviceToken) < 32 || strings.ContainsAny(serviceToken, " \r\n\t") || serviceToken == authorityToken {
		return nil, ErrInvalid
	}
	auth, e := s.AuthorityHandler(authorityToken)
	if e != nil {
		return nil, e
	}
	expected := sha256.Sum256([]byte("Bearer " + serviceToken))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/internal/v1/artifact/authorize" {
			auth.ServeHTTP(w, r)
			return
		}
		p := Principal{OrganizationID: r.Header.Get("X-Organization-Id"), SubjectID: r.Header.Get("X-User-Id"), RequestID: r.Header.Get("X-Request-Id")}
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		for _, header := range []string{"Authorization", "X-Organization-Id", "X-User-Id", "X-Request-Id"} {
			if len(r.Header.Values(header)) != 1 {
				httpError(w, p, ErrForbidden)
				return
			}
		}
		if !p.Valid() || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			httpError(w, p, ErrForbidden)
			return
		}
		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if r.Method != "POST" || r.URL.RawQuery != "" || err != nil || mt != "application/json" || r.Header.Get("Content-Encoding") != "" {
			httpError(w, p, ErrInvalid)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, MaxManifestBytes+1))
		if err != nil || len(data) > MaxManifestBytes {
			httpError(w, p, ErrInvalid)
			return
		}
		decode := func(v any) bool {
			if strictjson.Decode(bytes.NewReader(data), MaxManifestBytes, v) != nil {
				httpError(w, p, ErrInvalid)
				return false
			}
			return true
		}
		var result any
		switch r.URL.Path {
		case "/internal/v1/project/create":
			var req CreateRequest
			if !decode(&req) {
				return
			}
			result, err = s.Create(r.Context(), p, req)
		case "/internal/v1/project/list":
			var req ListRequest
			if !decode(&req) {
				return
			}
			result, err = s.List(r.Context(), p, req)
		case "/internal/v1/project/open":
			var req OpenRequest
			if !decode(&req) {
				return
			}
			result, err = s.Open(r.Context(), p, req.ProjectID, req.RevisionID)
		case "/internal/v1/project/commit":
			var req CommitRequest
			req, err = decodeCommit(data)
			if err == nil {
				result, err = s.Commit(r.Context(), p, req)
			}
		case "/internal/v1/project/commit_status":
			var req StatusRequest
			if !decode(&req) {
				return
			}
			result, err = s.Status(r.Context(), p, req.ProjectID, req.CommitID)
		case "/internal/v1/project/use_asset":
			var req UseAssetRequest
			if !decode(&req) {
				return
			}
			result, err = s.UseAsset(r.Context(), p, req)
		case "/internal/v1/world/register":
			var req WorldRegisterRequest
			if !decode(&req) {
				return
			}
			result, err = s.WorldRegister(r.Context(), p, req)
		case "/internal/v1/world/get":
			var req WorldGetRequest
			if !decode(&req) {
				return
			}
			result, err = s.WorldGet(r.Context(), p, req)
		case "/internal/v1/world/list":
			var req WorldListRequest
			if !decode(&req) {
				return
			}
			result, err = s.WorldList(r.Context(), p, req)
		case "/internal/v1/world/commit_status":
			var req WorldStatusRequest
			if !decode(&req) {
				return
			}
			result, err = s.WorldCommitStatus(r.Context(), p, req)
		case "/internal/v1/world/grant":
			var req WorldGrantRequest
			if !decode(&req) {
				return
			}
			result, err = s.WorldGrant(r.Context(), p, req)
		case "/internal/v1/world/revoke":
			var req WorldRevokeRequest
			if !decode(&req) {
				return
			}
			result, err = s.WorldRevoke(r.Context(), p, req)
		default:
			err = ErrNotFound
		}
		if err != nil {
			httpError(w, p, err)
			return
		}
		w.Header().Set("X-Request-Id", p.RequestID)
		_ = json.NewEncoder(w).Encode(result)
	}), nil
}
func decodeCommit(data []byte) (CommitRequest, error) {
	var request CommitRequest
	if len(data) > MaxManifestBytes || !utf8.Valid(data) {
		return request, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if uniqueJSON(d, 0) != nil {
		return request, ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return request, ErrInvalid
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil {
		return request, ErrInvalid
	}
	var manifest *Manifest
	if original, ok := envelope["manifest"]; ok {
		value, e := Decode(original)
		if e != nil {
			return request, ErrInvalid
		}
		manifest = &value
		// Extensions are arbitrary namespaced JSON; the manifest decoder already
		// validates them. Exclude this opaque object from fixed-envelope shape checks.
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(original, &fields)
		delete(fields, "extensions")
		envelope["manifest"], _ = json.Marshal(fields)
		data, _ = json.Marshal(envelope)
	}
	if strictjson.Decode(bytes.NewReader(data), MaxManifestBytes, &request) != nil {
		return request, ErrInvalid
	}
	if manifest != nil {
		request.Manifest = manifest
	}
	return request, nil
}
func httpError(w http.ResponseWriter, p Principal, err error) {
	status, code := 500, "INTERNAL"
	for _, v := range []struct {
		error
		status int
	}{{ErrInvalid, 400}, {ErrForbidden, 403}, {ErrNotFound, 404}, {ErrConflict, 409}, {ErrIdempotency, 409}, {ErrNotReady, 409}, {ErrDependency, 503}} {
		if errors.Is(err, v.error) {
			status, code = v.status, v.Error()
			break
		}
	}
	if !ValidID(p.RequestID) {
		p.RequestID = newID()
	}
	w.Header().Set("X-Request-Id", p.RequestID)
	w.WriteHeader(status)
	var details map[string]string
	var conflict *ConflictError
	if errors.As(err, &conflict) {
		details = map[string]string{"current_revision_id": conflict.CurrentRevisionID}
	}
	var worldConflict *WorldConflictError
	if errors.As(err, &worldConflict) {
		details = map[string]string{"current_asset_version_id": worldConflict.CurrentVersionID}
	}
	_ = json.NewEncoder(w).Encode(struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		RequestID string            `json:"request_id"`
		Details   map[string]string `json:"details,omitempty"`
	}{code, "Project request failed", p.RequestID, details})
}
