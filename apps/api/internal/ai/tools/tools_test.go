package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDefinitionsExposeOnlyReadOnlyTools(t *testing.T) {
	blocked := map[string]bool{
		"create_order":       true,
		"cancel_order":       true,
		"create_cart":        true,
		"add_to_cart":        true,
		"initialize_payment": true,
		"create_complaint":   true,
		"handoff_to_agent":   true,
	}
	for _, tool := range Definitions() {
		if blocked[tool.Function.Name] {
			t.Fatalf("write-capable tool %q was exposed", tool.Function.Name)
		}
	}
}

func TestExecuteBlocksUnregisteredWriteToolBeforeRuntime(t *testing.T) {
	registry := NewRegistry(nil, nil)
	_, call, err := registry.Execute(context.Background(), "create_order", runtime.ConversationSession{ID: uuid.New(), OrganizationID: uuid.New()}, map[string]any{})
	if err == nil {
		t.Fatal("expected write tool to be blocked")
	}
	if call.Status != "blocked" {
		t.Fatalf("expected blocked status, got %q", call.Status)
	}
}

func TestCheckInventoryResolvesIDsFromLatestUserText(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&core.Store{}, &core.StoreHour{}, &core.StoreFulfilmentMode{}, &core.Product{}, &core.Variant{}, &core.ProductImage{}, &core.InventoryLevel{}, &bot.CommerceWorkflowConfiguration{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	orgID := uuid.New()
	store := core.Store{ID: uuid.New(), OrganizationID: orgID, Name: "Bing Chun Lekki Branch", Code: "BCNG-LEKKI", Status: core.StatusActive, Address: "Admiralty Way, Lekki", Metadata: "{}"}
	product := core.Product{ID: uuid.New(), OrganizationID: orgID, Name: "Milkshake", Slug: "milkshake", Description: "Creamy milkshake", Status: core.StatusActive, Metadata: "{}"}
	variant := core.Variant{ID: uuid.New(), OrganizationID: orgID, ProductID: product.ID, SKU: "BCNG-MS-REG", Name: "Regular Milkshake", PriceMinor: 520000, Currency: "NGN", Status: core.StatusActive, Metadata: "{}"}
	level := core.InventoryLevel{ID: uuid.New(), OrganizationID: orgID, StoreID: store.ID, VariantID: variant.ID, OnHand: 8}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := db.Create(&variant).Error; err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	if err := db.Create(&level).Error; err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	registry := NewRegistry(db, core.NewService(db, core.SafeTestProvider{}))
	result, call, err := registry.Execute(context.Background(), "check_inventory", runtime.ConversationSession{ID: uuid.New(), OrganizationID: orgID}, map[string]any{
		"store_id":          "",
		"variant_id":        "",
		"quantity":          2,
		"_latest_user_text": "I would like to place an order of 2 milkshakes from the Lekki branch",
	})
	if err != nil {
		t.Fatalf("execute check_inventory: %v", err)
	}
	if call.Status != "ok" {
		t.Fatalf("expected ok call, got %#v", call)
	}
	if result["store_id"] != store.ID.String() || result["variant_id"] != variant.ID.String() {
		t.Fatalf("expected resolved store and variant IDs, got %#v", result)
	}
	if call.Inputs["store_id"] != store.ID.String() || call.Inputs["variant_id"] != variant.ID.String() {
		t.Fatalf("expected debug inputs to include resolved IDs, got %#v", call.Inputs)
	}
	if _, ok := call.Inputs["_latest_user_text"]; ok {
		t.Fatalf("hidden latest user text leaked into debug inputs: %#v", call.Inputs)
	}
}
