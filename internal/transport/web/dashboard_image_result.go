package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type dashboardImageResultService interface {
	ReadImageResult(context.Context, string, string, application.DashboardImageResultInput, application.DashboardContentGateway, application.ImageResultReader) application.Result
}

func (s *Server) enableDashboardImageResult(r *gin.Engine, dashboards DashboardService, gate func(*gin.Context) (string, bool), result func(*gin.Context, application.Result)) {
	r.POST("/studio/dashboards/:session_id/capabilities/image.result.read", func(c *gin.Context) {
		token, ok := gate(c)
		if !ok {
			return
		}
		service, ok := dashboards.(dashboardImageResultService)
		if !ok {
			result(c, application.Error(503, "CAPABILITY_UNAVAILABLE"))
			return
		}
		in, err := application.DecodeDashboardImageResult(c.Request.Body)
		if err != nil {
			result(c, application.Error(400, "INVALID_RESULT_REQUEST"))
			return
		}
		s.mu.Lock()
		gateway := s.mcpGateway
		s.mu.Unlock()
		if gateway == nil {
			result(c, application.Error(503, "RESULT_STATUS_UNAVAILABLE"))
			return
		}
		ctx := dashboard.WithIncludeNSFW(c.Request.Context(), c.GetHeader("X-VF-Include-NSFW") == "1")
		result(c, service.ReadImageResult(ctx, token, c.Param("session_id"), in, gateway, s.readImageResult))
	})
}

func (s *Server) readImageResult(ctx context.Context, principal project.Principal, projectID, artifactID string) (application.ImageResultStatus, int, error) {
	return s.readImageResultWithPreference(ctx, principal, projectID, artifactID, dashboard.IncludeNSFW(ctx))
}
func (s *Server) readMCPImageResult(ctx context.Context, principal project.Principal, projectID, artifactID string) (application.ImageResultStatus, int, error) {
	return s.readImageResultWithPreference(ctx, principal, projectID, artifactID, false)
}
func (s *Server) readImageResultWithPreference(ctx context.Context, principal project.Principal, projectID, artifactID string, include bool) (application.ImageResultStatus, int, error) {
	var value application.ImageResultStatus
	s.mu.Lock()
	cfg := s.imageConfig
	s.mu.Unlock()
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || cfg.Token == "" || !principal.Valid() {
		return value, 503, fmt.Errorf("image result service unavailable")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/internal/results/read"
	body, _ := json.Marshal(map[string]string{"project_id": projectID, "artifact_id": artifactID})
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return value, 503, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", principal.SubjectID)
	req.Header.Set("X-Organization-Id", principal.OrganizationID)
	req.Header.Set("X-Request-Id", principal.RequestID)
	if include {
		req.Header.Set("X-VF-Include-NSFW", "1")
	}
	response, err := s.proxyClient().Do(req)
	if err != nil {
		return value, 503, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		if response.StatusCode == 403 || response.StatusCode == 404 {
			return value, response.StatusCode, nil
		}
		return value, 503, fmt.Errorf("image result service unavailable")
	}
	data, err := strictjson.ReadValue(response.Body, 4096)
	if err != nil {
		return value, 503, err
	}
	var fields map[string]json.RawMessage
	var rating map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 4 || len(fields["project_id"]) == 0 || len(fields["artifact_id"]) == 0 || len(fields["sha256"]) == 0 || json.Unmarshal(fields["content_rating"], &rating) != nil || len(rating) != 3 || len(rating["rating"]) == 0 || len(rating["source"]) == 0 || len(rating["human_cleared"]) == 0 || string(rating["human_cleared"]) == "null" || json.Unmarshal(data, &value) != nil {
		return value, 503, fmt.Errorf("invalid image result response")
	}
	return value, 200, nil
}
