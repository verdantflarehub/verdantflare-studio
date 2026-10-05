package migrations_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/testdb"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

func TestUpgradeProjectDatabaseToWorld(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	base, e := os.ReadFile("0001_project_revisions.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, `CREATE SCHEMA studio; CREATE TABLE studio.schema_migrations(version integer PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(base)); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, "INSERT INTO studio.schema_migrations(version,name,checksum) VALUES(1,$1,$2)", "0001_project_revisions.sql", fmt.Sprintf("%x", sha256.Sum256(base))); e != nil {
		t.Fatal(e)
	}
	project, org, owner := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	if _, e = db.Exec(ctx, "INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category) VALUES($1,$2,$3,'existing project','music')", project, org, owner); e != nil {
		t.Fatal(e)
	}
	if migrations.Check(ctx, db) == nil {
		t.Fatal("old schema reported current")
	}
	if e = migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e = migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e = migrations.Check(ctx, db); e != nil {
		t.Fatal(e)
	}
	var name string
	if e = db.QueryRow(ctx, "SELECT name FROM studio.projects WHERE project_id=$1", project).Scan(&name); e != nil || name != "existing project" {
		t.Fatal("upgrade lost existing project", e)
	}
	var count int
	if e = db.QueryRow(ctx, "SELECT count(*) FROM studio.world_assets").Scan(&count); e != nil || count != 0 {
		t.Fatal("World tables unavailable", e)
	}
}

func TestStudioOwnershipAndMigrationHistory(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if migrations.Check(ctx, db) == nil {
		t.Fatal("uninitialized database reported ready")
	}
	if e := migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e := migrations.Check(ctx, db); e != nil {
		t.Fatal(e)
	}
	var station bool
	if e := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='station')").Scan(&station); e != nil || station {
		t.Fatal("Studio migration touched Station schema", e)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	org, project, owner := id(), id(), id()
	if _, e := db.Exec(ctx, "INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category) VALUES($1,$2,$3,'fixture','image')", project, org, owner); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(ctx, "INSERT INTO studio.project_members(organization_id,project_id,subject_id,role) VALUES($1,$2,$3,'reader')", id(), project, owner); e == nil {
		t.Fatal("cross-organization member foreign key accepted")
	}
	if _, e := db.Exec(ctx, "INSERT INTO studio.studio_events(event_id,organization_id,subject_id,request_id,action,resource_id) VALUES($1,$2,$3,$4,'fixture',$5)", id(), org, owner, id(), project); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(ctx, "DELETE FROM studio.studio_events"); e == nil {
		t.Fatal("audit deletion allowed")
	}
	if _, e := db.Exec(ctx, "UPDATE studio.schema_migrations SET checksum=repeat('0',64)"); e != nil {
		t.Fatal(e)
	}
	if migrations.Check(ctx, db) == nil || migrations.Apply(ctx, db) == nil {
		t.Fatal("tampered migration accepted")
	}
}
