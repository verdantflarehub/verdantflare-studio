package web

import (
	"context"
	"io"
	"mime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/preinstalled"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type PreinstalledService interface {
	Get(context.Context, string) application.Result
	Execute(context.Context, string, preinstalled.Intent) application.Result
}

func (s *Server) PreinstalledStore() preinstalled.Store {
	return &preinstalled.EtcdStore{Client: func() clientv3.KV {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.etcd == nil {
			return nil
		}
		return s.etcd
	}}
}

// EnablePreinstalled uses the same server-owned Cookie identity for browser and
// desktop's remote Studio adapter. No App-controlled token or route is accepted.
func (s *Server) EnablePreinstalled(r *gin.Engine, service PreinstalledService) {
	result := func(c *gin.Context, out application.Result) {
		c.Header("X-Request-ID", out.RequestID)
		c.Data(out.Status, "application/json", out.Data)
	}
	gate := func(c *gin.Context) (string, bool) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		req := c.Request
		fail := func(status int, code string) (string, bool) {
			result(c, application.Error(status, code))
			return "", false
		}
		if req.URL.RawQuery != "" || req.URL.ForceQuery || strings.Contains(req.URL.EscapedPath(), "%") || strings.Contains(req.URL.Path, "\\") || len(req.Header.Values("Authorization")) != 0 || len(req.Header.Values("Content-Encoding")) != 0 {
			return fail(400, "INVALID_PREINSTALLED_REQUEST")
		}
		origins := req.Header.Values("Origin")
		if len(origins) > 1 || (len(origins) == 1 && origins[0] != s.origin) || (req.Method != "GET" && len(origins) != 1) {
			return fail(403, "PREINSTALLED_DENIED")
		}
		if sites := req.Header.Values("Sec-Fetch-Site"); len(sites) > 1 || (len(sites) == 1 && sites[0] != "same-origin" && sites[0] != "none") {
			return fail(403, "PREINSTALLED_DENIED")
		}
		if req.Method == "POST" {
			value, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
			if len(req.Header.Values("Content-Type")) != 1 || err != nil || value != "application/json" {
				return fail(400, "INVALID_PREINSTALLED_REQUEST")
			}
		} else {
			body, err := io.ReadAll(io.LimitReader(req.Body, 1))
			if err != nil || len(body) != 0 {
				return fail(400, "INVALID_PREINSTALLED_REQUEST")
			}
		}
		cookies := req.CookiesNamed(cookieName)
		if len(cookies) != 1 {
			return fail(401, "UNAUTHENTICATED")
		}
		token, ok := s.getSession(req.Context(), cookies[0].Value)
		if !ok {
			return fail(401, "UNAUTHENTICATED")
		}
		if service == nil {
			return fail(503, "PREINSTALLED_UNAVAILABLE")
		}
		return token, true
	}
	r.GET("/studio/preinstalled", func(c *gin.Context) {
		token, ok := gate(c)
		if ok {
			result(c, service.Get(c.Request.Context(), token))
		}
	})
	r.POST("/studio/preinstalled/reconcile", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		var patch preinstalled.Intent
		if strictjson.Decode(c.Request.Body, 4096, &patch) != nil {
			result(c, application.Error(400, "INVALID_PREINSTALLED_REQUEST"))
			return
		}
		result(c, service.Execute(c.Request.Context(), token, patch))
	})
}
