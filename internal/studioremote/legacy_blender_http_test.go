package studioremote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Run on the dev test host: this test creates real HTTP/WebSocket listeners.
// It verifies desktop transport, not a native Wails GUI or live Blender worker.
func TestLegacyBlenderHTTPAndSignalLifecycle(t *testing.T) {
	for _, mode := range []string{"close", "logout", "new-login", "cancel", "server-close", "binary", "oversize", "bad-write", "http-cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			disconnected, started := make(chan struct{}), make(chan struct{})
			var origin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Origin") != origin || r.Header.Get("Authorization") != "" {
					t.Error("incorrect origin or exposed credential")
					w.WriteHeader(403)
					return
				}
				if r.URL.Path == "/api/login" {
					http.SetCookie(w, &http.Cookie{Name: "vf_studio_session", Value: "test-session-only", HttpOnly: true, Path: "/"})
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(201)
					fmt.Fprint(w, `{"user_id":"fixture"}`)
					return
				}
				cookie, err := r.Cookie("vf_studio_session")
				if err != nil || cookie.Value != "test-session-only" {
					t.Error("Go session cookie missing")
					w.WriteHeader(401)
					return
				}
				if r.URL.Path == "/api/logout" {
					w.WriteHeader(204)
					return
				}
				if r.URL.Path == "/studio/apps/blender/instances" {
					if mode == "http-cancel" {
						close(started)
						<-r.Context().Done()
						close(disconnected)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"instances": []any{}})
					return
				}
				if r.URL.Path != "/apps/blender/blenderA/"+strings.Repeat("s", 43)+"/webrtc/signalling/" {
					t.Error("unexpected path")
					w.WriteHeader(404)
					return
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Error("upgrade failed")
					return
				}
				defer conn.CloseNow()
				defer close(disconnected)
				if mode == "binary" {
					_ = conn.Write(ctx, websocket.MessageBinary, []byte("no-binary"))
				} else if mode == "oversize" {
					_ = conn.Write(ctx, websocket.MessageText, []byte(strings.Repeat("x", blenderSignalLimit+1)))
				} else {
					_ = conn.Write(ctx, websocket.MessageText, []byte("HELLO"))
					kind, data, err := conn.Read(ctx)
					if err != nil || kind != websocket.MessageText || string(data) != "SESSION original" {
						t.Error("message altered or missing")
						return
					}
					_ = conn.Write(ctx, websocket.MessageText, data)
					if mode == "server-close" {
						_ = conn.Close(websocket.StatusPolicyViolation, "lease ended")
						return
					}
				}
				_, _, _ = conn.Read(ctx)
			}))
			defer server.Close()
			origin = server.URL
			client, err := New(origin)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			login(t, client)
			session, _ := client.Session()
			if mode == "http-cancel" {
				readCtx, stop := context.WithCancel(ctx)
				defer stop()
				done := make(chan error, 1)
				go func() {
					_, err := session.LegacyBlender(readCtx, "GET", "/studio/apps/blender/instances", nil)
					done <- err
				}()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("request never started")
				}
				stop()
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("cancelled read succeeded")
					}
				case <-ctx.Done():
					t.Fatal("read did not cancel")
				}
			} else {
				out, err := session.LegacyBlender(ctx, "GET", "/studio/apps/blender/instances", nil)
				if err != nil || out.Status != 200 {
					t.Fatal("authenticated JSON read failed", err)
				}
				signalCtx, stop := context.WithCancel(ctx)
				defer stop()
				signal, err := session.OpenBlenderSignal(signalCtx, "blenderA", strings.Repeat("s", 43))
				if err != nil {
					t.Fatal("authenticated signal open failed", err)
				}
				defer signal.Close()
				if mode != "binary" && mode != "oversize" {
					data, err := signal.Read(ctx)
					if err != nil || string(data) != "HELLO" {
						t.Fatal("signal closed after handshake", err)
					}
					if err := signal.Write(ctx, []byte("SESSION original")); err != nil {
						t.Fatal("write failed", err)
					}
					data, err = signal.Read(ctx)
					if err != nil || string(data) != "SESSION original" {
						t.Fatal("signal did not round trip", err)
					}
				}
				switch mode {
				case "close":
					signal.Close()
				case "logout":
					if _, err := client.Send(ctx, "POST", "/api/logout", nil); err != nil {
						t.Fatal(err)
					}
				case "new-login":
					login(t, client)
				case "cancel":
					stop()
				case "bad-write":
					if err := signal.Write(ctx, []byte(strings.Repeat("x", blenderSignalLimit+1))); err == nil {
						t.Fatal("oversized write accepted")
					}
				}
				if mode == "close" || mode == "logout" || mode == "new-login" || mode == "cancel" || mode == "bad-write" {
					// Closing must reach the upstream even when the caller is not reading.
					select {
					case <-disconnected:
					case <-ctx.Done():
						t.Fatal("idle signal was not closed")
					}
				}
				if _, err := signal.Read(ctx); err == nil {
					t.Fatal("closed or invalid signal remained readable")
				}
				if err := signal.Write(ctx, []byte("do-not-replay")); err == nil {
					t.Fatal("closed signal remained writable")
				}
			}
			select {
			case <-disconnected:
			case <-ctx.Done():
				t.Fatal("upstream request did not end")
			}
			client.mu.Lock()
			active := len(client.transfers)
			client.mu.Unlock()
			if active != 0 {
				t.Fatal("transfer registration leaked")
			}
		})
	}
}
