package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var blenderGUISession = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

func (s *Server) blenderGUIControl(c *gin.Context) {
	if !blenderAlias.MatchString(c.Param("instance_alias")) || c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") || (c.Param("action") != "open" && c.Param("action") != "close") {
		c.Status(404)
		return
	}
	p, _, ok := s.authenticateMCP(c, false)
	if !ok {
		return
	}
	s.blenderForward(c, p, "/internal/gui/"+c.Param("instance_alias")+"/"+c.Param("action"))
}

func (s *Server) blenderGUICheck(ctx context.Context, p project.Principal, alias, sid string) (*url.URL, error) {
	u, ok := s.blenderOrigin()
	token := os.Getenv("STUDIO_BLENDER_SERVICE_TOKEN")
	if !ok || token == "" {
		return nil, errors.New("GUI service unavailable")
	}
	u.Path = "/internal/gui/" + alias + "/check"
	body, _ := json.Marshal(map[string]string{"editing_session_id": sid})
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("GUI lease unavailable")
	}
	var result struct {
		Origin string `json:"gui_origin"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil {
		return nil, errors.New("GUI response invalid")
	}
	target, err := url.Parse(result.Origin)
	if err != nil || target.Host == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Path != "" || target.RawQuery != "" || target.Fragment != "" {
		return nil, errors.New("GUI target invalid")
	}
	return target, nil
}

func (s *Server) blenderDesktop(c *gin.Context) {
	alias, sid, path := c.Param("instance_alias"), c.Param("editing_session_id"), c.Param("path")
	if !blenderAlias.MatchString(alias) || !blenderGUISession.MatchString(sid) || c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") || strings.Contains(path, "..") {
		c.Status(404)
		return
	}
	p, _, ok := s.authenticateMCP(c, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	initialToken := strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer ")
	cookieID := ""
	if cookie, err := c.Request.Cookie(cookieName); err == nil {
		cookieID = cookie.Value
	}
	websocket := path == "/webrtc/signalling/"
	var target *url.URL
	if websocket {
		if !strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			c.Status(400)
			return
		}
		var err error
		target, err = s.blenderGUICheck(ctx, p, alias, sid)
		if err != nil {
			c.Status(403)
			return
		}
		target.Path = "/ws"
	} else {
		target, ok = s.blenderOrigin()
		if !ok {
			c.Status(503)
			return
		}
		if path == "/settings" || path == "/turn" {
			target.Path = "/internal/gui-config/" + alias + "/" + sid + path
		} else {
			target.Path = "/internal/gui-assets/" + alias + "/" + sid + path
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 10 * time.Second
	defer transport.CloseIdleConnections()
	proxy := &httputil.ReverseProxy{Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL = target
			pr.Out.Host = target.Host
			pr.Out.Header = make(http.Header)
			if websocket {
				pr.Out.Header.Set("Connection", "Upgrade")
				pr.Out.Header.Set("Upgrade", "websocket")
				for _, key := range []string{"Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"} {
					if value := c.GetHeader(key); value != "" {
						pr.Out.Header.Set(key, value)
					}
				}
			} else {
				pr.Out.Header.Set("Authorization", "Bearer "+os.Getenv("STUDIO_BLENDER_SERVICE_TOKEN"))
				pr.Out.Header.Set("X-User-Id", p.SubjectID)
				pr.Out.Header.Set("X-Organization-Id", p.OrganizationID)
				pr.Out.Header.Set("X-Request-Id", p.RequestID)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			resp.Header.Del("Location")
			resp.Header.Set("Cache-Control", "no-store")
			resp.Header.Set("X-Content-Type-Options", "nosniff")
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				return errors.New("GUI redirect denied")
			}
			if websocket {
				if resp.StatusCode != 101 {
					return errors.New("GUI upgrade failed")
				}
				// Closing this stream closes the hijacked client connection as well.
				// The worker watchdog separately terminates WebRTC input on lease loss.
				go func() {
					defer resp.Body.Close()
					ticker := time.NewTicker(5 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							token := initialToken
							if token == "" {
								token, _ = s.getSession(ctx, cookieID)
							}
							identity, status := s.station.Principal(ctx, token, p.RequestID)
							if status != 200 || identity.SubjectID != p.SubjectID || identity.OrganizationID != p.OrganizationID {
								return
							}
							if _, err := s.blenderGUICheck(ctx, identity, alias, sid); err != nil {
								return
							}
						}
					}
				}()
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "Blender desktop unavailable", 502)
		},
	}
	proxy.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
}
