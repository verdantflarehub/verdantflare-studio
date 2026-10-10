package web

import (
	"mime"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcprpc"
)

func (s *Server) SetMCPGateway(gw *mcp.Gateway) { s.mu.Lock(); defer s.mu.Unlock(); s.mcpGateway = gw }

func (s *Server) mcpHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	var empty mcprpc.Request
	mt, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || mt != "application/json" || c.Request.URL.RawQuery != "" || c.GetHeader("Content-Encoding") != "" {
		mcprpc.Reject(c.Writer, c.Request, empty, 400, -32600, "Invalid request")
		return
	}
	principal, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	request, e := mcprpc.Decode(c.Request.Body)
	if e != nil {
		mcprpc.Reject(c.Writer, c.Request, empty, 400, -32700, "Invalid JSON-RPC request")
		return
	}
	if mcprpc.Common(c.Writer, request, "verdantflare-studio", "0.1.0") {
		return
	}
	s.mu.Lock()
	gateway := s.mcpGateway
	s.mu.Unlock()
	switch request.Method {
	case "tools/list":
		if len(request.ID) == 0 {
			mcprpc.Reject(c.Writer, c.Request, request, 400, -32600, "Request ID required")
			return
		}
		tools := []mcp.ToolDefinition{}
		if gateway != nil {
			tools = gateway.ListTools()
		}
		mcprpc.Result(c.Writer, request, map[string]any{"tools": tools})
	case "tools/call":
		name, args, _, e := request.Call()
		if e != nil {
			mcprpc.Reject(c.Writer, c.Request, request, 400, -32602, "Invalid tool parameters")
			return
		}
		if gateway == nil {
			mcprpc.Reject(c.Writer, c.Request, request, 503, -32001, "Service discovery unavailable")
			return
		}
		result, status, e := gateway.CallVerified(c.Request.Context(), name, args, principal, c.GetHeader("X-Project-Id"))
		if e != nil {
			mcprpc.Reject(c.Writer, c.Request, request, status, -32603, e.Error())
			return
		}
		c.Status(status)
		mcprpc.Result(c.Writer, request, result)
	default:
		mcprpc.Reject(c.Writer, c.Request, request, 404, -32601, "Method not found")
	}
}
