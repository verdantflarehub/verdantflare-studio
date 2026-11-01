package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestImageResultServiceTransport(t *testing.T) {
	p := project.Principal{SubjectID: "01980000-0000-7000-8000-000000000001", OrganizationID: "01980000-0000-7000-8000-000000000002", RequestID: "01980000-0000-7000-8000-000000000003"}
	status, calls := 200, 0
	include := false
	body := `{"project_id":"project","artifact_id":"art-0123456789abcdef","sha256":"` + strings.Repeat("a", 64) + `","content_rating":{"rating":"unrated","source":null,"human_cleared":false}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/internal/results/read" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer test-service" || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID || r.Header.Get("X-Request-Id") != p.RequestID {
			t.Error("incorrect service request")
		}
		if (r.Header.Get("X-VF-Include-NSFW") == "1") != include {
			t.Error("incorrect UI preference")
		}
		var args map[string]string
		if json.NewDecoder(r.Body).Decode(&args) != nil || len(args) != 2 || args["project_id"] != "project" || args["artifact_id"] != "art-0123456789abcdef" {
			t.Error("incorrect input")
		}
		w.Header().Set("Location", "/must-not-follow")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()
	s := &Server{imageConfig: ImageConfig{URL: upstream.URL, Token: "test-service"}}
	for _, include = range []bool{false, true} {
		ctx := dashboard.WithIncludeNSFW(context.Background(), include)
		for _, status = range []int{200, 403, 404, 401, 302, 500} {
			before := calls
			value, got, err := s.readImageResult(ctx, p, "project", "art-0123456789abcdef")
			want := status
			if status == 401 || status == 302 || status == 500 {
				want = 503
			}
			if got != want || calls != before+1 {
				t.Fatalf("status %d returned %d, calls %d", status, got, calls-before)
			}
			if got == 200 && (err != nil || value.Rating.Rating != "unrated" || value.Rating.Source != nil) {
				t.Fatal("unrated response rejected")
			}
		}
	}
	status, include = 200, false
	// MCP must drop even a previously marked UI context at the transport boundary.
	if _, got, err := s.readMCPImageResult(dashboard.WithIncludeNSFW(context.Background(), true), p, "project", "art-0123456789abcdef"); got != 200 || err != nil {
		t.Fatal("MCP result read failed")
	}

	for _, body = range []string{`{}`, `{"project_id":null}`, strings.Repeat("x", 4097), `{"project_id":"p","project_id":"q"}`} {
		if _, got, err := s.readImageResult(context.Background(), p, "project", "art-0123456789abcdef"); got != 503 || err == nil {
			t.Fatal("malformed upstream accepted")
		}
	}
}
