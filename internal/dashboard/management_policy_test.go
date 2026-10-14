package dashboard

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func managementFixture(t *testing.T) *registeredFixture {
	t.Helper()
	f := newRegisteredFixture(t)
	f.registry.Releases[0].Rules = append(f.registry.Releases[0].Rules,
		Rule{Method: "GET", Path: "/membership", UpstreamPath: "/internal/membership", RequireManage: true})
	f.responses["/internal/membership"] = []byte(`{"credits":"100"}`)
	f.write(t)
	return f
}

func TestManagementReadRequiresCurrentPermission(t *testing.T) {
	f := managementFixture(t)
	r := New(f.source)
	launch := openFixture(t, r, f.actor)
	binding, err := r.AuthorizeRequest(context.Background(), f.actor, launch.DashboardSessionID, "GET", "/membership", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, status, err := f.source.Request(context.Background(), f.actor, binding,
		AppRequest{Method: "GET", Path: "/membership"}, "request-one"); err != nil || status != 200 {
		t.Fatal("manager could not read membership", status, err)
	}

	// No actor or installation revision change is needed to enforce revocation.
	f.installed.CanManage = false
	if _, err := r.AuthorizeRequest(context.Background(), f.actor, launch.DashboardSessionID, "GET", "/membership", 0); err == nil {
		t.Fatal("revoked manager kept an authorized session")
	}
	f.installed.CanManage = true
	if _, err := r.Check(context.Background(), f.actor, launch.DashboardSessionID); !errors.Is(err, ErrExpired) {
		t.Fatal("restoring permission revived an old session", err)
	}

	f.installed.CanManage = false
	launch = openFixture(t, r, f.actor)
	if _, err := r.AuthorizeRequest(context.Background(), f.actor, launch.DashboardSessionID, "GET", "/instances", 0); err != nil {
		t.Fatal("ordinary read compatibility lost", err)
	}
	if _, err := r.AuthorizeRequest(context.Background(), f.actor, launch.DashboardSessionID, "GET", "/membership", 0); !errors.Is(err, ErrDenied) {
		t.Fatal("read-only user accessed membership", err)
	}
	// Even a direct request with the freshly resolved narrowed binding is denied
	// before a business request reaches the upstream service.
	binding, err = r.Check(context.Background(), f.actor, launch.DashboardSessionID)
	if err != nil {
		t.Fatal(err)
	}
	calls := f.calls
	if _, _, err := f.source.Request(context.Background(), f.actor, binding,
		AppRequest{Method: "GET", Path: "/membership"}, "request-two"); !errors.Is(err, ErrDenied) || f.calls != calls {
		t.Fatal("denied membership request reached upstream", err)
	}
}

func TestManagementPolicyRejectsNonTrueAndMisspelledFields(t *testing.T) {
	for _, replacement := range []string{
		`"requireManage":false`, `"requireManage":null`, `"requireManage":1`,
		`"requireManage":"true"`, `"RequireManage":true`,
		`"requireManage":true,"requireManage":true`,
	} {
		t.Run(replacement, func(t *testing.T) {
			f := managementFixture(t)
			data := strings.Replace(string(marshalFixture(t, f.registry)), `"requireManage":true`, replacement, 1)
			if err := os.WriteFile(f.source.path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.source.registry(); !errors.Is(err, ErrInvalid) {
				t.Fatal("malformed management policy accepted", err)
			}
		})
	}
}
