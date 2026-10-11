package web

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

type AppCenterService interface {
	Get(context.Context, string) application.Result
}

func (s *Server) EnableAppCenter(r *gin.Engine, service AppCenterService) {
	r.GET("/studio/appcenter/v1/view", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		fail := func(status int, code string) {
			id := application.ID()
			c.Header("X-Request-ID", id)
			c.JSON(status, gin.H{"code": code, "requestId": id})
		}
		req := c.Request
		if req.URL.RawQuery != "" || req.URL.ForceQuery || strings.Contains(req.URL.EscapedPath(), "%") || strings.Contains(req.URL.Path, "\\") || len(req.Header.Values("Authorization")) != 0 || len(req.Header.Values("Content-Encoding")) != 0 {
			fail(400, "INVALID_APPCENTER_REQUEST")
			return
		}
		origins := req.Header.Values("Origin")
		if len(origins) > 1 || (len(origins) == 1 && origins[0] != s.origin) {
			fail(403, "PERMISSION_DENIED")
			return
		}
		sites := req.Header.Values("Sec-Fetch-Site")
		if len(sites) > 1 || (len(sites) == 1 && sites[0] != "same-origin" && sites[0] != "none") {
			fail(403, "PERMISSION_DENIED")
			return
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, 1))
		if err != nil || len(body) != 0 {
			fail(400, "INVALID_APPCENTER_REQUEST")
			return
		}
		cookies := req.CookiesNamed(cookieName)
		if len(cookies) != 1 {
			fail(401, "UNAUTHENTICATED")
			return
		}
		token, ok := s.getSession(req.Context(), cookies[0].Value)
		if !ok {
			fail(401, "UNAUTHENTICATED")
			return
		}
		if service == nil {
			fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
			return
		}
		out := service.Get(req.Context(), token)
		if out.Status < 200 || out.Status > 599 || !json.Valid(out.Data) {
			fail(502, "APPCENTER_INVALID_FACTS")
			return
		}
		c.Header("X-Request-ID", out.RequestID)
		c.Data(out.Status, "application/json", out.Data)
	})
}
