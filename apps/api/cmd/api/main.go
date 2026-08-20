package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/database"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httpapi"
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
	var mailer email.Sender = email.NewLogSender(log, cfg.Email.From)
	if cfg.Email.Mode == "smtp" {
		mailer = email.NewSMTPSender(cfg.Email.SMTPHost, cfg.Email.SMTPPort, cfg.Email.SMTPUser, cfg.Email.SMTPPass, cfg.Email.From)
	}
	commerceService.ConfigureNotifications(mailer, cfg.Email.AppBaseURL)
	botService := bot.NewService(db)
	runtimeService := runtimeengine.NewService(db, commerceService, log)

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
	})

	log.Info("starting ZidiCommerce API", "port", cfg.ServerPort, "env", cfg.AppEnv)
	if err := app.Listen(":" + cfg.ServerPort); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
