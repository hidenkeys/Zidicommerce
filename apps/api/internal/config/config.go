package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
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
	AI            AIConfig
	Channels      ChannelConfig
}

// FieldServiceConfig controls the optional field-service pilot tenant. Seeding
// is opt-in and never runs unless SeedDemo is explicitly enabled.
type FieldServiceConfig struct {
	SeedDemo          bool
	DemoPassword      string
	ProviderPortalURL string
	PilotSeedProfile  string
	PilotPassword     string
	// Adopt* move an already-configured WhatsApp number to another workspace at
	// startup, for deployments whose database has no public endpoint.
	AdoptPhoneNumberID string
	AdoptTargetOrgSlug string
	AdoptVerifyToken   string
	AdoptBotID         string
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

type AIConfig struct {
	Provider              string
	Model                 string
	OllamaBaseURL         string
	OllamaChatModel       string
	GroqAPIKey            string
	GroqBaseURL           string
	GroqModel             string
	MaxToolCalls          int
	EmbeddingsEnabled     bool
	VectorSearchEnabled   bool
	EmbeddingProvider     string
	EmbeddingModel        string
	EmbeddingDimensions   int
	VectorSearchThreshold float64
}

type ChannelConfig struct {
	SecretEncryptionKey          string
	WhatsAppGraphBaseURL         string
	WhatsAppWebhookPublicBaseURL string
	WhatsAppSignatureBypass      bool
	MetaAppID                    string
	MetaAppSecret                string
	MetaEmbeddedConfigurationID  string
	MetaGraphAPIVersion          string
	MetaWebhookVerifyToken       string
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
			SeedDemo:           strings.EqualFold(strings.TrimSpace(os.Getenv("FIELD_SERVICE_DEMO_SEED")), "true"),
			DemoPassword:       strings.TrimSpace(os.Getenv("FIELD_SERVICE_DEMO_PASSWORD")),
			ProviderPortalURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("FIELD_SERVICE_PORTAL_URL")), "/"),
			PilotSeedProfile:   strings.TrimSpace(os.Getenv("FIELD_SERVICE_PILOT_SEED_PROFILE")),
			PilotPassword:      strings.TrimSpace(os.Getenv("FIELD_SERVICE_PILOT_PASSWORD")),
			AdoptPhoneNumberID: strings.TrimSpace(os.Getenv("WHATSAPP_ADOPT_PHONE_NUMBER_ID")),
			AdoptTargetOrgSlug: strings.TrimSpace(os.Getenv("WHATSAPP_ADOPT_TARGET_ORG_SLUG")),
			AdoptVerifyToken:   strings.TrimSpace(os.Getenv("WHATSAPP_ADOPT_VERIFY_TOKEN")),
			AdoptBotID:         strings.TrimSpace(os.Getenv("WHATSAPP_ADOPT_BOT_ID")),
		},
		AI: AIConfig{
			Provider:              getenv("AI_PROVIDER", "ollama"),
			Model:                 strings.TrimSpace(os.Getenv("AI_MODEL")),
			OllamaBaseURL:         strings.TrimRight(getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434"), "/"),
			OllamaChatModel:       getenv("OLLAMA_CHAT_MODEL", "llama3.2:3b"),
			GroqAPIKey:            strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
			GroqBaseURL:           strings.TrimRight(getenv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"), "/"),
			GroqModel:             getenv("GROQ_MODEL", "openai/gpt-oss-20b"),
			EmbeddingsEnabled:     boolEnv("AI_EMBEDDINGS_ENABLED", false),
			VectorSearchEnabled:   boolEnv("AI_VECTOR_SEARCH_ENABLED", false),
			EmbeddingProvider:     getenv("AI_EMBEDDING_PROVIDER", "disabled"),
			EmbeddingModel:        getenv("AI_EMBEDDING_MODEL", "text-embedding-3-small"),
			EmbeddingDimensions:   intEnv("AI_EMBEDDING_DIMENSIONS", 1536),
			VectorSearchThreshold: floatEnv("AI_VECTOR_SEARCH_THRESHOLD", 0.72),
		},
		Channels: ChannelConfig{
			SecretEncryptionKey:          strings.TrimSpace(os.Getenv("CHANNEL_SECRET_ENCRYPTION_KEY")),
			WhatsAppGraphBaseURL:         strings.TrimRight(getenv("WHATSAPP_GRAPH_BASE_URL", "https://graph.facebook.com"), "/"),
			WhatsAppWebhookPublicBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("WHATSAPP_WEBHOOK_PUBLIC_BASE_URL")), "/"),
			WhatsAppSignatureBypass:      boolEnv("WHATSAPP_SIGNATURE_BYPASS", false),
			MetaAppID:                    strings.TrimSpace(os.Getenv("META_APP_ID")),
			MetaAppSecret:                strings.TrimSpace(os.Getenv("META_APP_SECRET")),
			MetaEmbeddedConfigurationID:  strings.TrimSpace(os.Getenv("META_EMBEDDED_SIGNUP_CONFIGURATION_ID")),
			MetaGraphAPIVersion:          strings.TrimSpace(os.Getenv("META_GRAPH_API_VERSION")),
			MetaWebhookVerifyToken:       strings.TrimSpace(os.Getenv("META_WEBHOOK_VERIFY_TOKEN")),
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
	maxToolCalls, err := strconv.Atoi(getenv("AI_MAX_TOOL_CALLS", "4"))
	if err != nil || maxToolCalls <= 0 {
		return Config{}, errors.New("AI_MAX_TOOL_CALLS must be a positive integer")
	}
	cfg.AI.MaxToolCalls = maxToolCalls
	if cfg.AI.EmbeddingDimensions <= 0 {
		return Config{}, errors.New("AI_EMBEDDING_DIMENSIONS must be a positive integer")
	}
	if cfg.AI.VectorSearchThreshold <= 0 || cfg.AI.VectorSearchThreshold > 1 {
		return Config{}, errors.New("AI_VECTOR_SEARCH_THRESHOLD must be between 0 and 1")
	}
	if strings.EqualFold(cfg.AppEnv, "production") && cfg.Channels.WhatsAppSignatureBypass {
		return Config{}, errors.New("WHATSAPP_SIGNATURE_BYPASS cannot be enabled in production")
	}
	if baseURL := cfg.Channels.WhatsAppWebhookPublicBaseURL; baseURL != "" {
		parsed, parseErr := url.Parse(baseURL)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, errors.New("WHATSAPP_WEBHOOK_PUBLIC_BASE_URL must be an absolute HTTP or HTTPS origin without a path")
		}
		if strings.EqualFold(cfg.AppEnv, "production") && parsed.Scheme != "https" {
			return Config{}, errors.New("WHATSAPP_WEBHOOK_PUBLIC_BASE_URL must use HTTPS in production")
		}
	}

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

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func floatEnv(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
