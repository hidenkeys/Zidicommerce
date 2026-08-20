package core

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type commerceFixture struct {
	db       *gorm.DB
	service  *Service
	actor    auth.CurrentUser
	customer Customer
	store    Store
	product  Product
	variant  Variant
}

func newCommerceFixture(t *testing.T, inventory int) commerceFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{},
		&organization.User{},
		&Store{},
		&StoreHour{},
		&StoreFulfilmentMode{},
		&Category{},
		&Product{},
		&Variant{},
		&ProductImage{},
		&InventoryLevel{},
		&Customer{},
		&Cart{},
		&CartItem{},
		&Order{},
		&OrderItem{},
		&OrderEvent{},
		&Payment{},
		&Fulfilment{},
		&Channel{},
	); err != nil {
		t.Fatal(err)
	}

	orgID := uuid.New()
	userID := uuid.New()
	actor := auth.CurrentUser{ID: userID, OrganizationID: orgID, Role: authz.MerchantAdmin}
	org := organization.Organization{ID: orgID, Name: "Test Merchant", Slug: "test-merchant", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db, SafeTestProvider{})
	store, err := service.CreateStore(context.Background(), actor, StoreInput{
		Name:    "Main Store",
		Code:    "MAIN",
		Address: "1 Test Road",
		FulfilmentModes: []StoreFulfilmentModeInput{
			{Mode: FulfilmentPickup, Enabled: true},
			{Mode: FulfilmentMerchantRider, Enabled: true, DeliveryFeeMinor: 500},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	category, err := service.CreateCategory(context.Background(), actor, CategoryInput{Name: "Drinks", Slug: "drinks"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := service.CreateProduct(context.Background(), actor, ProductInput{
		CategoryID: &category.ID,
		Name:       "Milkshake",
		Slug:       "milkshake",
		Status:     StatusActive,
		Variants: []VariantInput{
			{SKU: "MILK-REG", Name: "Regular", PriceMinor: 420000, Currency: "NGN", Status: StatusActive},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	customer, err := service.CreateCustomer(context.Background(), actor, CustomerInput{Name: "Customer", Phone: "2348000000000", Email: "customer@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	level, err := service.UpsertInventory(context.Background(), actor, InventoryCreateInput{StoreID: store.ID, VariantID: product.Variants[0].ID, OnHand: inventory, ReorderThreshold: 2})
	if err != nil {
		t.Fatal(err)
	}
	if level.OnHand != inventory {
		t.Fatalf("expected inventory %d, got %d", inventory, level.OnHand)
	}

	return commerceFixture{db: db, service: service, actor: actor, customer: customer, store: store, product: product, variant: product.Variants[0]}
}

func TestOrderCreationUsesAuthoritativePriceAndDecrementsInventory(t *testing.T) {
	fx := newCommerceFixture(t, 5)

	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentMerchantRider,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.SubtotalMinor != 840000 || order.DeliveryFeeMinor != 500 || order.TotalMinor != 840500 {
		t.Fatalf("unexpected totals: %+v", order)
	}

	var inventory InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, fx.store.ID, fx.variant.ID).First(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.OnHand != 3 {
		t.Fatalf("expected on hand to be decremented to 3, got %d", inventory.OnHand)
	}
}

func TestOrderCreationRejectsInsufficientInventory(t *testing.T) {
	fx := newCommerceFixture(t, 1)

	if _, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 2}},
	}); err == nil {
		t.Fatal("expected insufficient inventory error")
	}

	var inventory InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, fx.store.ID, fx.variant.ID).First(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.OnHand != 1 {
		t.Fatalf("inventory should remain unchanged, got %d", inventory.OnHand)
	}
}

func TestCartCalculatesBackendPrices(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	cart, err := fx.service.CreateCart(context.Background(), fx.actor, CartInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, Currency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := fx.service.AddCartItem(context.Background(), fx.actor, cart.ID, CartItemInput{VariantID: fx.variant.ID, Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if summary.SubtotalMinor != 840000 || len(summary.Items) != 1 {
		t.Fatalf("unexpected cart summary: %+v", summary)
	}
}

func TestOrderTransitionRejectsInvalidJump(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.TransitionOrder(context.Background(), fx.actor, order.ID, TransitionInput{Status: OrderCompleted}); err == nil {
		t.Fatal("expected invalid transition to be rejected")
	}
}

func TestPaymentInitializationIsIdempotent(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := PaymentInput{OrderID: order.ID, Provider: "test", Email: "customer@example.com", IdempotencyKey: "same-key"}
	first, err := fx.service.InitializePayment(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.InitializePayment(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Reference != second.Reference {
		t.Fatalf("expected idempotent payment, got %+v and %+v", first, second)
	}
}

func TestTenantIsolationPreventsCrossOrganizationStoreAccess(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	otherActor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}

	if _, err := fx.service.GetStore(context.Background(), otherActor, fx.store.ID); err == nil {
		t.Fatal("expected cross-organization store lookup to be rejected")
	}
}
