package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
)

type jsonRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) SetMCPGateway(gw *mcp.Gateway) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpGateway = gw
}

func (s *Server) mcpHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")

	// 1. Authentication
	authHeader := c.GetHeader("Authorization")
	configuredToken := os.Getenv("STUDIO_BEARER_TOKEN")

	var authenticated bool
	var userID = c.GetHeader("X-User-Id")
	var projectID = c.GetHeader("X-Project-Id")

	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if configuredToken == "" || token == configuredToken {
			authenticated = true
		}
	}

	if !authenticated {
		// Fallback to cookie session
		sid, _ := c.Cookie(cookieName)
		if _, valid := s.getSession(c.Request.Context(), sid); valid {
			authenticated = true
		}
	}

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"jsonrpc": "2.0",
			"error": gin.H{
				"code":    -32000,
				"message": "Unauthorized: valid Bearer token or Studio session required",
			},
		})
		return
	}

	if userID == "" {
		userID = "usr_studio_default"
	}
	if projectID == "" {
		projectID = "prj_studio_default"
	}

	reqID := c.GetHeader("X-Request-Id")
	if reqID == "" {
		reqID = generateRequestID()
	}
	c.Header("X-Request-Id", reqID)

	// 2. Decode JSON-RPC 2.0 Request
	var rpcReq jsonRPCRequest
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"error": gin.H{
				"code":    -32700,
				"message": "Parse error: request body too large or invalid",
			},
		})
		return
	}

	if err := json.Unmarshal(bodyBytes, &rpcReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"error": gin.H{
				"code":    -32700,
				"message": "Parse error: invalid JSON",
			},
		})
		return
	}

	s.mu.Lock()
	gw := s.mcpGateway
	s.mu.Unlock()

	// 3. Dispatch Methods
	switch rpcReq.Method {
	case "tools/list":
		var tools []mcp.ToolDefinition
		if gw != nil {
			tools = gw.ListTools()
		}
		if tools == nil {
			tools = []mcp.ToolDefinition{}
		}
		c.JSON(http.StatusOK, gin.H{
			"jsonrpc": "2.0",
			"id":      rpcReq.ID,
			"result": gin.H{
				"tools": tools,
			},
		})
		return

	case "tools/call":
		if gw == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"error": gin.H{
					"code":    -32001,
					"message": "Service Unavailable: MCP Gateway discovery is not ready",
				},
			})
			return
		}

		toolName, _ := rpcReq.Params["name"].(string)
		if toolName == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"error": gin.H{
					"code":    -32602,
					"message": "Invalid params: 'name' is required in params",
				},
			})
			return
		}

		arguments, _ := rpcReq.Params["arguments"].(map[string]any)
		if arguments == nil {
			arguments = make(map[string]any)
		}

		headers := map[string]string{
			"X-User-Id":    userID,
			"X-Project-Id": projectID,
			"X-Request-Id": reqID,
		}
		if authHeader != "" {
			headers["Authorization"] = authHeader
		}

		result, status, err := gw.CallTool(c.Request.Context(), toolName, arguments, headers)
		if err != nil {
			c.JSON(status, gin.H{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"error": gin.H{
					"code":    -32603,
					"message": err.Error(),
				},
			})
			return
		}

		c.JSON(status, gin.H{
			"jsonrpc": "2.0",
			"id":      rpcReq.ID,
			"result":  result,
		})
		return

	default:
		c.JSON(http.StatusNotFound, gin.H{
			"jsonrpc": "2.0",
			"id":      rpcReq.ID,
			"error": gin.H{
				"code":    -32601,
				"message": "Method not found: " + rpcReq.Method,
			},
		})
	}
}
