package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/appstatehints"
)

type appCenterEventAuthorizer interface {
	AuthorizeEvents(context.Context, string) (appcenter.Scope, int)
}

func (s *Server) appCenterEvents(c *gin.Context, cookie, token string, authorizer appCenterEventAuthorizer) {
	fail := func(status int, code string) {
		id := application.ID()
		c.Header("X-Request-ID", id)
		c.JSON(status, gin.H{"code": code, "requestId": id})
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	initial, status := authorizer.AuthorizeEvents(ctx, token)
	if status != 200 {
		if status == 401 {
			fail(401, "UNAUTHENTICATED")
		} else if status == 403 {
			fail(403, "PERMISSION_DENIED")
		} else {
			fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		}
		return
	}
	if _, err := appstatehints.Key(initial.StationID, initial.OrganizationID); err != nil {
		fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		return
	}
	s.mu.Lock()
	client, subscribe := s.etcd, s.appStateHints
	s.mu.Unlock()
	if subscribe == nil {
		subscribe = func(ctx context.Context, scope appcenter.Scope) (<-chan appstatehints.RefreshEvent, error) {
			return appstatehints.Subscribe(ctx, client, scope.StationID, scope.OrganizationID)
		}
	}
	events, err := subscribe(ctx, initial)
	if err != nil {
		fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		return
	}
	controller := http.NewResponseController(c.Writer)
	if controller.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
		fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	c.Header("X-Request-ID", application.ID())
	valid := func() bool {
		current, ok := s.getSession(ctx, cookie)
		if !ok || current != token {
			return false
		}
		scope, status := authorizer.AuthorizeEvents(ctx, token)
		return status == 200 && scope == initial
	}
	write := func(event *appstatehints.RefreshEvent) bool {
		if controller.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
			return false
		}
		if event == nil {
			_, err = fmt.Fprint(c.Writer, ": keepalive\n\n")
		} else {
			raw, encodeErr := json.Marshal(event)
			if encodeErr != nil {
				return false
			}
			_, err = fmt.Fprintf(c.Writer, "event: refresh\ndata: %s\n\n", raw)
		}
		return err == nil && controller.Flush() == nil
	}
	// Every connection requests a fresh view after the subscription barrier.
	if !valid() || !write(&appstatehints.RefreshEvent{SchemaVersion: 1}) {
		return
	}
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	throttle := time.NewTicker(time.Second)
	defer throttle.Stop()
	var pending *appstatehints.RefreshEvent
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				if valid() {
					_ = write(&appstatehints.RefreshEvent{SchemaVersion: 1})
				}
				return
			}
			pending = &event
		case <-throttle.C:
			if pending != nil {
				if !valid() || !write(pending) {
					return
				}
				pending = nil
			}
		case <-heartbeat.C:
			if !valid() || !write(nil) {
				return
			}
		}
	}
}
