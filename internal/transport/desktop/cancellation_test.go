package desktop

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

// This test opens a listener and runs only in the development environment.
func TestDesktopCallCancellationReachesHTTP(t *testing.T) {
	for _, remote := range []bool{false, true} {
		name := "core"
		if remote {
			name = "studio"
		}
		t.Run(name, func(t *testing.T) {
			started, cancelled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/login") {
					http.SetCookie(w, &http.Cookie{Name: "vf_studio_session", Value: "test-session", Path: "/", HttpOnly: true})
					w.WriteHeader(201)
					_, _ = io.WriteString(w, `{"access_token":"test-session"}`)
					return
				}
				close(started)
				select {
				case <-r.Context().Done():
					close(cancelled)
				case <-time.After(5 * time.Second):
				}
			}))
			defer server.Close()
			var service *Service
			if remote {
				client, err := studioremote.New(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				service = NewRemote(client)
			} else {
				station, err := application.NewStation(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				service = New(station)
			}
			if out := service.Call(context.Background(), application.Request{Method: "POST", Path: "login"}); out.Status != 201 {
				t.Fatal("login failed")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan application.Result, 1)
			go func() { done <- service.Call(ctx, application.Request{Method: "GET", Path: "apps"}) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not start")
			}
			cancel()
			select {
			case <-cancelled:
			case <-time.After(3 * time.Second):
				t.Fatal("upstream did not observe cancellation")
			}
			select {
			case out := <-done:
				if out.Status != 503 || out.Token != "" {
					t.Fatal("cancelled result was accepted")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("binding did not return")
			}
		})
	}
}

type cancellationTransport func(*http.Request) (*http.Response, error)

func (f cancellationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDesktopCallIgnoresCancelledSessionResponses(t *testing.T) {
	// The direct Core client uses DefaultTransport. No listener or network is used.
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, item := range []struct {
		path      string
		status    int
		wantToken string
	}{
		{"login", 201, "original"}, {"apps", 401, "original"}, {"logout", 204, ""},
	} {
		t.Run(item.path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			http.DefaultTransport = cancellationTransport(func(*http.Request) (*http.Response, error) {
				cancel()
				return &http.Response{StatusCode: item.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"late-session"}`))}, nil
			})
			station, err := application.NewStation("https://core.invalid")
			if err != nil {
				t.Fatal(err)
			}
			service := New(station)
			service.token = "original"
			method := "POST"
			if item.path == "apps" {
				method = "GET"
			}
			out := service.Call(ctx, application.Request{Method: method, Path: item.path})
			if out.Status != 503 || out.Token != "" || service.token != item.wantToken {
				t.Fatal("cancelled response changed session or exposed credential")
			}
		})
	}
}

func TestDesktopCallCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := New(nil)
	if out := service.Call(ctx, application.Request{Method: "GET", Path: "apps"}); out.Status != 503 {
		t.Fatal("cancelled request was dispatched")
	}
}
