package fieldservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLoadSeedProfileValidatesReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(`{
		"organization_name":"Pilot","organization_slug":"pilot",
		"owner":{"email":"owner@pilot.demo"},
		"pools":["Plumber"],
		"providers":[{"code":"one","name":"One Provider","email":"one@pilot.demo","pools":["Electrician"]}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeedProfile(path); err == nil {
		t.Fatal("expected an unknown provider pool to fail validation")
	}
}

func TestDFixItSeedProfile(t *testing.T) {
	profile, err := LoadSeedProfile(filepath.Join("..", "..", "..", "..", "seed-profiles", "dfix-it.json"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.OrganizationSlug != "dfix-it" || len(profile.Pools) != 8 || len(profile.Providers) != 20 || len(profile.Jobs) < 6 {
		t.Fatalf("unexpected DFix It profile: slug=%s pools=%d providers=%d jobs=%d", profile.OrganizationSlug, len(profile.Pools), len(profile.Providers), len(profile.Jobs))
	}
}

func TestSeedTenantIsIdempotentAndOrganizationScoped(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{}, &organization.User{}, &organization.OrganizationMembership{}, &organization.AuditLog{},
		&core.Store{}, &core.StoreHour{}, &core.StoreFulfilmentMode{}, &core.Category{}, &core.Product{}, &core.Variant{}, &core.ProductImage{},
		&core.InventoryLevel{}, &core.Customer{}, &core.Cart{}, &core.CartItem{}, &core.Order{}, &core.OrderItem{}, &core.OrderEvent{},
		&core.Payment{}, &core.Fulfilment{}, &core.Channel{}, &core.CommerceNotification{},
		&Settings{}, &Pool{}, &Provider{}, &ProviderPool{}, &Request{}, &Match{}, &DispatchAttempt{}, &Assignment{}, &Quote{}, &QuoteItem{}, &Rating{}, &Message{},
		&bot.Bot{}, &bot.BotVersion{}, &bot.VersionModule{}, &bot.Variable{}, &bot.Question{}, &bot.Action{}, &bot.Condition{}, &bot.Integration{}, &bot.Step{}, &bot.PublishedSnapshot{},
	); err != nil {
		t.Fatal(err)
	}
	commerce := core.NewService(db, core.SafeTestProvider{})
	field := NewService(db, commerce, nil)
	bots := bot.NewService(db)
	commerce.ConfigureAfterPaymentPaid(field.OnPaymentPaid)
	profile := SeedProfile{
		OrganizationName: "Pilot Services", OrganizationSlug: "pilot-services",
		Owner:          SeedOwnerProfile{Email: "owner@pilot.demo", FirstName: "Pilot", LastName: "Owner"},
		WelcomeMessage: "Welcome to Pilot Services",
		Pools:          []string{"Plumber"},
		Providers: []SeedProviderProfile{{
			Code: "provider-one", Name: "Provider One", Email: "provider@pilot.demo", Phone: "+2347000000001",
			Area: "Ikeja", Latitude: 6.6018, Longitude: 3.3515, Pools: []string{"Plumber"}, Rating: 4.7,
			RatingCount: 10, JobsCompleted: 12, Availability: AvailabilityAvailable,
		}},
	}
	options := SeedOptions{Password: "A-Secure-Test-Password-1!", ProviderPortalURL: "https://field.example.com"}
	first, err := SeedTenant(context.Background(), db, commerce, bots, field, profile, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SeedTenant(context.Background(), db, commerce, bots, field, profile, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.OrganizationID != second.OrganizationID || first.BotID != second.BotID {
		t.Fatal("idempotent seed changed organization or bot identity")
	}
	assertCount := func(model any, want int64) {
		t.Helper()
		var count int64
		if err := db.Model(model).Where("organization_id = ?", first.OrganizationID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("%T count = %d, want %d", model, count, want)
		}
	}
	assertCount(&Pool{}, 1)
	assertCount(&Provider{}, 1)
	assertCount(&bot.Bot{}, 1)
	assertCount(&bot.PublishedSnapshot{}, 1)
	assertCount(&core.Channel{}, 2)

	var providerUser organization.User
	if err := db.Where("lower(email) = ?", "provider@pilot.demo").First(&providerUser).Error; err != nil {
		t.Fatal(err)
	}
	if providerUser.OrganizationID == nil || *providerUser.OrganizationID != first.OrganizationID {
		t.Fatal("provider login was not scoped to the seeded organization")
	}
}
