package control

import (
	"context"
	_ "embed"
)

//go:embed schema.sql
var schema string

//go:embed machines.sql
var machineSchema string

// The first migration also adopts databases created before migration tracking.
func (a *API) Migrate(ctx context.Context) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(438927104)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY)"); err != nil {
		return err
	}
	for i, sql := range []string{schema, machineSchema} {
		var applied bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", i+1).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err = tx.Exec(ctx, sql); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations VALUES ($1)", i+1); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
