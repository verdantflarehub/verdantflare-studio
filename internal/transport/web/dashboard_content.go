package web

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

type dashboardContentService interface {
	ReadContent(context.Context, string, string, application.DashboardContentInput, application.DashboardContentGateway) application.Result
	DownloadContent(context.Context, string, string, string, application.DashboardContentGateway) (*http.Response, application.Result)
}

func (s *Server) enableDashboardContent(r *gin.Engine, dashboards DashboardService, gate func(*gin.Context) (string, bool), result func(*gin.Context, application.Result)) {
	service, supported := dashboards.(dashboardContentService)
	gateway := func() application.DashboardContentGateway {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.mcpGateway == nil {
			return nil
		}
		return s.mcpGateway
	}
	r.POST("/studio/dashboards/:session_id/capabilities/artifact.read", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		if !supported {
			result(c, application.Error(503, "CAPABILITY_UNAVAILABLE"))
			return
		}
		in, err := application.DecodeDashboardContent(c.Request.Body)
		if err != nil {
			result(c, application.Error(400, "INVALID_CONTENT_REQUEST"))
			return
		}
		result(c, service.ReadContent(c.Request.Context(), token, c.Param("session_id"), in, gateway()))
	})
	r.GET("/studio/dashboards/:session_id/content/:handle", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		if !supported {
			result(c, application.Error(503, "CAPABILITY_UNAVAILABLE"))
			return
		}
		id, err := uuid.Parse(c.Param("handle"))
		if err != nil || id.Version() != 4 || id.String() != c.Param("handle") || c.Request.Header.Get("Range") != "" {
			result(c, application.Error(400, "INVALID_CONTENT_REQUEST"))
			return
		}
		response, out := service.DownloadContent(c.Request.Context(), token, c.Param("session_id"), id.String(), gateway())
		if out.Status != 200 {
			result(c, out)
			return
		}
		defer response.Body.Close()
		c.Header("X-Request-ID", out.RequestID)
		c.Header("Content-Type", response.Header.Get("Content-Type"))
		c.Header("Content-Length", strconv.FormatInt(response.ContentLength, 10))
		c.Header("Content-Disposition", "attachment")
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'")
		c.Status(200)
		_, _ = io.CopyN(c.Writer, response.Body, response.ContentLength)
	})
}
