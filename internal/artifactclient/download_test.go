package artifactclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestStreamedDownloadEnforcesAuthorizationAndIntegrity(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	ref := project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}
	content := []byte("fixed content")
	sum := sha256.Sum256(content)
	v := project.ContentVersion{SchemaVersion: 2, ContentRef: ref, OrganizationID: p.OrganizationID, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)), MIME: "application/octet-stream", Source: project.Source{Kind: "user_import", ProjectID: id()}, VersionNo: 1, CreatedBy: p.SubjectID, CreatedAt: time.Now().UTC()}
	for _, mode := range []string{"ok", "short", "long", "digest", "limit", "denied", "writer"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
					t.Error("missing authenticated context")
				}
				if !strings.HasSuffix(r.URL.Path, "/content") {
					_ = json.NewEncoder(w).Encode(v)
					return
				}
				requests++
				if mode == "denied" {
					w.WriteHeader(403)
					_ = json.NewEncoder(w).Encode(map[string]string{"code": "PERMISSION_DENIED", "message": "denied", "request_id": p.RequestID})
					return
				}
				body := content
				switch mode {
				case "short":
					body = body[:len(body)-1]
				case "long":
					body = append(append([]byte{}, body...), 1)
				case "digest":
					body = bytes.Repeat([]byte("x"), len(body))
				}
				_, _ = w.Write(body)
			}))
			defer srv.Close()
			c, e := New(srv.URL, strings.Repeat("s", 32))
			if e != nil {
				t.Fatal(e)
			}
			var out bytes.Buffer
			var dst io.Writer = &out
			if mode == "writer" {
				dst = failingWriter{}
			}
			limit := int64(1024)
			if mode == "limit" {
				limit = 1
			}
			e = c.Download(context.Background(), p, ref, project.Access{}, dst, limit)
			switch mode {
			case "ok":
				if e != nil || !bytes.Equal(out.Bytes(), content) {
					t.Fatal(e)
				}
			case "limit":
				if !errors.Is(e, project.ErrInvalid) || requests != 0 {
					t.Fatal("oversized download started", e)
				}
			case "denied":
				if !errors.Is(e, project.ErrForbidden) || out.Len() != 0 {
					t.Fatal("revoked download proceeded", e)
				}
			case "writer":
				if !errors.Is(e, io.ErrClosedPipe) {
					t.Fatal(e)
				}
			default:
				if !errors.Is(e, project.ErrNotReady) {
					t.Fatal("corrupt bytes accepted", e)
				}
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
