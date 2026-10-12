package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/appstatehints"
)

type eventAuthorizer struct {
	appCenterWebStub
	mu     sync.Mutex
	scope  appcenter.Scope
	status int
	checks int
}

func (a *eventAuthorizer) AuthorizeEvents(_ context.Context, token string) (appcenter.Scope, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks++
	if token != "test-core-session" {
		return appcenter.Scope{}, 401
	}
	return a.scope, a.status
}

func eventFixture(t *testing.T) (*gin.Engine, *Server, *eventAuthorizer, chan appstatehints.RefreshEvent, <-chan struct{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
	s.sessions["test-cookie"] = session{token: "test-core-session", expires: time.Now().Add(time.Hour)}
	a := &eventAuthorizer{scope: appcenter.Scope{UserID: uuid.NewString(), OrganizationID: uuid.NewString(), StationID: uuid.NewString(), AuthorizationEpoch: strings.Repeat("a", 64)}, status: 200}
	expectedScope := a.scope
	events, closed := make(chan appstatehints.RefreshEvent, 16), make(chan struct{})
	s.appStateHints = func(ctx context.Context, scope appcenter.Scope) (<-chan appstatehints.RefreshEvent, error) {
		if scope != expectedScope {
			t.Error("subscription scope came from another identity")
		}
		go func() { <-ctx.Done(); close(closed) }()
		return events, nil
	}
	s.EnableAppCenter(r, a)
	return r, s, a, events, closed
}

func eventRequest(ctx context.Context, url string) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "test-cookie"})
	r.Header.Set("Origin", "https://studio.invalid")
	return r
}

func readEvent(t *testing.T, reader *bufio.Reader) appstatehints.RefreshEvent {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal("missing SSE refresh")
		}
		if strings.HasPrefix(line, "data: ") {
			var event appstatehints.RefreshEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || event.SchemaVersion != 1 {
				t.Fatal("invalid SSE shape")
			}
			return event
		}
	}
}

func TestAppCenterEventsHTTPStreamAndCancellation(t *testing.T) {
	r, _, a, events, closed := eventFixture(t)
	server := httptest.NewServer(r)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	response, err := server.Client().Do(eventRequest(ctx, server.URL+"/studio/appcenter/v1/events"))
	if err != nil {
		t.Fatal("SSE connection failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("invalid SSE headers")
	}
	reader := bufio.NewReader(response.Body)
	if readEvent(t, reader).Revision != "" {
		t.Fatal("missing initial calibration")
	}
	events <- appstatehints.RefreshEvent{SchemaVersion: 1, Revision: "boot-z:10"}
	events <- appstatehints.RefreshEvent{SchemaVersion: 1, Revision: "boot-a:1"}
	if readEvent(t, reader).Revision != "boot-a:1" {
		t.Fatal("hints not coalesced or revisions ordered")
	}
	response.Body.Close()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("cancellation did not close subscription")
	}
	a.mu.Lock()
	checks := a.checks
	a.mu.Unlock()
	if checks < 3 {
		t.Fatal("event was sent without current authorization")
	}
}

func TestAppCenterEventsRechecksAuthorizationBeforeHint(t *testing.T) {
	for _, variant := range []string{"logout", "epoch", "permission"} {
		t.Run(variant, func(t *testing.T) {
			r, s, a, events, closed := eventFixture(t)
			server := httptest.NewServer(r)
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			response, err := server.Client().Do(eventRequest(ctx, server.URL+"/studio/appcenter/v1/events"))
			if err != nil {
				t.Fatal("SSE failed")
			}
			defer response.Body.Close()
			reader := bufio.NewReader(response.Body)
			readEvent(t, reader)
			switch variant {
			case "logout":
				s.mu.Lock()
				delete(s.sessions, "test-cookie")
				s.mu.Unlock()
			case "epoch":
				a.mu.Lock()
				a.scope.AuthorizationEpoch = strings.Repeat("b", 64)
				a.mu.Unlock()
			case "permission":
				a.mu.Lock()
				a.status = 403
				a.mu.Unlock()
			}
			events <- appstatehints.RefreshEvent{SchemaVersion: 1, Revision: "must-not-be-sent"}
			raw, err := io.ReadAll(reader)
			if err != nil || strings.Contains(string(raw), "must-not-be-sent") {
				t.Fatal("revoked event escaped or stream failed to close")
			}
			select {
			case <-closed:
			case <-ctx.Done():
				t.Fatal("revoked subscription survived")
			}
		})
	}
}

func TestAppCenterEventsKeepaliveAfterWriteDeadline(t *testing.T) {
	r, _, a, events, _ := eventFixture(t)
	server := httptest.NewServer(r)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	response, err := server.Client().Do(eventRequest(ctx, server.URL+"/studio/appcenter/v1/events"))
	if err != nil {
		t.Fatal("SSE connection failed")
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	readEvent(t, reader)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal("idle stream failed before keepalive")
		}
		if line == ": keepalive\n" {
			break
		}
	}
	a.mu.Lock()
	checks := a.checks
	a.mu.Unlock()
	if checks < 3 {
		t.Fatal("keepalive did not reauthorize")
	}
	events <- appstatehints.RefreshEvent{SchemaVersion: 1, Revision: "after-idle"}
	if readEvent(t, reader).Revision != "after-idle" {
		t.Fatal("idle stream failed to resume hints")
	}
}

func TestAppCenterEventsRejectClientScopeAndUnauthorizedSubscription(t *testing.T) {
	for _, variant := range []string{"query", "bearer", "origin", "cookie", "permission"} {
		t.Run(variant, func(t *testing.T) {
			r, s, a, _, _ := eventFixture(t)
			s.appStateHints = func(context.Context, appcenter.Scope) (<-chan appstatehints.RefreshEvent, error) {
				t.Fatal("unauthorized subscription")
				return nil, nil
			}
			req := eventRequest(t.Context(), "/studio/appcenter/v1/events")
			switch variant {
			case "query":
				req.URL.RawQuery = "organization=other"
			case "bearer":
				req.Header.Set("Authorization", "Bearer client-token")
			case "origin":
				req.Header.Set("Origin", "https://other.invalid")
			case "cookie":
				req.AddCookie(&http.Cookie{Name: cookieName, Value: "second"})
			case "permission":
				a.status = 403
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if response.Code != 400 && response.Code != 401 && response.Code != 403 {
				t.Fatal("unsafe request not rejected")
			}
		})
	}
}
