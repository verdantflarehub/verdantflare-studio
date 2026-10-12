package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

func (s *Server) checkGenerationProxy(c *gin.Context, token string) bool {
	if os.Getenv("STUDIO_GENERATION_CONTROL_ENABLED") != "true" {
		return true
	}
	if s.mcpGateway == nil {
		c.Status(503)
		return false
	}
	data, e := strictjson.ReadValue(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20), 1<<20)
	if e != nil {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(data))
	var args map[string]any
	if json.Unmarshal(data, &args) != nil || args == nil {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return false
	}
	p, status := s.station.Principal(c.Request.Context(), token, uuid.Must(uuid.NewV7()).String())
	if status != 200 {
		c.Status(status)
		return false
	}
	if status, e := s.mcpGateway.CheckGenerationDirect(c.Request.Context(), p, args, c.GetHeader("X-Project-Id")); e != nil {
		c.JSON(status, gin.H{"code": "GENERATION_APPROVAL_REQUIRED"})
		return false
	}
	return true
}

// MCP tokens cannot invoke this decision route. A fresh password is exchanged
// only with Station; the downstream Project receives the pinned digest alone.
func (s *Server) generationApprove(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	for _, key := range []string{"Authorization", "Origin", "Content-Type", "Sec-Fetch-Site"} {
		if len(c.Request.Header.Values(key)) > 1 {
			c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
			return
		}
	}
	if c.GetHeader("Authorization") != "" || c.GetHeader("Origin") != s.origin || c.GetHeader("Sec-Fetch-Site") != "same-origin" {
		c.JSON(403, gin.H{"code": "HUMAN_CONFIRMATION_REQUIRED"})
		return
	}
	mt, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || mt != "application/json" || c.Request.URL.RawQuery != "" || c.GetHeader("Content-Encoding") != "" {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return
	}
	if len(c.Request.CookiesNamed(cookieName)) != 1 {
		c.JSON(401, gin.H{"code": "UNAUTHENTICATED"})
		return
	}
	sid, _ := c.Cookie(cookieName)
	token, ok := s.getSession(c.Request.Context(), sid)
	if !ok || s.mcpGateway == nil {
		c.JSON(401, gin.H{"code": "UNAUTHENTICATED"})
		return
	}
	var in struct {
		project.GenerationDecision
		Password string `json:"password"`
	}
	if strictjson.Decode(c.Request.Body, 8192, &in) != nil {
		c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
		return
	}
	id := uuid.Must(uuid.NewV7()).String()
	c.Header("X-Request-Id", id)
	p, status := s.station.ConfirmGenerationIdentity(c.Request.Context(), token, id, in.Password)
	in.Password = ""
	if status != 200 {
		c.JSON(status, gin.H{"code": "CONFIRMATION_AUTHENTICATION_FAILED"})
		return
	}
	data, status, e := s.mcpGateway.GenerationApprove(c.Request.Context(), p, in.GenerationDecision)
	if e != nil {
		c.JSON(status, gin.H{"code": "GENERATION_APPROVAL_UNAVAILABLE"})
		return
	}
	c.Data(status, "application/json", data)
}
