package desktop

import (
	"encoding/json"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopCredentialIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/login" {
			w.WriteHeader(201)
			io.WriteString(w, `{"access_token":"private-session"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer private-session" {
			t.Error("credential missing")
		}
		if r.URL.Path == "/identity/logout" {
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()
	station, _ := application.NewStation(server.URL)
	service := New(station)
	result := service.Call(application.Request{Method: "POST", Path: "login", Body: json.RawMessage(`{}`)})
	encoded, _ := json.Marshal(result)
	if result.Status != 201 || strings.Contains(string(encoded), "private-session") || result.Token != "" {
		t.Fatal("login leaked credential")
	}
	if service.Call(application.Request{Method: "GET", Path: "apps"}).Status != 200 {
		t.Fatal("session unavailable")
	}
	service.Call(application.Request{Method: "POST", Path: "logout"})
	if service.Call(application.Request{Method: "GET", Path: "apps"}).Status != 401 {
		t.Fatal("session not cleared")
	}
}
