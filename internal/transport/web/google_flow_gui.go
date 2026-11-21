package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

const (
	defaultGoogleFlowManagementOrigin = "http://google-flow-channel.verdantflare-app.svc.cluster.local:8001"
	defaultGoogleFlowWebRTCOrigin     = "http://google-flow-channel.verdantflare-app.svc.cluster.local:48083"
)

var googleFlowRecheckInterval = 5 * time.Second

func googleFlowOrigin(envKey, fallback string) (*url.URL, bool) {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		raw = fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return nil, false
	}
	u.Path = ""
	return u, true
}

func (s *Server) googleFlowManagementRequest(ctx context.Context, actor dashboard.Actor, requestID, method, path string) (int, http.Header, []byte, error) {
	u, ok := googleFlowOrigin("STUDIO_GOOGLE_FLOW_ORIGIN", defaultGoogleFlowManagementOrigin)
	token := strings.TrimSpace(os.Getenv("STUDIO_APP_GOOGLE_FLOW_TOKEN"))
	if !ok || token == "" {
		return 0, nil, nil, errors.New("google flow desktop service unavailable")
	}
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", actor.UserID)
	req.Header.Set("X-Organization-Id", actor.OrganizationID)
	req.Header.Set("X-Station-Id", actor.StationID)
	req.Header.Set("X-Request-Id", requestID)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header.Clone(), body, nil
}

func (s *Server) googleFlowGUICheck(ctx context.Context, actor dashboard.Actor, requestID string) error {
	status, _, body, err := s.googleFlowManagementRequest(ctx, actor, requestID, http.MethodGet, "/desktop")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return errors.New("google flow desktop unavailable")
	}
	var result struct {
		Open bool `json:"open"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return errors.New("google flow desktop status invalid")
	}
	if !result.Open {
		return errors.New("google flow desktop closed")
	}
	return nil
}

func filterGoogleFlowTurnResponse(raw []byte) ([]byte, error) {
	var parsed struct {
		ExpiresAt  any `json:"expires_at,omitempty"`
		IceServers []struct {
			URLs       []string `json:"urls"`
			Username   string   `json:"username,omitempty"`
			Credential string   `json:"credential,omitempty"`
		} `json:"iceServers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	type safeServer struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username,omitempty"`
		Credential string   `json:"credential,omitempty"`
	}
	servers := make([]safeServer, 0, len(parsed.IceServers))
	for _, srv := range parsed.IceServers {
		filtered := make([]string, 0, len(srv.URLs))
		for _, u := range srv.URLs {
			if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
				filtered = append(filtered, u)
			}
		}
		if len(filtered) > 0 {
			servers = append(servers, safeServer{
				URLs:       filtered,
				Username:   srv.Username,
				Credential: srv.Credential,
			})
		}
	}
	if len(servers) == 0 {
		return nil, errors.New("no valid turn relay servers")
	}
	out := map[string]any{
		"iceTransportPolicy": "relay",
		"iceServers":         servers,
	}
	if parsed.ExpiresAt != nil {
		out["expires_at"] = parsed.ExpiresAt
	}
	return json.Marshal(out)
}

func (s *Server) authenticateGoogleFlowDesktop(c *gin.Context, path string) (dashboard.Actor, string, string, string, bool) {
	// Allow direct browser address-bar navigation (Sec-Fetch-Site: none) to the desktop HTML entrypoint
	// while still enforcing strict same-origin checks on subresources, /turn, /settings, and WebSocket.
	isTopLevelEntry := (path == "/" || path == "/index.html") &&
		c.Request.Method == http.MethodGet &&
		c.GetHeader("Origin") == "" &&
		c.GetHeader("Authorization") == "" &&
		(c.GetHeader("Sec-Fetch-Site") == "none" || c.GetHeader("Sec-Fetch-Site") == "same-origin")

	initialToken := strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer ")
	cookieID := ""
	if cookie, err := c.Request.Cookie(cookieName); err == nil {
		cookieID = cookie.Value
	}

	var reqID string
	if isTopLevelEntry {
		reqID = uuid.Must(uuid.NewV7()).String()
		c.Header("X-Request-Id", reqID)
		token, _ := s.getSession(c.Request.Context(), cookieID)
		if token == "" {
			c.Status(http.StatusUnauthorized)
			return dashboard.Actor{}, "", "", "", false
		}
		actor, status := s.station.DashboardActor(c.Request.Context(), token, reqID)
		if status != http.StatusOK {
			c.Status(http.StatusUnauthorized)
			return dashboard.Actor{}, "", "", "", false
		}
		return actor, reqID, "", cookieID, true
	}

	p, ok := s.authenticateMCP(c)
	if !ok {
		return dashboard.Actor{}, "", "", "", false
	}
	token := initialToken
	if token == "" {
		token, _ = s.getSession(c.Request.Context(), cookieID)
	}
	actor, actorStatus := s.station.DashboardActor(c.Request.Context(), token, p.RequestID)
	if actorStatus != http.StatusOK || actor.UserID != p.SubjectID || actor.OrganizationID != p.OrganizationID {
		c.Status(http.StatusUnauthorized)
		return dashboard.Actor{}, "", "", "", false
	}
	return actor, p.RequestID, initialToken, cookieID, true
}

func (s *Server) googleFlowDesktop(c *gin.Context) {
	path := c.Param("path")
	if strings.Contains(c.Request.URL.EscapedPath(), "%2e") || strings.Contains(c.Request.URL.EscapedPath(), "%2E") || strings.Contains(path, "..") {
		c.Status(http.StatusNotFound)
		return
	}
	actor, reqID, initialToken, cookieID, ok := s.authenticateGoogleFlowDesktop(c, path)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")

	switch path {
	case "/turn":
		if err := s.googleFlowGUICheck(c.Request.Context(), actor, reqID); err != nil {
			c.Status(http.StatusForbidden)
			return
		}
		status, _, body, err := s.googleFlowManagementRequest(c.Request.Context(), actor, reqID, http.MethodPost, "/desktop/turn")
		if err != nil || status != http.StatusOK {
			c.Status(http.StatusBadGateway)
			return
		}
		safe, err := filterGoogleFlowTurnResponse(body)
		if err != nil {
			c.Status(http.StatusBadGateway)
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", safe)
		return
	case "/settings":
		if err := s.googleFlowGUICheck(c.Request.Context(), actor, reqID); err != nil {
			c.Status(http.StatusForbidden)
			return
		}
		status, _, body, err := s.googleFlowManagementRequest(c.Request.Context(), actor, reqID, http.MethodGet, "/desktop/settings")
		if err != nil || status != http.StatusOK {
			c.Status(http.StatusBadGateway)
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return
	default:
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		if err := s.googleFlowGUICheck(ctx, actor, reqID); err != nil {
			c.Status(http.StatusForbidden)
			return
		}
		target, ok := googleFlowOrigin("STUDIO_GOOGLE_FLOW_WEBRTC_ORIGIN", defaultGoogleFlowWebRTCOrigin)
		if !ok {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		isWS := strings.EqualFold(c.GetHeader("Upgrade"), "websocket")
		if !isWS && (path == "/" || path == "") && c.Query("path") == "" && c.GetHeader("Authorization") == "" {
			c.Redirect(http.StatusFound, "/apps/google-flow/desktop/?path=apps/google-flow/desktop/websockify&autoconnect=1&resize=remote")
			return
		}
		if path == "/webrtc/signalling/" {
			if !isWS {
				c.Status(http.StatusBadRequest)
				return
			}
			target.Path = "/ws"
		} else {
			if path == "" {
				target.Path = "/"
			} else {
				target.Path = path
			}
			target.RawQuery = c.Request.URL.RawQuery
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.ResponseHeaderTimeout = 10 * time.Second
		defer transport.CloseIdleConnections()
		interval := googleFlowRecheckInterval
		if interval <= 0 {
			interval = 5 * time.Second
		}
		proxy := &httputil.ReverseProxy{
			Transport: transport,
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL = target
				pr.Out.Host = target.Host
				pr.Out.Header = make(http.Header)
				if isWS {
					pr.Out.Header.Set("Connection", "Upgrade")
					pr.Out.Header.Set("Upgrade", "websocket")
					for _, key := range []string{"Origin", "Sec-WebSocket-Origin", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"} {
						if value := c.GetHeader(key); value != "" {
							pr.Out.Header.Set(key, value)
						}
					}
				} else {
					for _, key := range []string{"Accept", "Accept-Language", "Content-Type", "If-None-Match", "If-Modified-Since", "Range"} {
						if value := c.GetHeader(key); value != "" {
							pr.Out.Header.Set(key, value)
						}
					}
				}
			},
			ModifyResponse: func(resp *http.Response) error {
				resp.Header.Del("Set-Cookie")
				resp.Header.Del("Location")
				resp.Header.Set("Cache-Control", "no-store")
				resp.Header.Set("X-Content-Type-Options", "nosniff")
				if isWS {
					if resp.StatusCode != http.StatusSwitchingProtocols {
						return errors.New("google flow desktop upgrade failed")
					}
					go func(upstream io.ReadCloser) {
						defer upstream.Close()
						ticker := time.NewTicker(interval)
						defer ticker.Stop()
						for {
							select {
							case <-ctx.Done():
								return
							case <-ticker.C:
								curToken := initialToken
								if curToken == "" {
									curToken, _ = s.getSession(ctx, cookieID)
								}
								curActor, st := s.station.DashboardActor(ctx, curToken, reqID)
								if st != http.StatusOK || curActor.UserID != actor.UserID || curActor.OrganizationID != actor.OrganizationID || curActor.StationID != actor.StationID {
									return
								}
								if err := s.googleFlowGUICheck(ctx, curActor, reqID); err != nil {
									return
								}
							}
						}
					}(resp.Body)
				}
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				http.Error(w, "Google Flow desktop unavailable", http.StatusBadGateway)
			},
		}
		proxy.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
		return
	}
}
