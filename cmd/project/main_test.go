package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/testdb"
)

func TestProjectServerMigrationAndLifecycle(t *testing.T) {
	db := testdb.New(t)
	t.Setenv("STUDIO_DATABASE_URL", testdb.ConnString(db))
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("p", 32))
	t.Setenv("STUDIO_ARTIFACT_AUTHORITY_TOKEN", strings.Repeat("a", 32))
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", strings.Repeat("s", 32))
	artifact := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer artifact.Close()
	t.Setenv("STUDIO_ARTIFACT_URL", artifact.URL)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := l.Addr().String()
	l.Close()
	t.Setenv("STUDIO_PROJECT_LISTEN", addr)
	ctx := context.Background()
	if e := run(ctx, []string{"serve"}); e == nil {
		t.Fatal("serve implicitly migrated database")
	}
	if e := run(ctx, []string{"migrate"}); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := db.QueryRow(ctx, "SELECT count(*) FROM studio.schema_migrations").Scan(&count); e != nil || count != 1 {
		t.Fatal("migration missed isolated database")
	}
	if e := run(ctx, []string{"migrate"}); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, []string{"serve"}) }()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case e := <-done:
			t.Fatalf("server exited: %v", e)
		case <-deadline.C:
			t.Fatal("server did not listen")
		case <-tick.C:
			r, _ := http.NewRequest("POST", "http://"+addr+"/internal/v1/project/list", strings.NewReader("{}"))
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("p", 32))
			r.Header.Set("Content-Type", "application/json")
			for _, key := range []string{"X-User-Id", "X-Organization-Id", "X-Request-Id"} {
				r.Header.Set(key, uuid.Must(uuid.NewV7()).String())
			}
			response, e := client.Do(r)
			if e != nil {
				continue
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("list returned %d", response.StatusCode)
			}
			cancel()
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not finish")
			}
			return
		}
	}
}
