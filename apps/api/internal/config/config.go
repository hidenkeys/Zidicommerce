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
	Database      DatabaseConfig
	JWT           JWTConfig
	Payment       PaymentConfig
	Email         EmailConfig
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
	AppBaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		AppName:       getenv("APP_NAME", "zidicommerce"),
		AppEnv:        getenv("APP_ENV", "development"),
		ServerPort:    getenv("SERVER_PORT", "8080"),
		LogLevel:      getenv("LOG_LEVEL", "info"),
		MigrationsDir: getenv("MIGRATIONS_DIR", "migrations"),
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
		Email: EmailConfig{
			Mode:       getenv("EMAIL_MODE", "log"),
			SMTPHost:   os.Getenv("SMTP_HOST"),
			SMTPPort:   getenv("SMTP_PORT", "587"),
			SMTPUser:   os.Getenv("SMTP_USER"),
			SMTPPass:   os.Getenv("SMTP_PASSWORD"),
			From:       getenv("EMAIL_FROM", "noreply@zidicommerce.local"),
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

	return cfg, nil
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
