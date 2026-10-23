package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var contentRatingIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)

func hasProjectWritePermission(roles, scopes []string) bool {
	if slices.Contains(roles, "viewer") || slices.Contains(roles, "readonly") {
		return false
	}
	for _, r := range roles {
		if r == "admin" || r == "owner" || r == "editor" || r == "operator" {
			return true
		}
	}
	for _, sc := range scopes {
		if sc == "app:manage" || sc == "project:write" {
			return true
		}
	}
	return false
}

func (s *Server) handleContentRatingAction(c *gin.Context, path string) {
	if c.Request.Method != http.MethodPost {
		c.JSON(405, gin.H{"code": "METHOD_NOT_ALLOWED"})
		return
	}
	if c.GetHeader("Authorization") != "" || c.GetHeader("Origin") != s.origin || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		c.JSON(403, gin.H{"code": "PERMISSION_DENIED"})
		return
	}
	if strings.Contains(c.Request.URL.EscapedPath(), "%") || c.Request.URL.RawQuery != "" {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "content-ratings" {
		c.JSON(404, gin.H{"code": "NOT_FOUND"})
		return
	}
	domain, versionID, action := parts[2], parts[3], parts[4]
	if !slices.Contains([]string{"image", "speech", "music", "video"}, domain) ||
		(action != "clear" && action != "mark") ||
		!contentRatingIDPattern.MatchString(versionID) || strings.Contains(versionID, "..") {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return
	}

	rawBody, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	if err != nil {
		c.JSON(413, gin.H{"code": "INVALID_ARGUMENT"})
		return
	}
	projectID := strings.TrimSpace(c.GetHeader("X-Project-Id"))
	forwardPayload := map[string]any{}
	if len(bytes.TrimSpace(rawBody)) > 0 {
		mt, _, parseErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if parseErr != nil || mt != "application/json" {
			c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
			return
		}
		var in map[string]any
		if json.Unmarshal(rawBody, &in) != nil || in == nil {
			c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
			return
		}
		if pid, ok := in["project_id"].(string); ok && strings.TrimSpace(pid) != "" {
			if projectID != "" && projectID != strings.TrimSpace(pid) {
				c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
				return
			}
			projectID = strings.TrimSpace(pid)
		}
		if reason, ok := in["reason"].(string); ok {
			if len([]rune(reason)) > 200 {
				c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
				return
			}
			forwardPayload["reason"] = reason
		}
		if reasons, ok := in["reasons"].([]any); ok {
			forwardPayload["reasons"] = reasons
		}
		if projectID != "" {
			forwardPayload["project_id"] = projectID
		}
	}
	if projectID != "" && !contentRatingIDPattern.MatchString(projectID) {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return
	}
	forwardBytes, _ := json.Marshal(forwardPayload)

	sid, _ := c.Cookie(cookieName)
	token, valid := s.getSession(c.Request.Context(), sid)
	if !valid {
		c.JSON(401, gin.H{"code": "UNAUTHENTICATED"})
		return
	}
	auth := s.station.Call(c.Request.Context(), token, application.Request{Path: "me", Method: "GET"})
	if auth.Status != 200 {
		c.Data(auth.Status, "application/json", auth.Data)
		return
	}
	var identity struct {
		Roles  []string `json:"roles"`
		Scopes []string `json:"scopes"`
	}
	if json.Unmarshal(auth.Data, &identity) != nil || !hasProjectWritePermission(identity.Roles, identity.Scopes) {
		c.JSON(403, gin.H{"code": "PERMISSION_DENIED"})
		return
	}
	reqID := uuid.Must(uuid.NewV7()).String()
	principal, status := s.station.Principal(c.Request.Context(), token, reqID)
	if status != 200 {
		c.JSON(status, gin.H{"code": "UNAUTHENTICATED"})
		return
	}

	s.mu.Lock()
	gw := s.mcpGateway
	imgCfg := s.imageConfig
	vidCfg := s.videoConfig
	s.mu.Unlock()

	if gw != nil {
		if _, ok := gw.ServiceEndpoint(domain); ok {
			respBytes, code, fwdErr := gw.ForwardContentRatingAction(c.Request.Context(), domain, versionID, action, forwardBytes, principal, projectID)
			if fwdErr != nil {
				c.JSON(code, gin.H{"code": "DEPENDENCY_UNAVAILABLE", "message": fwdErr.Error()})
				return
			}
			c.Data(code, "application/json", respBytes)
			return
		}
	}

	var fallbackURL, fallbackToken string
	switch domain {
	case "image":
		fallbackURL, fallbackToken = imgCfg.URL, imgCfg.Token
	case "video":
		fallbackURL, fallbackToken = vidCfg.URL, vidCfg.Token
	}
	if fallbackURL == "" || fallbackToken == "" {
		c.JSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
		return
	}
	u, err := url.Parse(fallbackURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		c.JSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
		return
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/internal/content-ratings/" + versionID + "/" + action
	u.RawQuery = ""
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, u.String(), bytes.NewReader(forwardBytes))
	if err != nil {
		c.JSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+fallbackToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-User-Id", principal.SubjectID)
	req.Header.Set("X-Organization-Id", principal.OrganizationID)
	req.Header.Set("X-Request-Id", principal.RequestID)
	if projectID != "" {
		req.Header.Set("X-Project-Id", projectID)
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"code": "DEPENDENCY_UNAVAILABLE"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 || resp.StatusCode >= 500 {
		c.JSON(502, gin.H{"code": "DEPENDENCY_UNAVAILABLE"})
		return
	}
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(respBytes) > 64<<10 {
		c.JSON(502, gin.H{"code": "DEPENDENCY_UNAVAILABLE"})
		return
	}
	c.Data(resp.StatusCode, "application/json", respBytes)
}

var _ = project.Principal{}
