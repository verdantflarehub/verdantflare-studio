package project_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestInternalMCPAuthenticationAndNoNotificationWrites(t *testing.T) {
	// No database: rejected protocol requests must never dispatch business work.
	service := new(project.Service)
	h, e := service.MCPHandler(strings.Repeat("s", 32), strings.Repeat("a", 32))
	if e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name, body, token string
		status            int
	}{
		{"no internal credential", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "", 403},
		{"list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, strings.Repeat("s", 32), 200},
		{"notification write", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"project.create","arguments":{}}}`, strings.Repeat("s", 32), 400},
		{"duplicate argument", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project.create","arguments":{"name":"a","name":"b"}}}`, strings.Repeat("s", 32), 400},
		{"unregistered action", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project.delete","arguments":{}}}`, strings.Repeat("s", 32), 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/mcp", strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+test.token)
			r.Header.Set("X-User-Id", id())
			r.Header.Set("X-Organization-Id", id())
			r.Header.Set("X-Request-Id", id())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
