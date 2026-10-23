package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReviewOriginRequiresAuthenticatedContextAndExactAppRoute(t *testing.T) {
	for _, tc := range []struct {
		app, path, upstream, want string
		interactive               bool
	}{
		{"project", "/api/v1/project/review/append", "/api/v1/project/review/append", "human_interactive", true},
		{"world", "/api/v1/world/review/open", "/api/v1/world/review/open", "human_interactive", true},
		{"project", "/api/v1/project/review/append", "/api/v1/project/review/append", "agent", false},
		{"sample", "/api/v1/project/review/append", "/api/v1/project/review/append", "", true},
		{"project", "/api/v1/world/review/append", "/api/v1/world/review/append", "", true},
		{"project", "/api/v1/project/review/append", "/other", "", true},
		{"project", "/api/v1/project/edit", "/api/v1/project/edit", "", true},
	} {
		t.Run(tc.app+tc.path+tc.want+tc.upstream, func(t *testing.T) {
			f := newRegisteredFixture(t)
			binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
			if err != nil {
				t.Fatal(err)
			}
			binding.AppID = tc.app
			binding.Declaration.AppID = tc.app
			binding.Rules = []Rule{{Method: "POST", Path: tc.path, UpstreamPath: tc.upstream, MaxBody: 1024}}
			f.source.client.Transport = sourceTransport(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("X-Review-Origin") != tc.want || req.Header.Get("Cookie") != "" {
					t.Fatal("untrusted review origin")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})
			ctx := context.Background()
			if tc.interactive {
				ctx = WithInteractiveReview(ctx)
			}
			if _, _, err := f.source.Request(ctx, f.actor, binding, AppRequest{Method: "POST", Path: tc.path, Body: []byte(`{}`)}, "review-request"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNonManagerCanReachOnlyExactContentBusinessRules(t *testing.T) {
	for _, app := range []string{"project", "world"} {
		f := newRegisteredFixture(t)
		f.installed.AppID, f.installed.CanManage = app, false
		release := &f.registry.Releases[0]
		release.AppID = app
		var declaration Declaration
		if json.Unmarshal(f.responses[release.Backend.DeclarationPath], &declaration) != nil {
			t.Fatal("fixture")
		}
		declaration.AppID = app
		data := marshalFixture(t, declaration)
		f.responses[release.Backend.DeclarationPath] = data
		release.DeclarationDigest = sum(data)
		prefix := "/api/v1/" + app + "/"
		release.Rules = []Rule{}
		for _, op := range []string{"review/open", "review/append", "review/status", "list"} {
			release.Rules = append(release.Rules, Rule{Method: "POST", Path: prefix + op, UpstreamPath: prefix + op, MaxBody: 1024})
		}
		release.Rules = append(release.Rules,
			Rule{Method: "POST", Path: prefix + "edit", UpstreamPath: prefix + "edit", MaxBody: 1024},
			Rule{Method: "POST", Path: prefix + "review/open/:id", UpstreamPath: prefix + "review/open/:id", MaxBody: 1024},
			Rule{Method: "POST", Path: "/admin", UpstreamPath: prefix + "review/open", MaxBody: 1024},
			Rule{Method: "GET", Path: "/management", UpstreamPath: "/management", RequireManage: true})
		f.write(t)
		binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
		if err != nil || len(binding.Rules) != 4 {
			t.Fatal("business permissions mixed with installation management", err, len(binding.Rules))
		}
		for _, rule := range binding.Rules {
			if !strings.HasPrefix(rule.Path, prefix+"review/") && rule.Path != prefix+"list" {
				t.Fatal("unexpected rule retained")
			}
		}
		for _, rule := range []Rule{
			{Method: "POST", Path: prefix + "review/open", UpstreamPath: prefix + "review/open", RequireManage: true},
			{Method: "POST", Path: prefix + "review/open", UpstreamPath: "/other"},
		} {
			if contentBusinessRule(app, rule) {
				t.Fatal("explicit restriction bypassed")
			}
		}
	}
}
