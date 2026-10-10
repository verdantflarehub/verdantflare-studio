package web

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcprpc"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func (s *Server) authenticateMCP(c *gin.Context) (project.Principal, bool) {
	var empty mcprpc.Request
	for _, key := range []string{"Authorization", "Origin", "Sec-Fetch-Site", "X-User-Id", "X-Organization-Id", "X-Project-Id", "X-Request-Id"} {
		if len(c.Request.Header.Values(key)) > 1 {
			mcprpc.Reject(c.Writer, c.Request, empty, 400, -32600, "Duplicate request header")
			return project.Principal{}, false
		}
	}
	if origin := c.GetHeader("Origin"); origin != "" && origin != s.origin {
		mcprpc.Reject(c.Writer, c.Request, empty, 403, -32000, "Origin denied")
		return project.Principal{}, false
	}
	id := c.GetHeader("X-Request-Id")
	if !project.ValidID(id) {
		id = uuid.Must(uuid.NewV7()).String()
	}
	c.Header("X-Request-Id", id)
	token := ""
	if auth := c.GetHeader("Authorization"); auth != "" {
		if !strings.HasPrefix(auth, "Bearer ") || strings.ContainsAny(strings.TrimPrefix(auth, "Bearer "), " \t\r\n") {
			mcprpc.Reject(c.Writer, c.Request, empty, 401, -32000, "Authentication required")
			return project.Principal{}, false
		}
		token = strings.TrimPrefix(auth, "Bearer ")

	} else {
		// Browsers omit Origin on same-origin GET downloads. Fetch Metadata is
		// browser-controlled; same-site and cross-site requests remain denied.
		sameOriginDownload := c.Request.Method == "GET" && c.GetHeader("Origin") == "" && c.GetHeader("Sec-Fetch-Site") == "same-origin"
		if c.GetHeader("Origin") != s.origin && !sameOriginDownload {
			mcprpc.Reject(c.Writer, c.Request, empty, 403, -32000, "Same-origin session required")
			return project.Principal{}, false
		}
		sid, _ := c.Cookie(cookieName)
		token, _ = s.getSession(c.Request.Context(), sid)
	}
	if token == "" {
		mcprpc.Reject(c.Writer, c.Request, empty, 401, -32000, "Authentication required")
		return project.Principal{}, false
	}
	principal, status := s.station.Principal(c.Request.Context(), token, id)
	if status != 200 {
		mcprpc.Reject(c.Writer, c.Request, empty, status, -32000, "Bearer identity verification failed")
		return project.Principal{}, false
	}
	if (c.GetHeader("X-User-Id") != "" && c.GetHeader("X-User-Id") != principal.SubjectID) || (c.GetHeader("X-Organization-Id") != "" && c.GetHeader("X-Organization-Id") != principal.OrganizationID) {
		mcprpc.Reject(c.Writer, c.Request, empty, 403, -32000, "Identity header mismatch")
		return project.Principal{}, false
	}
	c.Set("verifiedStationToken", token)
	return principal, true
}
