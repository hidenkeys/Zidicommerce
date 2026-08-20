package config

import (
	"strings"
	"testing"
)

func TestDatabaseConfigDSNUsesDatabaseURLWhenProvided(t *testing.T) {
	cfg := DatabaseConfig{
		URL:  "postgres://user:pass@example.com:5432/db?sslmode=require",
		Host: "ignored",
	}

	if got := cfg.DSN(); got != cfg.URL {
		t.Fatalf("expected DATABASE_URL to win, got %q", got)
	}
}

func TestDatabaseConfigDSNBuildsFromParts(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		Name:     "zidicommerce",
		User:     "postgres",
		Password: "secret",
		SSLMode:  "disable",
		Timezone: "Africa/Lagos",
	}

	got := cfg.DSN()
	for _, part := range []string{"host=localhost", "user=postgres", "password=secret", "dbname=zidicommerce", "port=5432", "sslmode=disable"} {
		if !strings.Contains(got, part) {
			t.Fatalf("expected DSN to contain %q, got %q", part, got)
		}
	}
}
