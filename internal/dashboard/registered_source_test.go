package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// All transport and installation responses are in memory. These fixtures never
// establish trust for a deployed release and do not contact a local service.
type registeredFixture struct {
	source    *RegisteredSource
	actor     Actor
	installed InstalledRelease
	registry  Registry
	responses map[string][]byte
	calls     int
	status    int
	mime      string
}

func marshalFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newRegisteredFixture(t *testing.T) *registeredFixture {
	t.Helper()
	actor, binding := fixture()
	backend := Backend{Origin: "http://sample.apps.svc.cluster.local:8080", TokenEnv: "STUDIO_APP_SAMPLE_TOKEN",
		DeclarationPath: "/internal/dashboard-declaration", IndexPath: "/internal/dashboard-index",
		AssetPath: "/internal/dashboard/", AssetPrefix: "dashboard/"}
	asset := []byte("<!doctype html><p>independent dashboard</p>")
	index := marshalFixture(t, assetIndex{SchemaVersion: 1, Files: map[string]string{"console.html": strings.TrimPrefix(sum(asset), "sha256:")}})
	binding.Declaration.Dashboard.ArtifactDigest = sum(index)
	declaration := marshalFixture(t, binding.Declaration)
	f := &registeredFixture{actor: actor, installed: InstalledRelease{InstallationID: binding.InstallationID,
		AppID: binding.AppID, AppVersion: binding.AppVersion, StationID: actor.StationID,
		OrganizationID: actor.OrganizationID, State: "installed", Revision: 1, CanManage: true},
		registry: Registry{SchemaVersion: 1, Releases: []RegisteredRelease{{AppID: binding.AppID, AppVersion: binding.AppVersion,
			DeclarationDigest: sum(declaration), ArtifactDigest: sum(index), AppAPIPolicyRef: binding.Declaration.Dashboard.AppAPIPolicyRef,
			Backend: backend, Grants: binding.Grants, Rules: binding.Rules}}},
		responses: map[string][]byte{backend.DeclarationPath: declaration, backend.IndexPath: index, backend.AssetPath + "console.html": asset}}
	f.source = NewRegisteredSource(filepath.Join(t.TempDir(), "registry.json"), func(_ context.Context, got Actor, id string) (InstalledRelease, error) {
		if got != f.actor || id != f.installed.InstallationID {
			return InstalledRelease{}, ErrDenied
		}
		return f.installed, nil
	})
	f.source.credential = func(name string) string {
		if name != "STUDIO_APP_SAMPLE_TOKEN" {
			t.Fatal("unexpected credential reference")
		}
		return "unit-only-service-credential-000000000"
	}
	f.source.client.Transport = sourceTransport(func(req *http.Request) (*http.Response, error) {
		f.calls++
		if req.URL.Scheme != "http" || req.URL.Host != "sample.apps.svc.cluster.local:8080" || req.Method != "GET" ||
			req.Header.Get("X-User-Id") != actor.UserID || req.Header.Get("X-Organization-Id") != actor.OrganizationID ||
			req.Header.Get("Authorization") != "Bearer unit-only-service-credential-000000000" || req.Header.Get("Cookie") != "" {
			t.Fatal("incorrect controlled request")
		}
		data, ok := f.responses[req.URL.Path]
		if !ok {
			t.Fatal("unregistered upstream path")
		}
		mime := "application/json"
		if strings.HasSuffix(req.URL.Path, ".html") {
			mime = "text/html"
		}
		if f.mime != "" {
			mime = f.mime
		}
		status := http.StatusOK
		if f.status != 0 {
			status = f.status
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)),
			Header: http.Header{"Content-Type": {mime + "; charset=utf-8"}, "Location": {"https://elsewhere.invalid/"}}, Request: req}, nil
	})
	t.Cleanup(f.source.Close)
	f.write(t)
	return f
}

func (f *registeredFixture) write(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(f.source.path, marshalFixture(t, f.registry), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryAcceptsAccumulatedReleasesAndStillEnforcesCap(t *testing.T) {
	f := newRegisteredFixture(t)
	base := f.registry.Releases[0]
	fill := func(n int) {
		t.Helper()
		f.registry.Releases = make([]RegisteredRelease, n)
		for i := range f.registry.Releases {
			item := base
			item.AppVersion = fmt.Sprintf("1.0.%d", i)
			f.registry.Releases[i] = item
		}
		f.write(t)
	}
	fill(129)
	if _, _, err := f.source.registry(); err != nil {
		t.Fatal("accumulated releases rejected", err)
	}
	fill(maxRegistryReleases + 1)
	if _, _, err := f.source.registry(); !errors.Is(err, ErrInvalid) {
		t.Fatal("release cap not enforced", err)
	}
}

func TestRegisteredReleaseBindsOriginalBytesAndKeepsBackendPrivate(t *testing.T) {
	f := newRegisteredFixture(t)
	r := New(f.source)
	launch := openFixture(t, r, f.actor)
	binding, err := r.Check(context.Background(), f.actor, launch.DashboardSessionID)
	if err != nil {
		t.Fatal(err)
	}
	data, mime, err := f.source.Asset(context.Background(), f.actor, binding, "dashboard/console.html")
	if err != nil || mime != "text/html; charset=utf-8" || !bytes.Equal(data, f.responses["/internal/dashboard/console.html"]) {
		t.Fatal("asset not verified", err)
	}
	wire := string(marshalFixture(t, launch))
	for _, private := range []string{"svc.cluster.local", "STUDIO_APP_", "credential", "upstreamPath", "/internal/", "backend"} {
		if strings.Contains(wire, private) {
			t.Fatal("backend configuration escaped into launch")
		}
	}
	// Release registration, rather than any app-name branch, controls entry.
	if !strings.HasSuffix(launch.EntryURL, "/assets/dashboard/console.html") {
		t.Fatal("entry lost")
	}
}

func TestRegisteredSourceRejectsTamperingAndUnlistedAssets(t *testing.T) {
	for _, kind := range []string{"declaration", "reserialized declaration", "index", "asset", "unlisted", "traversal"} {
		t.Run(kind, func(t *testing.T) {
			f := newRegisteredFixture(t)
			binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
			if err != nil {
				t.Fatal(err)
			}
			path := "dashboard/console.html"
			switch kind {
			case "declaration":
				f.responses["/internal/dashboard-declaration"] = []byte("{}")
			case "reserialized declaration":
				f.responses["/internal/dashboard-declaration"] = append(f.responses["/internal/dashboard-declaration"], '\n')
			case "index":
				f.responses["/internal/dashboard-index"] = []byte("{}")
			case "asset":
				f.responses["/internal/dashboard/console.html"] = []byte("tampered")
			case "unlisted":
				path = "dashboard/missing.html"
			case "traversal":
				path = "dashboard/../console.html"
			}
			if strings.Contains(kind, "declaration") {
				_, err = f.source.Lookup(context.Background(), f.actor, "install-a")
			} else {
				_, _, err = f.source.Asset(context.Background(), f.actor, binding, path)
			}
			if err == nil {
				t.Fatal("accepted modified or unlisted artifact")
			}
		})
	}
}

func TestRegisteredSourceRequiresInstalledScopeBeforeFetching(t *testing.T) {
	for _, change := range []func(*InstalledRelease){
		func(i *InstalledRelease) { i.OrganizationID = "other" }, func(i *InstalledRelease) { i.StationID = "other" },
		func(i *InstalledRelease) { i.Revision = 0 }, func(i *InstalledRelease) { i.State = "removed" },
		func(i *InstalledRelease) { i.AppVersion = "1.2.4" }, func(i *InstalledRelease) { i.AppID = "other" },
	} {
		f := newRegisteredFixture(t)
		change(&f.installed)
		if _, err := f.source.Lookup(context.Background(), f.actor, "install-a"); !errors.Is(err, ErrDenied) {
			t.Fatal("installation accepted", err)
		}
		if f.calls != 0 {
			t.Fatal("untrusted installation contacted backend")
		}
	}
}

func TestRegistrationAndInstallationChangesPermanentlyRetireSession(t *testing.T) {
	for _, kind := range []string{"remove registration", "delete registry", "change registration", "remove installation", "installation revision", "lose management permission"} {
		t.Run(kind, func(t *testing.T) {
			f := newRegisteredFixture(t)
			r := New(f.source)
			launch := openFixture(t, r, f.actor)
			registry, installed := f.registry, f.installed
			switch kind {
			case "remove registration":
				f.registry.Releases = []RegisteredRelease{}
			case "change registration":
				f.registry.Releases = append([]RegisteredRelease{}, f.registry.Releases...)
				f.registry.Releases[0].Grants = []string{"app.request", "instances.open"}
			case "remove installation":
				f.installed.State = "removed"
			case "installation revision":
				f.installed.Revision++
			case "lose management permission":
				f.installed.CanManage = false
			}
			f.write(t)
			if kind == "delete registry" {
				if err := os.Remove(f.source.path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := r.Check(context.Background(), f.actor, launch.DashboardSessionID); err == nil {
				t.Fatal("changed trust kept session")
			}
			f.registry, f.installed = registry, installed
			f.write(t)
			if _, err := r.Check(context.Background(), f.actor, launch.DashboardSessionID); !errors.Is(err, ErrExpired) {
				t.Fatal("old session revived", err)
			}
			openFixture(t, r, f.actor)
		})
	}
}

func TestRegisteredSourceNarrowsWritePolicy(t *testing.T) {
	f := newRegisteredFixture(t)
	f.installed.CanManage = false
	r := New(f.source)
	launch := openFixture(t, r, f.actor)
	if _, err := r.AuthorizeRequest(context.Background(), f.actor, launch.DashboardSessionID, "GET", "/instances", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AuthorizeRequest(context.Background(), f.actor, launch.DashboardSessionID, "POST", "/instances/one/start", 0); !errors.Is(err, ErrDenied) {
		t.Fatal("write allowed", err)
	}
}

func TestRegistryRejectsUnsafeOriginsPathsAndAmbiguousPolicies(t *testing.T) {
	cases := map[string]func(*RegisteredRelease){
		"public origin":      func(r *RegisteredRelease) { r.Backend.Origin = "https://example.invalid" },
		"metadata address":   func(r *RegisteredRelease) { r.Backend.Origin = "http://169.254.169.254" },
		"userinfo":           func(r *RegisteredRelease) { r.Backend.Origin = "http://user@sample.apps.svc.cluster.local" },
		"empty port":         func(r *RegisteredRelease) { r.Backend.Origin = "http://sample.apps.svc.cluster.local:" },
		"large port":         func(r *RegisteredRelease) { r.Backend.Origin = "http://sample.apps.svc.cluster.local:65536" },
		"leading zero port":  func(r *RegisteredRelease) { r.Backend.Origin = "http://sample.apps.svc.cluster.local:080" },
		"origin path":        func(r *RegisteredRelease) { r.Backend.Origin += "/" },
		"empty fragment":     func(r *RegisteredRelease) { r.Backend.Origin += "#" },
		"credential scope":   func(r *RegisteredRelease) { r.Backend.TokenEnv = "OTHER_TOKEN" },
		"metadata template":  func(r *RegisteredRelease) { r.Backend.IndexPath = "/index/:id" },
		"upstream traversal": func(r *RegisteredRelease) { r.Rules[0].UpstreamPath = "/internal/../instances" },
		"renamed parameter":  func(r *RegisteredRelease) { r.Rules[1].UpstreamPath = "/internal/:other/start" },
		"repeated parameter": func(r *RegisteredRelease) { r.Rules[1].UpstreamPath = "/:instanceId/:instanceId" },
		"invalid parameter":  func(r *RegisteredRelease) { r.Rules[1].Path = "/instances/:id:extra/start" },
		"duplicate rule":     func(r *RegisteredRelease) { r.Rules = append(r.Rules, r.Rules[0]) },
		"overlapping rule": func(r *RegisteredRelease) {
			r.Rules = append(r.Rules, Rule{Method: "POST", Path: "/instances/special/start", UpstreamPath: "/internal/special/start", MaxBody: 512})
		},
		"duplicate grant": func(r *RegisteredRelease) { r.Grants = append(r.Grants, r.Grants[0]) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRegisteredFixture(t)
			change(&f.registry.Releases[0])
			f.write(t)
			if _, err := f.source.Lookup(context.Background(), f.actor, "install-a"); !errors.Is(err, ErrInvalid) {
				t.Fatal("unsafe registration accepted", err)
			}
			if f.calls != 0 {
				t.Fatal("invalid registry contacted backend")
			}
		})
	}
	f := newRegisteredFixture(t)
	for _, data := range [][]byte{
		[]byte(`{"schemaVersion":1,"schemaVersion":1,"releases":[]}`),
		[]byte(`{"schemaVersion":1,"releases":[],"unknown":false}`),
		[]byte(`{"schemaVersion":1,"releases":null}`), bytes.Repeat([]byte(" "), maxRegistryBytes+1),
	} {
		if err := os.WriteFile(f.source.path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.source.registry(); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid wire shape accepted", err)
		}
	}
}

func TestRegisteredMetadataTransportRejectsRedirectsWrongTypesAndOversize(t *testing.T) {
	for _, kind := range []string{"redirect", "forbidden", "offline", "wrong mime", "oversize", "missing credential"} {
		t.Run(kind, func(t *testing.T) {
			f := newRegisteredFixture(t)
			switch kind {
			case "redirect":
				f.status = 302
			case "forbidden":
				f.status = 403
			case "offline":
				f.status = 503
			case "wrong mime":
				f.mime = "text/html"
			case "oversize":
				f.responses["/internal/dashboard-declaration"] = bytes.Repeat([]byte(" "), (64<<10)+1)
			case "missing credential":
				f.source.credential = func(string) string { return "" }
			}
			if _, err := f.source.Lookup(context.Background(), f.actor, "install-a"); err == nil {
				t.Fatal("invalid response accepted")
			}
			if f.calls > 1 {
				t.Fatal("redirect followed or invalid metadata consumed")
			}
		})
	}
}
