package web

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

func (s *Server) artifactTransfer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	fail := func(status int, code string) {
		if c.Request.Body != nil && c.Request.ContentLength > 0 && c.Request.ContentLength <= 64<<10 && c.Request.Header.Get("Expect") == "" {
			_, _ = io.Copy(io.Discard, io.LimitReader(c.Request.Body, c.Request.ContentLength))
		}
		c.JSON(status, gin.H{"code": code, "message": "Content transfer failed", "request_id": c.Writer.Header().Get("X-Request-Id")})
	}
	p, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	r := c.Request
	scope, err := mcp.TransferScope(r.Method, r.URL.Path, r.URL.RawQuery)
	if err != nil || r.URL.RawPath != "" || r.Header.Get("Content-Encoding") != "" || r.Header.Get("Range") != "" || (r.Header.Get("X-Project-Id") != "" && r.Header.Get("X-Project-Id") != scope) || (r.Method == "GET" && r.ContentLength != 0) {
		fail(400, "INVALID_ARGUMENT")
		return
	}
	maxSize := int64(1 << 30)
	if raw := os.Getenv("STUDIO_ARTIFACT_MAX_CONTENT_BYTES"); raw != "" {
		maxSize, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || maxSize <= 0 || maxSize > 1<<50 {
			fail(503, "DEPENDENCY_UNAVAILABLE")
			return
		}
	}
	if r.Method == "PUT" && (r.ContentLength < 0 || r.ContentLength > maxSize) {
		fail(413, "INVALID_ARGUMENT")
		return
	}
	controller := http.NewResponseController(c.Writer)
	if controller.SetReadDeadline(time.Now().Add(30*time.Minute)) != nil || controller.SetWriteDeadline(time.Now().Add(30*time.Minute)) != nil {
		fail(503, "DEPENDENCY_UNAVAILABLE")
		return
	}
	s.mu.Lock()
	gateway := s.mcpGateway
	s.mu.Unlock()
	if gateway == nil {
		fail(503, "DEPENDENCY_UNAVAILABLE")
		return
	}
	var body io.Reader
	if r.Method == "PUT" {
		body = http.MaxBytesReader(c.Writer, r.Body, maxSize)
	}
	response, status, err := gateway.Transfer(r.Context(), p, r.Method, r.URL.Path, r.URL.RawQuery, body, r.ContentLength)
	if err != nil {
		fail(status, "DEPENDENCY_UNAVAILABLE")
		return
	}
	defer response.Body.Close()
	if status != 200 {
		// Do not forward arbitrary downstream bodies or headers on rejection.
		code := "DEPENDENCY_UNAVAILABLE"
		switch status {
		case 400:
			code = "INVALID_ARGUMENT"
		case 403:
			code = "PERMISSION_DENIED"
		case 404:
			code = "NOT_FOUND"
		case 409:
			code = "CONTENT_NOT_READY"
		default:
			status = 502
		}
		fail(status, code)
		return
	}
	if r.Method == "PUT" {
		b, err := io.ReadAll(io.LimitReader(response.Body, 4097))
		if err != nil || len(b) > 4096 {
			fail(502, "DEPENDENCY_UNAVAILABLE")
			return
		}
		var upload struct {
			UploadID    string `json:"upload_id"`
			VersionID   string `json:"version_id"`
			ArtifactID  string `json:"artifact_id"`
			State       string `json:"state"`
			ContentPath string `json:"content_path"`
		}
		if strictjson.Decode(bytes.NewReader(b), 4096, &upload) != nil || !project.ValidID(upload.UploadID) || !project.ValidID(upload.VersionID) || !project.ValidID(upload.ArtifactID) || (upload.State != "prepared" && upload.State != "committed") || upload.ContentPath != "/v2/artifacts/uploads/"+upload.UploadID+"/content" || upload.ContentPath != r.URL.Path {
			fail(502, "DEPENDENCY_UNAVAILABLE")
			return
		}
		c.Data(200, "application/json", b)
		return
	}
	if response.ContentLength < 0 || response.ContentLength > maxSize || response.Header.Get("Content-Encoding") != "" {
		fail(502, "DEPENDENCY_UNAVAILABLE")
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "ETag"} {
		c.Header(key, response.Header.Get(key))
	}
	c.Header("Content-Disposition", "attachment")
	c.Status(200)
	_, _ = io.Copy(c.Writer, io.LimitReader(response.Body, response.ContentLength))
}
