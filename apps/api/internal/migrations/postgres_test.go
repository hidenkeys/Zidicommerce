package migrations

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRepositoryMigrationsApplyToEmptyPostgres(t *testing.T) {
	dsn := os.Getenv("ZIDI_MIGRATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ZIDI_MIGRATION_TEST_DATABASE_URL to a dedicated empty PostgreSQL database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var existing string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(to_regclass('public.schema_migrations')::text, '')`).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		t.Fatal("migration test database must be empty and dedicated to this test")
	}
	dir := filepath.Clean(filepath.Join("..", "..", "..", "..", "migrations"))
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("repository migrations were not found: files=%d err=%v", len(files), err)
	}
	if err := Run(ctx, db, dir); err != nil {
		t.Fatal(err)
	}
	var applied int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != len(files) {
		t.Fatalf("applied %d migrations, expected %d", applied, len(files))
	}
	for _, table := range []string{"channel_capabilities", "channel_oauth_sessions", "channel_oauth_assets", "channel_meta_connection_states", "channel_meta_contact_states"} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(to_regclass('public.' || $1)::text, '')`, table).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == "" {
			t.Fatalf("Phase R0 table %s was not created", table)
		}
	}
}
