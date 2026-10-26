package web

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

type dashboardMediaService interface {
	DownloadMedia(context.Context, string, string, string, string, application.DashboardContentGateway) (application.MediaFile, application.Result)
}

func (s *Server) enableDashboardMedia(r *gin.Engine, dashboards DashboardService, gate func(*gin.Context) (string, bool), result func(*gin.Context, application.Result)) {
	service, supported := dashboards.(dashboardMediaService)
	handler := func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		if !supported {
			result(c, application.Error(503, "CAPABILITY_UNAVAILABLE"))
			return
		}
		s.mu.Lock()
		var gateway application.DashboardContentGateway
		if s.mcpGateway != nil {
			gateway = s.mcpGateway
		}
		s.mu.Unlock()
		media, out := service.DownloadMedia(c.Request.Context(), token, c.Param("cache_scope"), c.Param("resource_key"), c.Param("representation_key"), gateway)
		if out.Status != 200 {
			if out.Status == 429 {
				c.Header("Retry-After", "1")
			}
			result(c, out)
			return
		}
		defer media.File.Close()
		c.Header("X-Request-ID", out.RequestID)
		serveDashboardMedia(c.Writer, c.Request, media)
	}
	r.GET("/studio/media/:cache_scope/:resource_key/:representation_key", handler)
	r.HEAD("/studio/media/:cache_scope/:resource_key/:representation_key", handler)
}

func serveDashboardMedia(w http.ResponseWriter, r *http.Request, media application.MediaFile) {
	w.Header().Set("Cache-Control", application.MediaCacheControl)
	if media.CacheStatus != "" {
		w.Header().Set("X-VF-Media-Cache", media.CacheStatus)
	}
	w.Header().Set("Content-Type", media.MIME)
	w.Header().Set("ETag", media.ETag)
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'")
	http.ServeContent(mediaResponseWriter{w}, r, "", time.Time{}, io.NewSectionReader(media.File, 32, media.Size))
}

type mediaResponseWriter struct{ http.ResponseWriter }

func (w mediaResponseWriter) WriteHeader(status int) {
	if status >= 400 {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("ETag")
	}
	w.ResponseWriter.WriteHeader(status)
}
