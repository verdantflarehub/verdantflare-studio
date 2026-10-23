package web

import (
	"context"
	"io"
	"mime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type DashboardService interface {
	Installations(context.Context, string) application.Result
	Inspect(context.Context, string, string) application.Result
	Open(context.Context, string, string) application.Result
	Asset(context.Context, string, string, string) ([]byte, string, application.Result)
	Request(context.Context, string, string, dashboard.AppRequest) application.Result
	Close(context.Context, string, string) application.Result
	OpenInstance(context.Context, string, string, dashboard.InstanceOpen) application.Result
	InspectWorkspace(context.Context, string, string) application.Result
	WorkspaceAsset(context.Context, string, string, string) ([]byte, string, application.Result)
	WorkspaceRequest(context.Context, string, string, dashboard.AppRequest) application.Result
	CloseWorkspace(context.Context, string, string) application.Result
	PublicInstanceAsset(context.Context, string, string) ([]byte, string, application.Result)
}

// EnableDashboards registers only platform routes, independent of App names.
// Install at startup before serving requests; the service reads policy per call.
func (s *Server) EnableDashboards(r *gin.Engine, service DashboardService) {
	result := func(c *gin.Context, out application.Result) {
		c.Header("X-Request-ID", out.RequestID)
		if out.Status == 204 {
			c.Status(204)
			return
		}
		c.Data(out.Status, "application/json", out.Data)
	}
	r.GET("/studio/instance-assets/:artifact/*path", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Cross-Origin-Resource-Policy", "cross-origin")
		req := c.Request
		body, err := io.ReadAll(io.LimitReader(req.Body, 1))
		if err != nil || len(body) != 0 || req.URL.RawQuery != "" || req.URL.ForceQuery || strings.Contains(req.URL.EscapedPath(), "%") || len(req.Header.Values("Authorization")) != 0 {
			result(c, application.Error(400, "INVALID_INSTANCE_ASSET"))
			return
		}
		if service == nil {
			result(c, application.Error(503, "DASHBOARD_UNAVAILABLE"))
			return
		}
		data, mime, out := service.PublicInstanceAsset(req.Context(), c.Param("artifact"), strings.TrimPrefix(c.Param("path"), "/"))
		if out.Status != 200 {
			result(c, out)
			return
		}
		c.Header("Content-Security-Policy", "frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		c.Data(200, mime, data)
	})
	gate := func(c *gin.Context) (string, bool) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		req := c.Request
		fail := func(status int, code string) (string, bool) {
			result(c, application.Error(status, code))
			return "", false
		}
		if req.URL.RawQuery != "" || req.URL.ForceQuery || strings.Contains(req.URL.EscapedPath(), "%") || strings.Contains(req.URL.Path, "\\") ||
			len(req.Header.Values("Authorization")) != 0 || len(req.Header.Values("Content-Encoding")) != 0 || len(req.Header.Values("X-Review-Origin")) != 0 ||
			len(req.Header.Values("X-VF-Include-NSFW")) > 1 || (req.Header.Get("X-VF-Include-NSFW") != "" && req.Header.Get("X-VF-Include-NSFW") != "1") {
			return fail(400, "INVALID_DASHBOARD_REQUEST")
		}
		origins := req.Header.Values("Origin")
		if len(origins) > 1 || (len(origins) == 1 && origins[0] != s.origin) || (req.Method != "GET" && len(origins) != 1) {
			return fail(403, "DASHBOARD_DENIED")
		}
		if site := req.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			return fail(403, "DASHBOARD_DENIED")
		}
		if req.Method == "POST" {
			value, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
			if len(req.Header.Values("Content-Type")) != 1 || err != nil || value != "application/json" {
				return fail(400, "INVALID_DASHBOARD_REQUEST")
			}
		} else {
			body, err := io.ReadAll(io.LimitReader(req.Body, 1))
			if err != nil || len(body) != 0 {
				return fail(400, "INVALID_DASHBOARD_REQUEST")
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
		if id := c.Param("session_id"); id != "" {
			parsed, err := uuid.Parse(id)
			if err != nil || parsed.Version() != 4 || parsed.String() != id {
				return fail(400, "INVALID_DASHBOARD_REQUEST")
			}
		}
		if service == nil {
			return fail(503, "DASHBOARD_UNAVAILABLE")
		}
		return token, true
	}
	r.GET("/studio/dashboard-installations", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		result(c, service.Installations(c.Request.Context(), token))
	})
	r.GET("/studio/dashboards/:session_id", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		result(c, service.Inspect(c.Request.Context(), token, c.Param("session_id")))
	})
	r.POST("/studio/dashboards", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		var input struct {
			InstallationID string `json:"installationId"`
		}
		if strictjson.Decode(c.Request.Body, 4096, &input) != nil {
			result(c, application.Error(400, "INVALID_DASHBOARD_REQUEST"))
			return
		}
		id, err := uuid.Parse(input.InstallationID)
		if err != nil || id.Version() != 7 || id.String() != input.InstallationID {
			result(c, application.Error(400, "INVALID_DASHBOARD_REQUEST"))
			return
		}
		result(c, service.Open(c.Request.Context(), token, input.InstallationID))
	})
	r.GET("/studio/dashboards/:session_id/assets/*path", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		data, mime, out := service.Asset(c.Request.Context(), token, c.Param("session_id"), strings.TrimPrefix(c.Param("path"), "/"))
		if out.Status != 200 {
			result(c, out)
			return
		}
		c.Header("X-Request-ID", out.RequestID)
		// Wujie consumes fetched bytes; standalone navigation must not execute them.
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		c.Data(200, mime, data)
	})
	r.POST("/studio/dashboards/:session_id/requests", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		input, err := dashboard.DecodeRequest(c.Request.Body)
		if err != nil {
			result(c, application.Error(400, "INVALID_DASHBOARD_REQUEST"))
			return
		}
		if input.Path == "/api/v1/workspaces/release" && input.Method == "POST" {
			if blocked := s.guardComfyUIWorkspaceRelease(c.Request.Context(), token, input.Body); blocked != nil {
				result(c, *blocked)
				return
			}
		}
		reqCtx := dashboard.WithInteractiveReview(c.Request.Context())
		reqCtx = dashboard.WithIncludeNSFW(reqCtx, c.GetHeader("X-VF-Include-NSFW") == "1")
		out := service.Request(reqCtx, token, c.Param("session_id"), input)
		if input.Path == "/api/v1/workspaces" && input.Method == "GET" && out.Status == 200 {
			out.Data = s.enrichWorkspaces(c.Request.Context(), token, out.Data)
		}
		result(c, out)
	})
	r.DELETE("/studio/dashboards/:session_id", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		result(c, service.Close(c.Request.Context(), token, c.Param("session_id")))
	})
	s.enableDashboardContent(r, service, gate, result)
	r.POST("/studio/dashboards/:session_id/instances/open", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		var input dashboard.InstanceOpen
		if strictjson.Decode(c.Request.Body, 4096, &input) != nil || !dashboard.ValidInstanceOpen(input) {
			result(c, application.Error(400, "INVALID_INSTANCE_ENTRY"))
			return
		}
		result(c, service.OpenInstance(c.Request.Context(), token, c.Param("session_id"), input))
	})
	r.GET("/studio/instance-workspaces/:session_id", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		result(c, service.InspectWorkspace(c.Request.Context(), token, c.Param("session_id")))
	})
	r.GET("/studio/instance-workspaces/:session_id/assets/*path", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		data, mime, out := service.WorkspaceAsset(c.Request.Context(), token, c.Param("session_id"), strings.TrimPrefix(c.Param("path"), "/"))
		if out.Status != 200 {
			result(c, out)
			return
		}
		c.Header("X-Request-ID", out.RequestID)
		// The trusted App wrapper may execute; its vendor editor has a separate
		// opaque sandbox. Workspaces have no top navigation or popup capability.
		c.Header("Content-Security-Policy", "frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		c.Data(200, mime, data)
	})
	r.POST("/studio/instance-workspaces/:session_id/requests", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		input, err := dashboard.DecodeRequest(c.Request.Body)
		if err != nil {
			result(c, application.Error(400, "INVALID_WORKSPACE_REQUEST"))
			return
		}
		reqCtx := dashboard.WithIncludeNSFW(c.Request.Context(), c.GetHeader("X-VF-Include-NSFW") == "1")
		result(c, service.WorkspaceRequest(reqCtx, token, c.Param("session_id"), input))
	})
	r.DELETE("/studio/instance-workspaces/:session_id", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		result(c, service.CloseWorkspace(c.Request.Context(), token, c.Param("session_id")))
	})
}
