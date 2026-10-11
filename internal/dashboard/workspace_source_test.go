package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWorkspaceSourceVerifiesEntryAndAssetsAndBindsRequests(t *testing.T) {
	f := newRegisteredFixture(t)
	entry := workspaceEntryFixture()
	asset := []byte("<!doctype html><title>App owned editor</title>")
	index := marshalFixture(t, assetIndex{1, map[string]string{entry.EntryRef: strings.TrimPrefix(sum(asset), "sha256:")}})
	entry.ArtifactDigest = sum(index)
	f.registry.Releases[0].InstanceEntries = []InstanceEntry{entry}
	f.write(t)
	f.responses[entry.IndexPath] = index
	f.responses[entry.AssetPath+entry.EntryRef] = asset
	original := f.source.client.Transport
	resolveBody := `{"instanceId":"instance-one","entryId":"editor","state":"running"}`
	var sentPath string
	f.source.client.Transport = sourceTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return original.RoundTrip(r)
		}
		if r.Header.Get("X-User-Id") != f.actor.UserID || r.Header.Get("X-Organization-Id") != f.actor.OrganizationID {
			t.Fatal("actor not forwarded")
		}
		body, _ := io.ReadAll(r.Body)
		response := resolveBody
		if r.URL.Path == entry.ResolvePath {
			if !bytes.Equal(body, []byte(`{"instanceId":"instance-one","entryId":"editor"}`)) {
				t.Fatal("resolution body changed")
			}
		} else {
			sentPath = r.URL.Path
			if string(body) != `{"version":2}` {
				t.Fatal("business body changed")
			}
			response = `{"version":3}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})
	binding, err := f.source.Lookup(context.Background(), f.actor, f.installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	in := InstanceOpen{"instance-one", "editor"}
	if err = f.source.ResolveInstance(context.Background(), f.actor, binding, entry, in); err != nil {
		t.Fatal(err)
	}
	data, mime, err := f.source.WorkspaceAsset(context.Background(), f.actor, binding, entry, entry.EntryRef)
	if err != nil || !bytes.Equal(data, asset) || mime != "text/html" {
		t.Fatal("workspace artifact not verified", err)
	}
	data, status, err := f.source.WorkspaceRequest(context.Background(), f.actor, binding, entry, in.InstanceID,
		AppRequest{Method: "POST", Path: "/draft", Body: json.RawMessage(`{"version":2}`)}, "request-one")
	if err != nil || status != 200 || string(data) != `{"version":3}` || sentPath != "/internal/workspace/instances/instance-one/draft" {
		t.Fatal("instance not bound", err)
	}
	if _, _, err = f.source.WorkspaceRequest(context.Background(), f.actor, binding, entry, in.InstanceID,
		AppRequest{Method: "POST", Path: "/instance-two/draft", Body: json.RawMessage(`{}`)}, "request-two"); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{"instanceId":"other","entryId":"editor","state":"running"}`, `{"instanceId":"instance-one","entryId":"editor","state":"running","url":"https://other.invalid"}`} {
		resolveBody = invalid
		if err = f.source.ResolveInstance(context.Background(), f.actor, binding, entry, in); !errors.Is(err, ErrInvalid) {
			t.Fatal("substitution accepted", err)
		}
	}
	f.responses[entry.AssetPath+entry.EntryRef] = []byte("tampered")
	if _, _, err = f.source.WorkspaceAsset(context.Background(), f.actor, binding, entry, entry.EntryRef); !errors.Is(err, ErrInvalid) {
		t.Fatal("tampered entry accepted", err)
	}
	if _, _, err = f.source.WorkspaceAsset(context.Background(), f.actor, binding, entry, "not-indexed.html"); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}

func TestWorkspacePublicAssetsRequireExplicitRegistrationAndNeverReadUserState(t *testing.T) {
	f := newRegisteredFixture(t)
	entry := workspaceEntryFixture()
	asset := []byte("<!doctype html>public release code")
	index := marshalFixture(t, assetIndex{1, map[string]string{entry.EntryRef: strings.TrimPrefix(sum(asset), "sha256:")}})
	entry.ArtifactDigest = sum(index)
	f.registry.Releases[0].InstanceEntries = []InstanceEntry{entry}
	f.write(t)
	f.source.read = func(context.Context, Actor, string) (InstalledRelease, error) {
		t.Fatal("public code read installation or user state")
		return InstalledRelease{}, ErrDenied
	}
	f.source.client.Transport = sourceTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.Header.Get("X-User-Id") != "" || r.Header.Get("X-Organization-Id") != "" || r.Header.Get("Authorization") == "" {
			t.Fatal("public asset context not isolated")
		}
		data, mime := index, "application/json"
		if r.URL.Path == entry.AssetPath+entry.EntryRef {
			data, mime = asset, "text/html"
		} else if r.URL.Path != entry.IndexPath {
			t.Fatal("unregistered resource path")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {mime}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	hex := strings.TrimPrefix(entry.ArtifactDigest, "sha256:")
	if data, _, err := f.source.PublicWorkspaceAsset(context.Background(), hex, entry.EntryRef); err != nil || !bytes.Equal(data, asset) {
		t.Fatal(err)
	}
	f.registry.Releases[0].InstanceEntries = nil
	f.write(t)
	if _, _, err := f.source.PublicWorkspaceAsset(context.Background(), hex, entry.EntryRef); !errors.Is(err, ErrDenied) {
		t.Fatal("withdrawn release remained public", err)
	}
}
