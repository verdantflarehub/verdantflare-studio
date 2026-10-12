package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

// Uses an isolated real gateway listener. Run on the documented dev host.
func TestDesktopLegacyBlenderDateRelay(t *testing.T) {
	for _, date := range []string{"Mon, 12 Oct 2026 00:00:00 GMT", "", "invalid"} {
		_, fixture, openBody, _ := blenderDesktopFixture(t)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header()["Date"] = nil
			if date != "" {
				w.Header().Set("Date", date)
			}
			switch r.URL.Path {
			case "/api/login":
				http.SetCookie(w, &http.Cookie{Name: "vf_studio_session", Value: "fixture-only", HttpOnly: true, Path: "/"})
				w.WriteHeader(201)
				_, _ = io.WriteString(w, `{}`)
			case "/studio/appcenter/v1/open":
				_ = json.NewEncoder(w).Encode(fixture.opened)
			case "/studio/apps/blender/instances":
				_, _ = io.WriteString(w, `{"schema_version":1,"instances":[]}`)
			default:
				w.WriteHeader(404)
			}
		}))
		remote, err := studioremote.New(upstream.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = remote.Send(context.Background(), "POST", "/api/login", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		handler := RemoteAssetHandler(NewRemote(remote), http.NotFoundHandler())
		request := httptest.NewRequest("POST", "http://wails.localhost"+blenderWorkspacePrefix, bytes.NewReader(openBody))
		request.Header.Set("Content-Type", "application/json")
		opened := httptest.NewRecorder()
		handler.ServeHTTP(opened, request)
		if opened.Code != 200 {
			t.Fatal("workspace open failed", opened.Code)
		}
		prefix := blenderWorkspacePrefix + "/" + opened.Header().Get("X-Studio-Legacy-Workspace")
		relay := httptest.NewServer(handler)
		response, err := http.Post(relay.URL+prefix+"/requests", "application/json", bytes.NewReader(blenderEnvelope("GET", "/studio/apps/blender/instances", "")))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		relay.Close()
		remote.Close()
		upstream.Close()
		if response.StatusCode != 200 || response.Header.Get("Date") != date {
			t.Fatal("Blender relay replaced upstream observation time", response.StatusCode)
		}
	}
}

func TestDesktopLegacyBlenderGatewayHTTP(t *testing.T) {
	for _, mode := range []string{"close", "binding"} {
		t.Run(mode, func(t *testing.T) {
			_, fixture, openBody, _ := blenderDesktopFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var changed atomic.Bool
			var closes, checks atomic.Int32
			disconnected, released := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("browser credential exposed")
					w.WriteHeader(403)
					return
				}
				if r.URL.Path == "/api/login" {
					http.SetCookie(w, &http.Cookie{Name: "vf_studio_session", Value: "fixture-only", HttpOnly: true, Path: "/"})
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(201)
					_, _ = io.WriteString(w, `{"user_id":"fixture"}`)
					return
				}
				cookie, err := r.Cookie("vf_studio_session")
				if err != nil || cookie.Value != "fixture-only" {
					t.Error("original Go login missing")
					w.WriteHeader(401)
					return
				}
				switch r.URL.Path {
				case "/studio/appcenter/v1/open":
					checks.Add(1)
					raw, _ := io.ReadAll(r.Body)
					if !bytes.Equal(raw, openBody) {
						t.Error("original AppCenter request changed")
					}
					value := fixture.opened
					if changed.Load() {
						value.BindingID = "replacement"
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(value)
				case "/studio/apps/blender/blenderA/open":
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{"editing_session_id": strings.Repeat("s", 43)})
				case "/studio/apps/blender/blenderA/close":
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["editing_session_id"] != strings.Repeat("s", 43) {
						t.Error("cleanup editing session changed")
					}
					closes.Add(1)
					w.WriteHeader(204)
					close(released)
				case "/apps/blender/blenderA/" + strings.Repeat("s", 43) + "/webrtc/signalling/":
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						t.Error("upgrade failed")
						return
					}
					defer conn.CloseNow()
					defer close(disconnected)
					if conn.Write(ctx, websocket.MessageText, []byte("HELLO original")) != nil {
						return
					}
					kind, data, err := conn.Read(ctx)
					if err != nil || kind != websocket.MessageText || string(data) != "SESSION original" {
						t.Error("message did not round trip")
						return
					}
					_ = conn.Write(ctx, kind, data)
					_, _, _ = conn.Read(ctx)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			remote, err := studioremote.New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer remote.Close()
			if _, err := remote.Send(ctx, "POST", "/api/login", []byte(`{"username":"fixture","password":"test-only"}`)); err != nil {
				t.Fatal(err)
			}
			h := RemoteAssetHandler(NewRemote(remote), http.NotFoundHandler())
			call := func(method, path string, body []byte) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "http://wails.localhost"+path, bytes.NewReader(body)).WithContext(ctx)
				if body != nil {
					r.Header.Set("Content-Type", "application/json")
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				return w
			}
			w := call("POST", blenderWorkspacePrefix, openBody)
			if w.Code != 200 {
				t.Fatal("workspace open", w.Code)
			}
			prefix := blenderWorkspacePrefix + "/" + w.Header().Get("X-Studio-Legacy-Workspace")
			if call("POST", prefix+"/requests", blenderEnvelope("POST", "/studio/apps/blender/blenderA/open", `{}`)).Code != 200 {
				t.Fatal("edit open failed")
			}
			data, _ := json.Marshal(map[string]string{"alias": "blenderA", "editingSession": strings.Repeat("s", 43)})
			w = call("POST", prefix+"/signals", data)
			if w.Code != 200 {
				t.Fatal("signal open", w.Code, w.Body.String())
			}
			var opened map[string]string
			_ = json.Unmarshal(w.Body.Bytes(), &opened)
			signal := prefix + "/signals/" + opened["signalId"]
			w = call("GET", signal, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "HELLO original") {
				t.Fatal("upstream message missing", w.Code)
			}
			if call("POST", signal, []byte(`{"message":"SESSION original"}`)).Code != 204 {
				t.Fatal("outbound message failed")
			}
			w = call("GET", signal, nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "SESSION original") {
				t.Fatal("message echo missing")
			}
			if mode == "close" {
				if call("DELETE", prefix, nil).Code != 204 {
					t.Fatal("workspace close failed")
				}
				select {
				case <-released:
				case <-ctx.Done():
					t.Fatal("original editing lease not released")
				}
				if closes.Load() != 1 {
					t.Fatal("cleanup repeated")
				}
			} else {
				changed.Store(true)
				if call("GET", signal, nil).Code != 409 {
					t.Fatal("binding change ignored")
				}
			}
			select {
			case <-disconnected:
			case <-ctx.Done():
				t.Fatal("upstream socket remained open")
			}
			if call("GET", signal, nil).Code != 410 {
				t.Fatal("closed local signal still available")
			}
			if checks.Load() < 6 {
				t.Fatal("entry was not rechecked")
			}
			if mode == "binding" && closes.Load() != 0 {
				t.Fatal("cleanup wrote against changed binding")
			}
		})
	}
}
