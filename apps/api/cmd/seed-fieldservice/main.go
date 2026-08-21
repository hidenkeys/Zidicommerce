package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/database"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/fieldservice"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/migrations"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	db, sqlDB, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		slog.Error("db", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()
	if err := migrations.Run(ctx, sqlDB, cfg.MigrationsDir); err != nil {
		slog.Error("migrations", "error", err)
		os.Exit(1)
	}
	commerce := core.NewService(db, core.SafeTestProvider{})
	bots := bot.NewService(db)
	field := fieldservice.NewService(db, commerce, slog.Default())
	// Booking-fee and quote payments only advance a request through the same
	// payment callback the API wires up, so the seeder needs it too.
	commerce.ConfigureAfterPaymentPaid(field.OnPaymentPaid)
	result, err := fieldservice.SeedDemo(ctx, db, commerce, bots, field, fieldservice.SeedOptions{
		Password:          os.Getenv("FIELD_SERVICE_DEMO_PASSWORD"),
		ProviderPortalURL: os.Getenv("FIELD_SERVICE_PORTAL_URL"),
	})
	if err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Seeded %s\nOwner: %s\nSample provider: %s\n", fieldservice.DemoCompanyName, result.OwnerEmail, result.ProviderEmail)
	if os.Getenv("FIELD_SERVICE_DEMO_PASSWORD") == "" {
		fmt.Printf("Password: %s (local development default)\n", fieldservice.DemoPassword)
	} else {
		fmt.Println("Password: taken from FIELD_SERVICE_DEMO_PASSWORD")
	}
}
