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

func TestEmailConfigLoadsSMTPSettings(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("EMAIL_MODE", "smtp")
	t.Setenv("SMTP_HOST", "smtp.zoho.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "admin@example.com")
	t.Setenv("SMTP_PASSWORD", "app-password")
	t.Setenv("EMAIL_FROM", "admin@example.com")
	t.Setenv("EMAIL_FROM_NAME", "ZidiCommerce")
	t.Setenv("SMTP_TLS", "starttls")
	t.Setenv("APP_BASE_URL", "https://admin.example.com")
	t.Setenv("APP_ENV", "production")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email.Mode != "smtp" || cfg.Email.SMTPHost != "smtp.zoho.com" || cfg.Email.SMTPUser != "admin@example.com" || cfg.Email.FromName != "ZidiCommerce" || cfg.Email.TLSMode != "starttls" {
		t.Fatalf("unexpected email config: %+v", cfg.Email)
	}
	if cfg.Email.SMTPPass == "" {
		t.Fatal("expected SMTP password to load from the environment")
	}
}

func TestEmailConfigRejectsProductionLogMode(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("APP_ENV", "production")
	t.Setenv("EMAIL_MODE", "log")
	if _, err := Load(); err == nil {
		t.Fatal("expected production log email mode to be rejected")
	}
}

func TestEmailConfigRejectsIncompleteSMTP(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("EMAIL_MODE", "smtp")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected incomplete SMTP configuration to be rejected")
	}
}
