package migrations_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/testdb"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

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
