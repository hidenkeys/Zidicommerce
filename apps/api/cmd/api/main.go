package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/database"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/fieldservice"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httpapi"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/migrations"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	runtimeengine "github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	log := config.NewLogger(cfg.LogLevel)
	ctx := context.Background()

	db, sqlDB, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	if err := migrations.Run(ctx, sqlDB, cfg.MigrationsDir); err != nil {
		log.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	tokenManager := auth.NewTokenManager(cfg.JWT)
	userRepo := organization.NewUserRepository(db)
	orgRepo := organization.NewRepository(db)
	var provider core.PaymentProvider = core.SafeTestProvider{}
	if cfg.Payment.Provider == "paystack" {
		provider = core.NewPaystackProvider(cfg.Payment.PaystackSecret)
	}
	commerceService := core.NewService(db, provider)
	commerceService.ConfigurePaymentWebhooks(cfg.Payment.PaystackSecret)
	if key, err := core.DecodePaymentSecretKey(cfg.Payment.SecretEncryptionKey); err != nil {
		log.Error("invalid payment secret encryption key", "error", err)
		os.Exit(1)
	} else if len(key) > 0 {
		store, err := core.NewEncryptedPaymentSecretStore(db, key, "v1")
		if err != nil {
			log.Error("failed to configure payment secret store", "error", err)
			os.Exit(1)
		}
		commerceService.ConfigurePaymentSecretStore(store)
	}
	jobService := jobs.NewService(db, log)
	commerceService.ConfigureJobs(jobService)
	mailer, err := email.NewSender(cfg.Email.Mode, cfg.Email.SMTPHost, cfg.Email.SMTPPort, cfg.Email.SMTPUser, cfg.Email.SMTPPass, cfg.Email.From, cfg.Email.FromName, cfg.Email.TLSMode, log)
	if err != nil {
		log.Error("failed to configure email sender", "error", err)
		os.Exit(1)
	}
	log.Info("email sender ready", "mode", cfg.Email.Mode, "host", cfg.Email.SMTPHost, "from", cfg.Email.From)
	commerceService.ConfigureNotifications(mailer, cfg.Email.AppBaseURL, log)
	botService := bot.NewService(db)
	runtimeService := runtimeengine.NewService(db, commerceService, log)
	runtimeService.ConfigureJobs(jobService)
	runtimeService.RegisterChannelSender("whatsapp", runtimeengine.NewWhatsAppCloudSender("", log))
	fieldService := fieldservice.NewService(db, commerceService, log)
	fieldService.ConfigureJobs(jobService)
	fieldService.ConfigureDispatcher(runtimeService)
	runtimeService.RegisterFieldService(fieldService)
	runtimeService.ConfigureHandoffInbound(func(ctx context.Context, organizationID, sessionID uuid.UUID, text string) (bool, string, error) {
		return fieldService.HandleHandoffInbound(ctx, organizationID, sessionID, text)
	})
	commerceService.ConfigureAfterPaymentPaid(fieldService.OnPaymentPaid)
	jobService.Register(jobs.JobTypeChannelOutbound, runtimeService.ProcessOutboundJob)
	jobService.Register(jobs.JobTypeNotificationDelivery, runtimeService.ProcessNotificationJob)
	jobService.Register(jobs.JobTypeServiceDispatchTimeout, fieldService.ProcessDispatchTimeout)
	jobService.Start(ctx, 2*time.Second, 25)

	// The field-service pilot tenant is seeded only when explicitly enabled. The
	// seed is idempotent, is scoped to its own organization, and never touches
	// another tenant's data. A failure here must not stop the API from serving.
	if cfg.FieldService.SeedDemo {
		// The sample history the demo tenant ships with (paid booking fees,
		// settled quotes) has to be produced without moving real money, so the
		// seeder runs against its own commerce service backed by the safe test
		// provider. Live traffic keeps using the configured provider.
		seedCommerce := core.NewService(db, core.SafeTestProvider{})
		seedField := fieldservice.NewService(db, seedCommerce, log)
		seedCommerce.ConfigureAfterPaymentPaid(seedField.OnPaymentPaid)
		result, err := fieldservice.SeedDemo(ctx, db, seedCommerce, botService, seedField, fieldservice.SeedOptions{
			Password:          cfg.FieldService.DemoPassword,
			ProviderPortalURL: cfg.FieldService.ProviderPortalURL,
		})
		if err != nil {
			log.Error("field service demo seed failed", "error", err)
		} else {
			log.Info("field service demo tenant ready",
				"organization_id", result.OrganizationID,
				"owner_email", result.OwnerEmail,
				"password_source", passwordSource(cfg.FieldService.DemoPassword))
			if count, cleanupErr := commerceService.SkipUndeliverableNotifications(ctx, result.OrganizationID); cleanupErr != nil {
				log.Warn("failed to close undeliverable demo notifications", "organization_id", result.OrganizationID, "error", cleanupErr)
			} else if count > 0 {
				log.Info("closed undeliverable demo notifications", "organization_id", result.OrganizationID, "count", count)
			}
		}
	}

	app := httpapi.New(httpapi.Dependencies{
		Config:       cfg,
		Logger:       log,
		DB:           sqlDB,
		TokenManager: tokenManager,
		AuthService:  auth.NewService(userRepo, tokenManager),
		OrgHandler:   organization.NewHandler(orgRepo),
		Commerce:     core.NewHandler(commerceService, tokenManager),
		Bot:          bot.NewHandler(botService),
		Runtime:      runtimeengine.NewHandler(runtimeService),
		Field:        fieldservice.NewHandler(fieldService),
	})

	log.Info("starting ZidiCommerce API", "port", cfg.ServerPort, "env", cfg.AppEnv)
	if err := app.Listen(":" + cfg.ServerPort); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// passwordSource reports where the demo password came from without ever putting
// the password itself into the logs.
func passwordSource(configured string) string {
	if configured != "" {
		return "FIELD_SERVICE_DEMO_PASSWORD"
	}
	return "built-in development default"
}
