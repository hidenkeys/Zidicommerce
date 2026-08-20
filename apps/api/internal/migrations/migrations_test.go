package migrations

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunAppliesUnappliedMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000001_test.sql"), []byte("CREATE TABLE test_table (id TEXT PRIMARY KEY);"), 0o644); err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version FROM schema_migrations").WithArgs("000001_test.sql").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE test_table").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs("000001_test.sql").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := Run(context.Background(), db, dir); err != nil {
		t.Fatalf("expected migration to run, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
