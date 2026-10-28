package web

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var videoErrorCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,95}$`)
var videoErrorTask = regexp.MustCompile(`^video_task_[a-zA-Z0-9_-]{1,96}$`)
var videoErrorRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)https?://\S+`),
	regexp.MustCompile(`(?i)["']?(?:api[_-]?key|access_token|refresh_token|token|password|secret|authorization)["']?\s*[:=]\s*["']?[^\s,}\]]+`),
	regexp.MustCompile(`(?i)bearer\s+\S+`),
	regexp.MustCompile(`/(?:[\w.-]+/)+[\w./-]*`),
	regexp.MustCompile(`(?i)\b[a-z]:[\\/][^\s,;]+`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{40,}\b`),
}

// writeVideoChannelError accepts only the Video public channel-error contract.
// Unknown keys and transport headers never cross the Studio boundary.
func writeVideoChannelError(c *gin.Context, resp *http.Response) bool {
	if strings.Split(resp.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(data) > 65536 {
		return false
	}
	var envelope struct {
		Error struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Source     string `json:"source"`
			Retryable  *bool  `json:"retryable"`
			Accepted   *bool  `json:"accepted,omitempty"`
			HTTPStatus *int   `json:"http_status,omitempty"`
			TaskID     string `json:"task_id,omitempty"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return false
	}
	e := &envelope.Error
	if !videoErrorCode.MatchString(e.Code) || e.Retryable == nil || strings.TrimSpace(e.Message) == "" {
		return false
	}
	switch e.Source {
	case "higgsfield", "fal", "grok", "google", "video":
	default:
		return false
	}
	for _, pattern := range videoErrorRedactions {
		e.Message = pattern.ReplaceAllString(e.Message, "[redacted]")
	}
	message := []rune(e.Message)
	if len(message) > 512 {
		e.Message = string(message[:512])
	}
	if e.HTTPStatus != nil && (*e.HTTPStatus < 400 || *e.HTTPStatus > 599) {
		e.HTTPStatus = nil
	}
	if !videoErrorTask.MatchString(e.TaskID) {
		e.TaskID = ""
	}
	c.JSON(resp.StatusCode, envelope)
	return true
}
