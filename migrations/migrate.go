// Package migrations owns only the Studio business schema.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, db *pgxpool.Pool) error { return run(ctx, db, true) }
func Check(ctx context.Context, db *pgxpool.Pool) error { return run(ctx, db, false) }
func run(ctx context.Context, db *pgxpool.Pool, apply bool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(2026100502)"); err != nil {
		return err
	}
	if apply {
		if _, err = tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS studio; CREATE TABLE IF NOT EXISTS studio.schema_migrations(version integer PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
			return err
		}
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT version,name,checksum FROM studio.schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	n := 0
	for rows.Next() {
		var version int
		var name, hash string
		if err = rows.Scan(&version, &name, &hash); err != nil {
			rows.Close()
			return err
		}
		if n >= len(names) || version != n+1 || name != names[n] {
			rows.Close()
			return errors.New("Studio migration history mismatch")
		}
		data, _ := files.ReadFile(name)
		sum := sha256.Sum256(data)
		if hash != hex.EncodeToString(sum[:]) {
			rows.Close()
			return errors.New("Studio migration checksum mismatch")
		}
		n++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !apply && n != len(names) {
		return errors.New("pending Studio migrations")
	}
	for i := n; i < len(names); i++ {
		data, err := files.ReadFile(names[i])
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if _, err = tx.Exec(ctx, "INSERT INTO studio.schema_migrations(version,name,checksum) VALUES($1,$2,$3)", i+1, names[i], hex.EncodeToString(sum[:])); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
