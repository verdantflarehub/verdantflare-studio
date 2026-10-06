package workspacehttp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

func newID() string { return uuid.Must(uuid.NewV7()).String() }

type callerReader struct {
	*bytes.Reader
	closed bool
}

func (r *callerReader) Close() error { r.closed = true; return nil }

type requestEnvelope struct {
	ID     string `json:"id"`
	Params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"params"`
}

func reply(w http.ResponseWriter, id string, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"isError": status >= 400, "structuredContent": value}})
}

func TestEndpointAndRedirectDoNotExposeSession(t *testing.T) {
	for _, endpoint := range []string{"http://example.test", "https://user:secret@example.test", "https://example.test?token=x", "https://example.test/other", "file:///tmp/data", "https://example.test#fragment"} {
		if _, err := New(endpoint, "session"); err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	hits := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(200) }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	remote, _ := New(server.URL+"/mcp", "private-session")
	err := remote.Call(t.Context(), "project.list", map[string]any{}, nil)
	if !errors.Is(err, project.ErrDependency) || hits != 0 {
		t.Fatalf("redirect followed: %v, %d", err, hits)
	}
}

func TestCallPreservesJSONAndBusinessConflict(t *testing.T) {
	current := newID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer core-session" || r.Header.Get("X-User-Id") != "" {
			t.Error("unexpected identity or route")
		}
		var in requestEnvelope
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			t.Error("invalid envelope")
		}
		if !bytes.Contains(in.Params.Arguments, []byte("9007199254740993")) {
			t.Error("opaque integer changed")
		}
		reply(w, in.ID, 409, map[string]string{"code": "REVISION_CONFLICT", "current_revision_id": current})
	}))
	defer server.Close()
	r, _ := New(server.URL, "core-session")
	err := r.Call(t.Context(), "project.commit", json.RawMessage(`{"extensions":{"large":9007199254740993,"optional":null}}`), nil)
	var conflict *project.ConflictError
	if !errors.As(err, &conflict) || conflict.CurrentRevisionID != current {
		t.Fatalf("lost conflict details: %v", err)
	}
	if err = r.Call(t.Context(), "video.create", map[string]any{}, nil); !errors.Is(err, project.ErrInvalid) {
		t.Fatal("generation exposed through management client")
	}
}

func TestUploadResumesAfterCommitResponseLoss(t *testing.T) {
	body := bytes.Repeat([]byte("binary\x00"), 1000)
	sum := sha256.Sum256(body)
	req := workspace.UploadRequest{ProjectID: newID(), WriteID: newID(), MIME: "application/octet-stream", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
	req.Source = project.Source{Kind: "user_import", ProjectID: req.ProjectID}
	uploadID, artifactID, versionID := newID(), newID(), newID()
	version := project.ContentVersion{SchemaVersion: 2, ContentRef: project.ContentRef{StoreID: newID(), ArtifactID: artifactID, VersionID: versionID}, Size: req.Size, SHA256: req.SHA256, MIME: req.MIME, Source: req.Source}
	committed := false
	puts, prepares, commits := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-session" {
			t.Error("missing user session")
		}
		if r.Method == "PUT" {
			puts++
			got, _ := io.ReadAll(r.Body)
			if r.URL.Path != "/v2/artifacts/uploads/"+uploadID+"/content" || r.ContentLength != req.Size || !bytes.Equal(got, body) {
				t.Error("wrong upload bytes, path or length")
			}
			w.WriteHeader(200)
			return
		}
		var in requestEnvelope
		_ = json.NewDecoder(r.Body).Decode(&in)
		var args map[string]json.RawMessage
		_ = json.Unmarshal(in.Params.Arguments, &args)
		var mode string
		_ = json.Unmarshal(args["mode"], &mode)
		switch mode {
		case "prepare":
			prepares++
			var writeID string
			_ = json.Unmarshal(args["write_id"], &writeID)
			if writeID != req.WriteID {
				t.Error("write ID changed")
			}
			state := "prepared"
			if committed {
				state = "committed"
			}
			reply(w, in.ID, 200, map[string]any{"upload": map[string]string{"upload_id": uploadID, "artifact_id": artifactID, "version_id": versionID, "state": state, "content_path": "/v2/artifacts/uploads/" + uploadID + "/content"}})
		case "commit":
			commits++
			if !committed {
				committed = true
				w.WriteHeader(503)
				_, _ = io.WriteString(w, "private-upstream-details")
				return
			}
			reply(w, in.ID, 200, map[string]any{"version": version})
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	r, _ := New(server.URL, "user-session")
	owned := &callerReader{Reader: bytes.NewReader(body)}
	if _, err := r.Upload(t.Context(), req, owned); !errors.Is(err, project.ErrDependency) {
		t.Fatalf("first attempt: %v", err)
	}
	if owned.closed {
		t.Fatal("HTTP transport closed the caller-owned source")
	}
	got, err := r.Upload(t.Context(), req, bytes.NewReader(body))
	if err != nil || got.ContentRef != version.ContentRef || puts != 1 || prepares != 2 || commits != 2 {
		t.Fatalf("resume: %v puts=%d prepares=%d commits=%d", err, puts, prepares, commits)
	}
}

func TestRejectUploadPathAndDownloadOverflow(t *testing.T) {
	id := newID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = io.WriteString(w, "longer-than-allowed")
			return
		}
		if r.Method == "PUT" {
			t.Error("followed invalid upload path")
			return
		}
		var in requestEnvelope
		_ = json.NewDecoder(r.Body).Decode(&in)
		reply(w, in.ID, 200, map[string]any{"upload": map[string]string{"upload_id": id, "artifact_id": id, "version_id": id, "state": "prepared", "content_path": "//attacker.invalid/upload"}})
	}))
	defer server.Close()
	r, _ := New(server.URL, "user-session")
	_, err := r.Upload(t.Context(), workspace.UploadRequest{ProjectID: id, WriteID: id, Source: project.Source{Kind: "user_import", ProjectID: id}}, strings.NewReader(""))
	if !errors.Is(err, project.ErrDependency) {
		t.Fatalf("path accepted: %v", err)
	}
	err = r.Download(t.Context(), project.ContentRef{StoreID: id, ArtifactID: id, VersionID: id}, project.Access{}, io.Discard, 3)
	if !errors.Is(err, workspace.ErrCorrupt) {
		t.Fatalf("overflow accepted: %v", err)
	}
}
