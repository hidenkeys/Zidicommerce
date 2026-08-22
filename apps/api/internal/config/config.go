package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppName       string
	AppEnv        string
	ServerPort    string
	LogLevel      string
	MigrationsDir string
	CORSOrigins   string
	Database      DatabaseConfig
	JWT           JWTConfig
	Payment       PaymentConfig
	Email         EmailConfig
	FieldService  FieldServiceConfig
}

// FieldServiceConfig controls the optional field-service pilot tenant. Seeding
// is opt-in and never runs unless SeedDemo is explicitly enabled.
type FieldServiceConfig struct {
	SeedDemo          bool
	DemoPassword      string
	ProviderPortalURL string
	PilotSeedProfile  string
	PilotPassword     string
}

type DatabaseConfig struct {
	URL      string
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
	Timezone string
}

type JWTConfig struct {
	Secret     string
	Issuer     string
	TTLMinutes int
}

type PaymentConfig struct {
	Provider            string
	PaystackSecret      string
	SecretEncryptionKey string
}

type EmailConfig struct {
	Mode       string
	SMTPHost   string
	SMTPPort   string
	SMTPUser   string
	SMTPPass   string
	From       string
	FromName   string
	TLSMode    string
	AppBaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		AppName:       getenv("APP_NAME", "zidicommerce"),
		AppEnv:        getenv("APP_ENV", "development"),
		ServerPort:    getenv("SERVER_PORT", "8080"),
		LogLevel:      getenv("LOG_LEVEL", "info"),
		MigrationsDir: getenv("MIGRATIONS_DIR", "migrations"),
		CORSOrigins:   getenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
		Database: DatabaseConfig{
			URL:      os.Getenv("DATABASE_URL"),
			Host:     getenv("DATABASE_HOST", "localhost"),
			Port:     getenv("DATABASE_PORT", "5432"),
			Name:     getenv("DATABASE_NAME", "zidicommerce"),
			User:     getenv("DATABASE_USER", "postgres"),
			Password: os.Getenv("DATABASE_PASSWORD"),
			SSLMode:  getenv("DATABASE_SSLMODE", "disable"),
			Timezone: getenv("DATABASE_TIMEZONE", "Africa/Lagos"),
		},
		JWT: JWTConfig{
			Secret: getenv("JWT_SECRET", ""),
			Issuer: getenv("JWT_ISSUER", "zidicommerce"),
		},
		Payment: PaymentConfig{
			Provider:            getenv("PAYMENT_PROVIDER", "test"),
			PaystackSecret:      os.Getenv("PAYSTACK_SECRET_KEY"),
			SecretEncryptionKey: os.Getenv("PAYMENT_SECRET_ENCRYPTION_KEY"),
		},
		FieldService: FieldServiceConfig{
			SeedDemo:          strings.EqualFold(strings.TrimSpace(os.Getenv("FIELD_SERVICE_DEMO_SEED")), "true"),
			DemoPassword:      strings.TrimSpace(os.Getenv("FIELD_SERVICE_DEMO_PASSWORD")),
			ProviderPortalURL: strings.TrimRight(strings.TrimSpace(os.Getenv("FIELD_SERVICE_PORTAL_URL")), "/"),
			PilotSeedProfile:  strings.TrimSpace(os.Getenv("FIELD_SERVICE_PILOT_SEED_PROFILE")),
			PilotPassword:     strings.TrimSpace(os.Getenv("FIELD_SERVICE_PILOT_PASSWORD")),
		},
		Email: EmailConfig{
			Mode:       getenv("EMAIL_MODE", "log"),
			SMTPHost:   os.Getenv("SMTP_HOST"),
			SMTPPort:   getenv("SMTP_PORT", "587"),
			SMTPUser:   firstEnv("SMTP_USER", "SMTP_USERNAME"),
			SMTPPass:   os.Getenv("SMTP_PASSWORD"),
			From:       getenv("EMAIL_FROM", getenv("SMTP_FROM_EMAIL", "noreply@zidicommerce.local")),
			FromName:   getenv("EMAIL_FROM_NAME", getenv("SMTP_FROM_NAME", "ZidiCommerce")),
			TLSMode:    getenv("SMTP_TLS", "starttls"),
			AppBaseURL: getenv("APP_BASE_URL", "http://localhost:3000"),
		},
	}

	ttl, err := strconv.Atoi(getenv("JWT_TTL_MINUTES", "1440"))
	if err != nil || ttl <= 0 {
		return Config{}, errors.New("JWT_TTL_MINUTES must be a positive integer")
	}
	cfg.JWT.TTLMinutes = ttl

	if cfg.JWT.Secret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.ServerPort == "" {
		return Config{}, errors.New("SERVER_PORT is required")
	}
	if err := cfg.Email.validate(cfg.AppEnv); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c EmailConfig) validate(appEnv string) error {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if strings.EqualFold(appEnv, "production") && mode != "smtp" {
		return errors.New("EMAIL_MODE must be smtp in production")
	}
	if mode != "smtp" {
		return nil
	}
	if strings.TrimSpace(c.SMTPHost) == "" || strings.TrimSpace(c.SMTPUser) == "" || strings.TrimSpace(c.SMTPPass) == "" || strings.TrimSpace(c.From) == "" {
		return errors.New("SMTP_HOST, SMTP_USER, SMTP_PASSWORD, and EMAIL_FROM are required when EMAIL_MODE=smtp")
	}
	if strings.Contains(strings.ToLower(c.AppBaseURL), "localhost") && strings.EqualFold(appEnv, "production") {
		return errors.New("APP_BASE_URL must be the production admin URL")
	}
	return nil
}

func (c DatabaseConfig) DSN() string {
	if strings.TrimSpace(c.URL) != "" {
		return c.URL
	}
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		c.Host,
		c.User,
		c.Password,
		c.Name,
		c.Port,
		c.SSLMode,
		c.Timezone,
	)
}

func (c JWTConfig) TTL() time.Duration {
	return time.Duration(c.TTLMinutes) * time.Minute
}

func NewLogger(level string) *slog.Logger {
	var parsed slog.Level
	switch strings.ToLower(level) {
	case "debug":
		parsed = slog.LevelDebug
	case "warn":
		parsed = slog.LevelWarn
	case "error":
		parsed = slog.LevelError
	default:
		parsed = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parsed}))
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
