package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

// NewRemote is startup wiring, not a WebView-callable Service method.
func NewRemote(remote *studioremote.Client) *Service {
	return &Service{remote: remote, workspaces: map[string]*workspace.Workspace{}}
}
func remoteFailure(err error) application.Result {
	switch {
	case errors.Is(err, studioremote.ErrDenied):
		return application.Error(401, "UNAUTHENTICATED")
	case errors.Is(err, studioremote.ErrChanged):
		return application.Error(409, "STUDIO_SESSION_CHANGED")
	case errors.Is(err, studioremote.ErrInvalid):
		return application.Error(400, "INVALID_STUDIO_REQUEST")
	default:
		return application.Error(503, "STUDIO_REMOTE_UNAVAILABLE")
	}
}
func (s *Service) remoteCall(ctx context.Context, path, method string, body []byte) application.Result {
	out, err := s.remote.Send(ctx, method, path, body)
	if err != nil {
		return remoteFailure(err)
	}
	if out.Status == 204 {
		out.Data = []byte(`{}`)
	}
	return application.Result{Status: out.Status, Data: json.RawMessage(out.Data), RequestID: out.RequestID}
}

// RemoteAssetHandler wraps Wails' in-process asset server. No local listener or
// service credential is needed; remote cookies remain exclusively in Go memory.
func RemoteAssetHandler(s *Service, fallback http.Handler) http.Handler {
	legacyMedia := newDesktopLegacyMedia(s)
	legacyBlender := newDesktopLegacyBlender(s)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == blenderWorkspacePrefix || strings.HasPrefix(r.URL.Path, blenderWorkspacePrefix+"/") || strings.HasPrefix(r.URL.Path, "/apps/blender/") {
			legacyBlender.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == legacyWorkspacePrefix || strings.HasPrefix(r.URL.Path, legacyWorkspacePrefix+"/") {
			legacyMedia.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/studio/") {
			fallback.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		failure := func(err error) {
			out := remoteFailure(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(out.Status)
			_, _ = w.Write(out.Data)
		}
		if s.remote == nil {
			failure(studioremote.ErrUnavailable)
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" || len(r.Header.Values("Authorization")) != 0 || len(r.Header.Values("Cookie")) != 0 || len(r.Header.Values("Content-Encoding")) != 0 {
			failure(studioremote.ErrInvalid)
			return
		}
		publicAsset := strings.HasPrefix(r.URL.Path, "/studio/instance-assets/") && studioremote.Allowed("GET", r.URL.Path) && r.Method == "GET"
		if site := r.Header.Get("Sec-Fetch-Site"); !publicAsset && site != "" && site != "same-origin" && site != "none" {
			failure(studioremote.ErrDenied)
			return
		}
		origins := r.Header.Values("Origin")
		if len(origins) > 1 {
			failure(studioremote.ErrDenied)
			return
		}
		if len(origins) == 1 && !publicAsset {
			u, err := url.Parse(origins[0])
			if err != nil || u.Host != r.Host || u.User != nil || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") {
				failure(studioremote.ErrDenied)
				return
			}
		}
		if strings.Contains(r.URL.Path, "/content/") {
			if r.Method != "GET" || r.ContentLength != 0 || r.Header.Get("Range") != "" {
				failure(studioremote.ErrInvalid)
				return
			}
			session, err := s.remote.Session()
			if err != nil {
				failure(err)
				return
			}
			out, err := session.Content(r.Context(), r.URL.Path)
			if err != nil {
				failure(err)
				return
			}
			defer out.Body.Close()
			if out.StatusCode != 200 {
				failure(studioremote.ErrUnavailable)
				return
			}
			w.Header().Set("Content-Type", out.Header.Get("Content-Type"))
			w.Header().Set("Content-Length", strconv.FormatInt(out.ContentLength, 10))
			w.Header().Set("Content-Disposition", "attachment")
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'")
			w.WriteHeader(200)
			_, _ = io.CopyN(w, out.Body, out.ContentLength)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+4097))
		if err != nil {
			failure(studioremote.ErrInvalid)
			return
		}
		out, err := s.remote.Send(r.Context(), r.Method, r.URL.Path, body)
		if err != nil {
			failure(err)
			return
		}
		w.Header().Set("X-Request-ID", out.RequestID)
		w.Header().Set("Content-Type", out.MIME)
		if publicAsset {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		}
		if publicAsset || strings.HasPrefix(r.URL.Path, "/studio/instance-workspaces/") && strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		} else if strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		}
		w.WriteHeader(out.Status)
		_, _ = w.Write(out.Data)
	})
}
