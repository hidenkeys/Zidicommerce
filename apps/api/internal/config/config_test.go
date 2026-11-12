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

func TestWhatsAppSignatureBypassDefaultsToFalse(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("APP_ENV", "development")
	t.Setenv("WHATSAPP_SIGNATURE_BYPASS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Channels.WhatsAppSignatureBypass {
		t.Fatal("WhatsApp signature bypass must default to false")
	}
	if cfg.Channels.WhatsAppGraphBaseURL != "https://graph.facebook.com" {
		t.Fatalf("unexpected default Graph API URL %q", cfg.Channels.WhatsAppGraphBaseURL)
	}
	if cfg.Channels.MetaAdvancedAccess {
		t.Fatal("Meta Advanced Access must default to false until App Review is confirmed")
	}
	if cfg.Channels.InstagramOAuthBaseURL != "https://www.instagram.com" || cfg.Channels.InstagramAPIOAuthBaseURL != "https://api.instagram.com" || cfg.Channels.InstagramGraphBaseURL != "https://graph.instagram.com" {
		t.Fatalf("unexpected Instagram provider endpoints: %+v", cfg.Channels)
	}
	if cfg.Channels.TikTokVerifiedAccess || cfg.Channels.TikTokAuthorizeURL != "https://www.tiktok.com/v2/auth/authorize/" || cfg.Channels.TikTokAPIBaseURL != "https://open.tiktokapis.com" {
		t.Fatalf("unexpected TikTok defaults: %+v", cfg.Channels)
	}
}

func TestWhatsAppSignatureBypassIsRejectedInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("APP_ENV", "production")
	t.Setenv("WHATSAPP_SIGNATURE_BYPASS", "true")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "WHATSAPP_SIGNATURE_BYPASS") {
		t.Fatalf("expected production signature bypass rejection, got %v", err)
	}
}

func TestWhatsAppWebhookPublicURLValidation(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("APP_ENV", "development")
	t.Setenv("WHATSAPP_WEBHOOK_PUBLIC_BASE_URL", "https://api.example.com/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Channels.WhatsAppWebhookPublicBaseURL != "https://api.example.com" {
		t.Fatalf("unexpected webhook base URL %q", cfg.Channels.WhatsAppWebhookPublicBaseURL)
	}

	t.Setenv("APP_ENV", "production")
	t.Setenv("WHATSAPP_WEBHOOK_PUBLIC_BASE_URL", "http://api.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("expected insecure production webhook URL rejection, got %v", err)
	}
}

func TestMetaEmbeddedSignupConfigurationLoadsWithoutExposingItToFrontendConfig(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("APP_ENV", "development")
	t.Setenv("META_APP_ID", "123456789")
	t.Setenv("META_APP_SECRET", "server-only-secret")
	t.Setenv("META_EMBEDDED_SIGNUP_CONFIGURATION_ID", "987654321")
	t.Setenv("META_GRAPH_API_VERSION", "v24.0")
	t.Setenv("META_WEBHOOK_VERIFY_TOKEN", "server-only-verify-token")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Channels.MetaAppID != "123456789" || cfg.Channels.MetaEmbeddedConfigurationID != "987654321" || cfg.Channels.MetaGraphAPIVersion != "v24.0" {
		t.Fatalf("unexpected Embedded Signup configuration: %+v", cfg.Channels)
	}
	if cfg.Channels.MetaAppSecret == "" || cfg.Channels.MetaWebhookVerifyToken == "" {
		t.Fatal("expected server-side Meta credentials to load")
	}
}

func TestControlledBetaRequiresSafeConfiguration(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("CONTROLLED_BETA", "true")
	t.Setenv("PAYMENT_PROVIDER", "paystack")
	t.Setenv("PAYSTACK_SECRET_KEY", "sk_test_fixture")
	if cfg, err := Load(); err != nil || !cfg.ControlledBeta {
		t.Fatalf("expected valid beta configuration: %v", err)
	}
	for key, value := range map[string]string{
		"PAYSTACK_SECRET_KEY":       "sk_live_fixture",
		"WHATSAPP_FLOWS_ENABLED":    "true",
		"WHATSAPP_SIGNATURE_BYPASS": "true",
		"FIELD_SERVICE_DEMO_SEED":   "true",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := Load(); err == nil {
				t.Fatal("expected unsafe beta configuration to be rejected")
			}
		})
	}
}

func TestControlledBetaFlowsRequireStagingAndDesignatedTester(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-value-for-config")
	t.Setenv("CONTROLLED_BETA", "true")
	t.Setenv("PAYMENT_PROVIDER", "paystack")
	t.Setenv("PAYSTACK_SECRET_KEY", "sk_test_fixture")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("WHATSAPP_FLOWS_ENABLED", "true")
	t.Setenv("WHATSAPP_FLOWS_TEST_WABA_ID", "test-waba")
	t.Setenv("WHATSAPP_FLOWS_TEST_PHONE_ID", "test-phone")
	t.Setenv("WHATSAPP_TESTER_NUMBERS", "2348156572209")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Fatal("production Flow beta accepted")
	}
	t.Setenv("APP_ENV", "staging")
	t.Setenv("WHATSAPP_TESTER_NUMBERS", "")
	if _, err := Load(); err == nil {
		t.Fatal("unrestricted Flow beta accepted")
	}
}
