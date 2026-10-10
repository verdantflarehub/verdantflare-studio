package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var blenderUploadPath = regexp.MustCompile(`^/artifact/uploads/([0-9a-f-]{36})/(content|commit)$`)
var blenderVersionPath = regexp.MustCompile(`^/artifact/versions/([0-9a-f-]{36})(/content)?$`)

// BlenderContentHandler is mounted ONLY on the private content listener, never
// on the public Gin router. It grants no MCP route or arbitrary target URL.
func (s *Server) BlenderContentHandler() http.Handler {
	return blenderContentHandler(func(domain string) (string, bool) {
		if s.mcpGateway == nil {
			return "", false
		}
		return s.mcpGateway.ServiceEndpoint(domain)
	})
}

func blenderContentHandler(resolve func(string) (string, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		secret := os.Getenv("STUDIO_BLENDER_CONTENT_TOKEN")
		actual, expected := sha256.Sum256([]byte(r.Header.Get("Authorization"))), sha256.Sum256([]byte("Bearer "+secret))
		p := project.Principal{SubjectID: r.Header.Get("X-User-Id"), OrganizationID: r.Header.Get("X-Organization-Id"), RequestID: r.Header.Get("X-Request-Id")}
		for _, h := range []string{"Authorization", "X-User-Id", "X-Organization-Id", "X-Request-Id"} {
			if len(r.Header.Values(h)) != 1 {
				http.Error(w, "Authentication required", 401)
				return
			}
		}
		if secret == "" || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 || !p.Valid() {
			http.Error(w, "Authentication required", 401)
			return
		}
		if strings.Contains(r.URL.EscapedPath(), "%") || r.Header.Get("Content-Encoding") != "" {
			http.Error(w, "Invalid path or encoding", 400)
			return
		}
		domain, path, token := "", "", ""
		switch {
		case r.Method == "POST" && r.URL.Path == "/runtime/options":
			domain, path, token = "station-core", "/internal/v1/blender/options", os.Getenv("STUDIO_BLENDER_CONTROL_TOKEN")
		case r.Method == "POST" && r.URL.Path == "/runtime/instances/create":
			domain, path, token = "station-core", "/internal/v1/blender/create", os.Getenv("STUDIO_BLENDER_CONTROL_TOKEN")
		case r.Method == "POST" && r.URL.Path == "/runtime/instances/start":
			domain, path, token = "station-core", "/internal/v1/blender/start", os.Getenv("STUDIO_BLENDER_CONTROL_TOKEN")
		case r.Method == "POST" && r.URL.Path == "/runtime/instances/stop":
			domain, path, token = "station-core", "/internal/v1/blender/stop", os.Getenv("STUDIO_BLENDER_CONTROL_TOKEN")
		case r.Method == "POST" && r.URL.Path == "/runtime/instances/destroy":
			domain, path, token = "station-core", "/internal/v1/blender/destroy", os.Getenv("STUDIO_BLENDER_CONTROL_TOKEN")
		case r.Method == "POST" && r.URL.Path == "/runtime/instances/access":
			domain, path, token = "station-core", "/internal/v1/blender/access", os.Getenv("STUDIO_BLENDER_CONTROL_TOKEN")
		case r.Method == "POST" && (r.URL.Path == "/project/create" || r.URL.Path == "/project/list" || r.URL.Path == "/project/open" || r.URL.Path == "/project/commit" || r.URL.Path == "/project/commit_status"):
			domain, path, token = "project", "/internal/v1"+r.URL.Path, os.Getenv("STUDIO_PROJECT_SERVICE_TOKEN")
		case r.Method == "POST" && r.URL.Path == "/artifact/uploads":
			domain, path, token = "artifact", "/v2/artifacts/uploads", os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN")
		default:
			if m := blenderUploadPath.FindStringSubmatch(r.URL.Path); m != nil && project.ValidID(m[1]) && ((m[2] == "content" && r.Method == "PUT") || (m[2] == "commit" && r.Method == "POST")) {
				domain, path, token = "artifact", "/v2/artifacts/uploads/"+m[1]+"/"+m[2], os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN")
			} else if m := blenderVersionPath.FindStringSubmatch(r.URL.Path); m != nil && project.ValidID(m[1]) && r.Method == "GET" {
				domain, path, token = "artifact", "/v2/artifacts/"+m[1]+m[2], os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN")
			}
		}
		if domain == "" {
			http.Error(w, "Route denied", 404)
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			http.Error(w, "Invalid query", 400)
			return
		}
		for k, v := range q {
			if r.Method != "GET" || (k != "store_id" && k != "artifact_id" && k != "project_id" && k != "project_revision_id") || len(v) != 1 || !project.ValidID(v[0]) {
				http.Error(w, "Invalid query", 400)
				return
			}
		}
		if r.Method == "GET" && (q.Get("store_id") == "" || q.Get("artifact_id") == "" || q.Get("project_id") == "") {
			http.Error(w, "Content scope required", 400)
			return
		}
		if token == "" {
			http.Error(w, "Service unavailable", 503)
			return
		}
		endpoint, ok := "", false
		expectedPath := "/mcp"
		if domain == "station-core" {
			endpoint, ok, expectedPath = strings.TrimSuffix(os.Getenv("STATION_CORE_URL"), "/"), true, ""
		} else {
			endpoint, ok = resolve(domain)
		}
		u, err := url.Parse(endpoint)
		if !ok || err != nil || u.Host == "" || u.User != nil || u.Path != expectedPath || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			http.Error(w, "Service unavailable", 503)
			return
		}
		u.Path = path
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.ResponseHeaderTimeout = 30 * time.Second
		defer transport.CloseIdleConnections()
		proxy := &httputil.ReverseProxy{
			Transport: transport,
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL = u
				pr.Out.URL.RawQuery = q.Encode()
				pr.Out.Host = u.Host
				pr.Out.Header = make(http.Header)
				pr.Out.Header.Set("Authorization", "Bearer "+token)
				pr.Out.Header.Set("X-User-Id", p.SubjectID)
				pr.Out.Header.Set("X-Organization-Id", p.OrganizationID)
				pr.Out.Header.Set("X-Request-Id", p.RequestID)
				pr.Out.Header.Set("Content-Type", "application/json")
				if r.Method == "PUT" {
					pr.Out.Header.Set("Content-Type", "application/octet-stream")
				}
			},
			ModifyResponse: func(resp *http.Response) error {
				if resp.StatusCode >= 300 && resp.StatusCode < 400 {
					return errors.New("content redirect denied")
				}
				headers := make(http.Header)
				for _, name := range []string{"Content-Type", "Content-Length", "Content-Disposition"} {
					if value := resp.Header.Get(name); value != "" {
						headers.Set(name, value)
					}
				}
				headers.Set("Cache-Control", "no-store")
				headers.Set("X-Content-Type-Options", "nosniff")
				resp.Header = headers
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				http.Error(w, "Content service unavailable", 502)
			},
		}
		limit := int64(512 << 20)
		if domain == "station-core" {
			limit = 16 << 10
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		proxy.ServeHTTP(w, r)
	})
}
