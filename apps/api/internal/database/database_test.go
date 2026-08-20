package database

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSQLDatabasePingSucceeds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectPing()
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("expected ping to succeed, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
