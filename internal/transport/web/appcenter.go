package web

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

type AppCenterService interface {
	Get(context.Context, string) application.Result
	Open(context.Context, string, appcenter.OpenRequest) application.Result
	Prepare(context.Context, string, appcenter.CommandPrepare) application.Result
	Submit(context.Context, string, appcenter.CommandSubmit) application.Result
}

func (s *Server) EnableAppCenter(r *gin.Engine, service AppCenterService) {
	handler := func(c *gin.Context) {
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
		if len(origins) > 1 || (len(origins) == 1 && origins[0] != s.origin) || (req.Method == "POST" && len(origins) != 1) {
			fail(403, "PERMISSION_DENIED")
			return
		}
		sites := req.Header.Values("Sec-Fetch-Site")
		if len(sites) > 1 || (len(sites) == 1 && sites[0] != "same-origin" && sites[0] != "none") {
			fail(403, "PERMISSION_DENIED")
			return
		}
		var open appcenter.OpenRequest
		var prepare appcenter.CommandPrepare
		var submit appcenter.CommandSubmit
		if req.Method == "POST" {
			name, limit, target := "open", int64(4<<10), any(&open)
			switch req.URL.Path {
			case "/studio/appcenter/v1/commands/prepare":
				name, limit, target = "command-prepare", 16<<10, &prepare
			case "/studio/appcenter/v1/commands":
				name, limit, target = "command-submit", 16<<10, &submit
			}
			kind, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
			if len(req.Header.Values("Content-Type")) != 1 || err != nil || kind != "application/json" || appcenter.Decode(req.Body, name, limit, target) != nil {
				fail(400, "INVALID_APPCENTER_REQUEST")
				return
			}
		} else if req.Body != nil {
			body, err := io.ReadAll(io.LimitReader(req.Body, 1))
			if err != nil || len(body) != 0 {
				fail(400, "INVALID_APPCENTER_REQUEST")
				return
			}
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
		if req.URL.Path == "/studio/appcenter/v1/events" || req.URL.Path == "/studio/navigation/events" {
			authorizer, ok := service.(appCenterEventAuthorizer)
			if !ok {
				fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
				return
			}
			s.appCenterEvents(c, cookies[0].Value, token, authorizer)
			return
		}
		var out application.Result
		switch req.URL.Path {
		case "/studio/appcenter/v1/commands/prepare":
			out = service.Prepare(req.Context(), token, prepare)
		case "/studio/appcenter/v1/commands":
			out = service.Submit(req.Context(), token, submit)
		case "/studio/appcenter/v1/open":
			out = service.Open(req.Context(), token, open)
		default:
			out = service.Get(req.Context(), token)
		}
		if out.Status < 200 || out.Status > 599 || !json.Valid(out.Data) {
			fail(502, "APPCENTER_INVALID_FACTS")
			return
		}
		c.Header("X-Request-ID", out.RequestID)
		c.Data(out.Status, "application/json", out.Data)
	}
	r.GET("/studio/appcenter/v1/view", handler)
	r.GET("/studio/appcenter/v1/events", handler)
	r.GET("/studio/navigation/events", handler)
	r.POST("/studio/appcenter/v1/open", handler)
	r.POST("/studio/appcenter/v1/commands/prepare", handler)
	r.POST("/studio/appcenter/v1/commands", handler)
}
