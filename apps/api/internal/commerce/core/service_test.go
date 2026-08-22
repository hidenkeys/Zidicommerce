package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"golang.org/x/crypto/bcrypt"
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

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{},
		&organization.User{},
		&organization.OrganizationMembership{},
		&organization.OrganizationInvitation{},
		&organization.AuditLog{},
		&Store{},
		&StoreHour{},
		&StoreFulfilmentMode{},
		&StoreUserAssignment{},
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
		&PaymentWebhookEvent{},
		&Fulfilment{},
		&Channel{},
		&CommerceNotification{},
		&MerchantImportJob{},
		&PaymentReconciliation{},
		&PaymentConfiguration{},
		&PaymentProviderSecret{},
		&jobs.Job{},
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
			{Mode: FulfilmentCustomerRider, Enabled: true},
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

func TestConcurrentCheckoutOnlyOneOrderConsumesSingleInventoryUnit(t *testing.T) {
	fx := newCommerceFixture(t, 1)
	sqlDB, err := fx.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	inputs := []OrderInput{
		{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}, IdempotencyKey: "checkout-a"},
		{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}, IdempotencyKey: "checkout-b"},
	}
	errs := make([]error, len(inputs))
	var wg sync.WaitGroup
	wg.Add(len(inputs))
	for index := range inputs {
		go func(index int) {
			defer wg.Done()
			_, errs[index] = fx.service.CreateOrder(context.Background(), fx.actor, inputs[index])
		}(index)
	}
	wg.Wait()

	successes := 0
	insufficient := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if strings.Contains(err.Error(), "Insufficient inventory") {
			insufficient++
			continue
		}
		t.Fatalf("unexpected checkout error: %v", err)
	}
	if successes != 1 || insufficient != 1 {
		t.Fatalf("expected one success and one insufficient-inventory failure, got successes=%d insufficient=%d errors=%v", successes, insufficient, errs)
	}

	var inventory InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, fx.store.ID, fx.variant.ID).First(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.OnHand != 0 {
		t.Fatalf("expected final inventory to be 0, got %d", inventory.OnHand)
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

func TestCheckoutUsesCurrentVariantPriceFromDatabase(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	cart, err := fx.service.CreateCart(context.Background(), fx.actor, CartInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, Currency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.AddCartItem(context.Background(), fx.actor, cart.ID, CartItemInput{VariantID: fx.variant.ID, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&Variant{}).Where("id = ?", fx.variant.ID).Update("price_minor", int64(500000)).Error; err != nil {
		t.Fatal(err)
	}
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{CartID: &cart.ID, CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, IdempotencyKey: "stale-cart-price"})
	if err != nil {
		t.Fatal(err)
	}
	if order.SubtotalMinor != 500000 {
		t.Fatalf("expected checkout to use current database price, got %d", order.SubtotalMinor)
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

func TestOrderTransitionIsIdempotentByKey(t *testing.T) {
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
	first, err := fx.service.TransitionOrder(context.Background(), fx.actor, order.ID, TransitionInput{Status: OrderCancelled, IdempotencyKey: "cancel-once"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.TransitionOrder(context.Background(), fx.actor, order.ID, TransitionInput{Status: OrderCancelled, IdempotencyKey: "cancel-once"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != OrderCancelled || second.Status != OrderCancelled {
		t.Fatalf("expected cancelled order on replay, got %s then %s", first.Status, second.Status)
	}
	var events int64
	if err := fx.db.Model(&OrderEvent{}).Where("organization_id = ? AND order_id = ? AND idempotency_key = ?", fx.actor.OrganizationID, order.ID, "cancel-once").Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("expected one idempotent order event, got %d", events)
	}
}

func TestOrderCreationIsIdempotentAndDoesNotDoubleDecrementInventory(t *testing.T) {
	fx := newCommerceFixture(t, 2)
	input := OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
		IdempotencyKey: "checkout-once",
	}
	first, err := fx.service.CreateOrder(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.CreateOrder(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same order on duplicate checkout, got %s and %s", first.ID, second.ID)
	}
	var inventory InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, fx.store.ID, fx.variant.ID).First(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.OnHand != 1 {
		t.Fatalf("expected inventory decremented once, got %d", inventory.OnHand)
	}
}

func TestOrderCancellationRestoresInventory(t *testing.T) {
	fx := newCommerceFixture(t, 1)
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.TransitionOrder(context.Background(), fx.actor, order.ID, TransitionInput{Status: OrderCancelled, IdempotencyKey: "cancel-restore"}); err != nil {
		t.Fatal(err)
	}
	var inventory InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, fx.store.ID, fx.variant.ID).First(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.OnHand != 1 {
		t.Fatalf("expected cancellation to restore inventory to 1, got %d", inventory.OnHand)
	}
}

func TestUpdateFulfilmentRejectsInvalidTransition(t *testing.T) {
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
	if _, err := fx.service.UpdateFulfilment(context.Background(), fx.actor, order.ID, FulfilmentInput{Status: "out_for_delivery"}); err == nil {
		t.Fatal("expected invalid pickup fulfilment transition to fail")
	}
}

func TestFulfilmentTransitionsForSupportedModes(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		transitions []string
	}{
		{name: "pickup", mode: FulfilmentPickup, transitions: []string{"ready", "completed"}},
		{name: "customer rider", mode: FulfilmentCustomerRider, transitions: []string{"ready", "completed"}},
		{name: "merchant rider", mode: FulfilmentMerchantRider, transitions: []string{"ready", "out_for_delivery", "completed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newCommerceFixture(t, 5)
			order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
				CustomerID:     fx.customer.ID,
				StoreID:        fx.store.ID,
				FulfilmentType: tc.mode,
				Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, status := range tc.transitions {
				fulfilment, err := fx.service.UpdateFulfilment(context.Background(), fx.actor, order.ID, FulfilmentInput{Status: status})
				if err != nil {
					t.Fatal(err)
				}
				if fulfilment.Status != status {
					t.Fatalf("expected fulfilment status %s, got %s", status, fulfilment.Status)
				}
			}
		})
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

func TestOnboardOrganizationCreatesMerchantAdminMembership(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	userID := uuid.New()
	if err := fx.db.Create(&organization.User{ID: userID, Email: "new@example.com", PasswordHash: "hash", Role: authz.Viewer, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}

	org, membership, err := fx.service.OnboardOrganization(context.Background(), auth.CurrentUser{ID: userID, Role: authz.Viewer}, OrganizationInput{
		Name:         "New Merchant",
		Slug:         "new-merchant",
		Country:      "GB",
		Currency:     "GBP",
		Timezone:     "Europe/London",
		ContactEmail: "owner@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if org.ID == uuid.Nil || membership.Role != authz.MerchantAdmin || !membership.IsOwner {
		t.Fatalf("unexpected onboarding output: %+v %+v", org, membership)
	}

	var user organization.User
	if err := fx.db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	if user.OrganizationID == nil || *user.OrganizationID != org.ID || user.Role != authz.MerchantAdmin {
		t.Fatalf("expected user to become merchant admin for org, got %+v", user)
	}
}

func TestInvitationAcceptanceIsSingleUse(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	invitation, token, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{
		Email:     "staff@example.com",
		FirstName: "Store",
		LastName:  "Staff",
		Role:      authz.StoreStaff.String(),
		StoreIDs:  []uuid.UUID{fx.store.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(invitation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), invitation.TokenHash) {
		t.Fatal("invitation token hash should not be serialized")
	}

	accepted, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: token, Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Role != authz.StoreStaff.String() || accepted.OrganizationID != fx.actor.OrganizationID {
		t.Fatalf("unexpected acceptance: %+v", accepted)
	}
	var assignments int64
	if err := fx.db.Model(&StoreUserAssignment{}).Where("organization_id = ? AND store_id = ? AND user_id = ?", fx.actor.OrganizationID, fx.store.ID, accepted.UserID).Count(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	if assignments != 1 {
		t.Fatalf("expected invitation store assignment to be created, got %d", assignments)
	}
	if _, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: token, Password: "password123"}); err == nil {
		t.Fatal("expected second acceptance to be rejected")
	}
}

func TestInvitationExpirationRejectsAcceptance(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	_, token, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "expired@example.com", Role: authz.Viewer.String()})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&organization.OrganizationInvitation{}).Where("token_hash = ?", hashInvitationToken(token)).Update("expires_at", fx.service.now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: token, Password: "password123"}); err == nil {
		t.Fatal("expected expired invitation to be rejected")
	}
}

func TestInviteMemberRejectsPlatformAdminRole(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	if _, _, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "bad@example.com", Role: authz.PlatformAdmin.String()}); err == nil {
		t.Fatal("expected platform admin invitation to be rejected")
	}
}

type capturingMailer struct {
	messages []email.Message
	err      error
}

func (m *capturingMailer) Send(message email.Message) error {
	m.messages = append(m.messages, message)
	return m.err
}

func TestInviteMemberSendsProfessionalEmail(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	mailer := &capturingMailer{}
	fx.service.ConfigureNotifications(mailer, "https://admin.zidicommerce.example", nil)
	_ = fx.db.Create(&organization.User{ID: fx.actor.ID, Email: "owner@example.com", FirstName: "Ada", LastName: "Merchant", PasswordHash: "x", Role: authz.MerchantAdmin, Status: "active"}).Error

	invitation, token, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{
		Email:     "staff@example.com",
		FirstName: "Store",
		LastName:  "Staff",
		Role:      authz.StoreStaff.String(),
		StoreIDs:  []uuid.UUID{fx.store.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if invitation.EmailStatus != "sent" || token == "" {
		t.Fatalf("expected sent invitation, got %+v token=%q", invitation, token)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("expected one email, got %d", len(mailer.messages))
	}
	message := mailer.messages[0]
	if !strings.Contains(message.Subject, "Test Merchant") {
		t.Fatalf("subject missing business name: %q", message.Subject)
	}
	if !strings.Contains(message.HTML, "Accept invitation") || !strings.Contains(message.HTML, "https://admin.zidicommerce.example/invitations/accept?token="+token) {
		t.Fatal("invitation email missing accept URL")
	}
	if strings.Contains(message.HTML, invitation.TokenHash) || strings.Contains(message.Text, invitation.ID.String()) {
		t.Fatal("invitation email leaked internal identifiers")
	}
}

func TestDuplicateInvitationReusesPendingAndResends(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	mailer := &capturingMailer{}
	fx.service.ConfigureNotifications(mailer, "https://admin.example.com", nil)
	first, firstToken, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "repeat@example.com", Role: authz.Viewer.String()})
	if err != nil {
		t.Fatal(err)
	}
	second, secondToken, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "repeat@example.com", Role: authz.StoreStaff.String(), StoreIDs: []uuid.UUID{fx.store.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatal("expected pending invitation to be reused")
	}
	if firstToken == secondToken {
		t.Fatal("expected resend to rotate the invitation token")
	}
	if second.Role != authz.StoreStaff {
		t.Fatalf("expected role update, got %s", second.Role)
	}
	var count int64
	if err := fx.db.Model(&organization.OrganizationInvitation{}).Where("organization_id = ? AND email = ?", fx.actor.OrganizationID, "repeat@example.com").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one invitation row, got %d", count)
	}
	if len(mailer.messages) != 2 {
		t.Fatalf("expected two emails, got %d", len(mailer.messages))
	}
}

func TestInvitationEmailFailureKeepsInvitation(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	mailer := &capturingMailer{err: httperror.Unavailable("smtp authentication failed")}
	fx.service.ConfigureNotifications(mailer, "https://admin.example.com", nil)
	invitation, _, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "fail@example.com", Role: authz.Viewer.String()})
	if err == nil {
		t.Fatal("expected email failure to be returned")
	}
	if !strings.Contains(err.Error(), "Invitation could not be sent") {
		t.Fatalf("expected human-readable email error, got %v", err)
	}
	var stored organization.OrganizationInvitation
	if err := fx.db.Where("id = ?", invitation.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "pending" || stored.EmailStatus != "failed" {
		t.Fatalf("expected pending invitation with failed email, got %+v", stored)
	}
}

func TestExistingUserInvitationRequiresMatchingPassword(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	hash, err := bcrypt.GenerateFromPassword([]byte("existing-pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	user := organization.User{ID: uuid.New(), Email: "existing@example.com", PasswordHash: string(hash), FirstName: "Existing", Role: authz.Viewer, Status: "active"}
	if err := fx.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	_, token, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "existing@example.com", Role: authz.StoreStaff.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: token, Password: "wrong-password"}); err == nil {
		t.Fatal("expected wrong password to be rejected")
	}
	accepted, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: token, Password: "existing-pass"})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.UserID != user.ID || accepted.Role != authz.StoreStaff.String() {
		t.Fatalf("unexpected acceptance %+v", accepted)
	}
}

func TestInviteMemberRejectsExistingActiveMember(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	user := organization.User{ID: uuid.New(), Email: "member@example.com", PasswordHash: "x", Role: authz.StoreStaff, Status: "active"}
	if err := fx.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&organization.OrganizationMembership{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, UserID: user.ID, Role: authz.StoreStaff, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "member@example.com", Role: authz.Viewer.String()}); err == nil {
		t.Fatal("expected existing member invitation to be rejected")
	}
}

func TestPreviewInvitationDoesNotExposeTokenHash(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	invitation, token, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "preview@example.com", Role: authz.Viewer.String()})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := fx.service.PreviewInvitation(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), invitation.TokenHash) || strings.Contains(string(encoded), invitation.ID.String()) {
		t.Fatalf("preview leaked internal identifiers: %s", encoded)
	}
	if preview.BusinessName != "Test Merchant" || preview.Email != "preview@example.com" {
		t.Fatalf("unexpected preview %+v", preview)
	}
}

func TestResendInvitationRotatesTokenAndRejectsAccepted(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	mailer := &capturingMailer{}
	fx.service.ConfigureNotifications(mailer, "https://admin.example.com", nil)
	invitation, oldToken, err := fx.service.InviteMember(context.Background(), fx.actor, InviteInput{Email: "resend@example.com", Role: authz.Viewer.String()})
	if err != nil {
		t.Fatal(err)
	}
	resent, err := fx.service.ResendInvitation(context.Background(), fx.actor, invitation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resent.TokenHash == invitation.TokenHash {
		t.Fatal("expected resend to rotate token hash")
	}
	if _, err := fx.service.PreviewInvitation(context.Background(), oldToken); err == nil {
		t.Fatal("old invitation token should no longer preview")
	}
	if _, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: oldToken, Password: "password123"}); err == nil {
		t.Fatal("old invitation token should no longer accept")
	}
	newToken := tokenFromAcceptURL(mailer.messages[len(mailer.messages)-1].Text)
	if _, err := fx.service.AcceptInvitation(context.Background(), AcceptInvitationInput{Token: newToken, Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ResendInvitation(context.Background(), fx.actor, invitation.ID); err == nil {
		t.Fatal("accepted invitations cannot be resent")
	}
}

func tokenFromAcceptURL(body string) string {
	const marker = "token="
	idx := strings.LastIndex(body, marker)
	if idx < 0 {
		return ""
	}
	token := strings.TrimSpace(body[idx+len(marker):])
	if cut := strings.IndexAny(token, " \n\r"); cut >= 0 {
		token = token[:cut]
	}
	return token
}

func TestStoreStaffSeesAssignedStoreOnly(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	otherStore, err := fx.service.CreateStore(context.Background(), fx.actor, StoreInput{Name: "Other Store", Code: "OTHER"})
	if err != nil {
		t.Fatal(err)
	}
	staffID := uuid.New()
	orgID := fx.actor.OrganizationID
	if err := fx.db.Create(&organization.User{ID: staffID, OrganizationID: &orgID, Email: "staff-scope@example.com", PasswordHash: "hash", Role: authz.StoreStaff, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	membership := organization.OrganizationMembership{ID: uuid.New(), OrganizationID: orgID, UserID: staffID, Role: authz.StoreStaff, Status: "active"}
	if err := fx.db.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.AssignMemberStores(context.Background(), fx.actor, membership.ID, []uuid.UUID{otherStore.ID}); err != nil {
		t.Fatal(err)
	}

	stores, err := fx.service.ListStores(context.Background(), auth.CurrentUser{ID: staffID, OrganizationID: orgID, Role: authz.StoreStaff})
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 1 || stores[0].ID != otherStore.ID {
		t.Fatalf("expected only assigned store, got %+v", stores)
	}
}

func TestDisabledMemberCannotAuthenticate(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	userID := uuid.New()
	orgID := fx.actor.OrganizationID
	if err := fx.db.Create(&organization.User{ID: userID, OrganizationID: &orgID, Email: "disabled@example.com", PasswordHash: "hash", Role: authz.StoreStaff, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&organization.OrganizationMembership{ID: uuid.New(), OrganizationID: orgID, UserID: userID, Role: authz.StoreStaff, Status: "disabled"}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := organization.NewUserRepository(fx.db).FindByEmail(context.Background(), "disabled@example.com")
	if err == nil || !strings.Contains(err.Error(), "active organization membership") {
		t.Fatalf("expected disabled membership to block login, got %v", err)
	}
}

func TestPaystackWebhookIsSignatureVerifiedAndIdempotent(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	fx.service.paymentProvider = namedPaymentProvider{name: "paystack"}
	fx.service.ConfigurePaymentWebhooks("paystack-secret")
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payment, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "paystack", Email: "customer@example.com", IdempotencyKey: "pay-1"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"event":"charge.success","data":{"id":12345,"reference":"` + payment.Reference + `","status":"success","amount":420000,"currency":"NGN"}}`)
	signature := paystackTestSignature("paystack-secret", body)
	first, err := fx.service.HandlePaystackWebhook(context.Background(), body, signature)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.HandlePaystackWebhook(context.Background(), body, signature)
	if err != nil {
		t.Fatal(err)
	}
	if first.EventID != second.EventID || first.Status != paymentWebhookProcessed || second.Status != paymentWebhookProcessed {
		t.Fatalf("expected idempotent processed webhook, got %+v then %+v", first, second)
	}
	var updated Payment
	if err := fx.db.Where("id = ?", payment.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != PaymentPaid {
		t.Fatalf("expected payment paid, got %s", updated.Status)
	}
	var updatedOrder Order
	if err := fx.db.Where("id = ?", order.ID).First(&updatedOrder).Error; err != nil {
		t.Fatal(err)
	}
	if updatedOrder.Status != OrderPaid {
		t.Fatalf("expected order paid, got %s", updatedOrder.Status)
	}
	var eventCount int64
	if err := fx.db.Model(&PaymentWebhookEvent{}).Where("provider = ? AND external_event_id = ?", "paystack", "12345").Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("expected one webhook event, got %d", eventCount)
	}
	var notifications int64
	if err := fx.db.Model(&CommerceNotification{}).Where("organization_id = ? AND order_id = ? AND notification_type = ?", fx.actor.OrganizationID, order.ID, "payment_confirmed").Count(&notifications).Error; err != nil {
		t.Fatal(err)
	}
	if notifications != 1 {
		t.Fatalf("expected one payment notification, got %d", notifications)
	}
}

func TestPaystackWebhookRejectsProviderVerificationMismatch(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	fx.service.paymentProvider = namedPaymentProvider{name: "paystack", paid: true, amountMinor: 100, currency: "NGN"}
	fx.service.ConfigurePaymentWebhooks("paystack-secret")
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payment, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "paystack", Email: "customer@example.com", IdempotencyKey: "pay-mismatch"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"event":"charge.success","data":{"id":22345,"reference":"` + payment.Reference + `","status":"success","amount":420000,"currency":"NGN"}}`)
	result, err := fx.service.HandlePaystackWebhook(context.Background(), body, paystackTestSignature("paystack-secret", body))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != paymentWebhookFailed {
		t.Fatalf("expected failed webhook, got %+v", result)
	}
	var updated Payment
	if err := fx.db.Where("id = ?", payment.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != PaymentPending {
		t.Fatalf("expected payment to remain pending, got %s", updated.Status)
	}
}

func TestReconcilePaymentAutoMarksPaidWhenProviderMatches(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	fx.service.paymentProvider = namedPaymentProvider{name: "test", paid: true, amountMinor: 420000, currency: "NGN"}
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payment, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "test", Email: "customer@example.com", IdempotencyKey: "pay-reconcile"})
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := fx.service.ReconcilePayment(context.Background(), fx.actor, payment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.Status != "auto_reconciled" || reconciliation.ActionTaken != "marked_paid" {
		t.Fatalf("expected auto reconciled payment, got %+v", reconciliation)
	}
	var updated Order
	if err := fx.db.Where("id = ?", order.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != OrderPaid {
		t.Fatalf("expected reconciled order paid, got %s", updated.Status)
	}
}

func TestReconcilePaymentPreservesDiscrepancyWhenLocalPaidProviderFailed(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	fx.service.paymentProvider = namedPaymentProvider{name: "test", paid: true, amountMinor: 420000, currency: "NGN"}
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{
		CustomerID:     fx.customer.ID,
		StoreID:        fx.store.ID,
		FulfilmentType: FulfilmentPickup,
		Items:          []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payment, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "test", Email: "customer@example.com", IdempotencyKey: "pay-discrepancy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&payment).Updates(map[string]any{"status": PaymentPaid}).Error; err != nil {
		t.Fatal(err)
	}
	fx.service.paymentProvider = namedPaymentProvider{name: "test", paid: false, status: "failed", amountMinor: 420000, currency: "NGN"}
	reconciliation, err := fx.service.ReconcilePayment(context.Background(), fx.actor, payment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.Status != "review_required" || reconciliation.Discrepancy == "" {
		t.Fatalf("expected review-required discrepancy, got %+v", reconciliation)
	}
	var updated Payment
	if err := fx.db.Where("id = ?", payment.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != PaymentPaid {
		t.Fatalf("expected local paid status to be preserved, got %s", updated.Status)
	}
}

func TestMerchantImportCreatesConfigurationTransactionally(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	input := MerchantImportInput{
		Stores:     []StoreImportInput{{ExternalKey: "store-1", Name: "Import Store", Code: "IMP", FulfilmentModes: []StoreFulfilmentModeInput{{Mode: FulfilmentPickup, Enabled: true}}}},
		Categories: []CategoryImportInput{{ExternalKey: "cat-1", Name: "Drinks", Slug: "drinks"}},
		Products: []ProductImportInput{{
			ExternalKey:         "prod-1",
			CategoryExternalKey: "cat-1",
			Name:                "Imported Tea",
			Slug:                "imported-tea",
			Variants:            []VariantImportInput{{ExternalKey: "variant-1", SKU: "IMP-TEA", Name: "Regular", PriceMinor: 300000, Currency: "NGN"}},
			Images:              []ProductImageInput{{URL: "https://example.com/tea.png", AltText: "Imported Tea"}},
		}},
		Inventory: []InventoryImportInput{{StoreExternalKey: "store-1", VariantExternalKey: "variant-1", OnHand: 12, ReorderThreshold: 3}},
		Channels:  []ChannelImportInput{{Provider: "whatsapp", DisplayName: "WhatsApp", PhoneNumberID: "phone-import", Status: StatusActive, Config: `{}`}},
	}
	result, err := fx.service.ImportMerchantConfiguration(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID == uuid.Nil || result.Stores["store-1"] == uuid.Nil || result.Variants["variant-1"] == uuid.Nil || len(result.Channels) != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	var jobs int64
	if err := fx.db.Model(&MerchantImportJob{}).Where("organization_id = ? AND status = ?", fx.actor.OrganizationID, "completed").Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("expected completed import job, got %d", jobs)
	}
	second, err := fx.service.ImportMerchantConfiguration(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.Stores["store-1"] != result.Stores["store-1"] || second.Variants["variant-1"] != result.Variants["variant-1"] || second.Channels[0] != result.Channels[0] {
		t.Fatalf("expected repeat import to reuse records, first=%+v second=%+v", result, second)
	}
	counts := map[string]struct {
		model any
		want  int64
	}{
		"stores":     {&Store{}, 2},
		"categories": {&Category{}, 1},
		"products":   {&Product{}, 2},
		"variants":   {&Variant{}, 2},
		"inventory":  {&InventoryLevel{}, 2},
		"channels":   {&Channel{}, 1},
	}
	for label, check := range counts {
		var got int64
		if err := fx.db.Model(check.model).Where("organization_id = ?", fx.actor.OrganizationID).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Fatalf("expected %s count %d after repeat import, got %d", label, check.want, got)
		}
	}
}

func TestMerchantImportRejectsInvalidReferencesWithoutPartialWrites(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	_, err := fx.service.ImportMerchantConfiguration(context.Background(), fx.actor, MerchantImportInput{
		Stores: []StoreImportInput{{ExternalKey: "store-1", Name: "Import Store", Code: "IMP"}},
		Products: []ProductImportInput{{
			ExternalKey:         "prod-1",
			CategoryExternalKey: "missing",
			Name:                "Imported Tea",
			Slug:                "imported-tea",
		}},
	})
	if err == nil {
		t.Fatal("expected import validation error")
	}
	var stores int64
	if err := fx.db.Model(&Store{}).Where("code = ?", "IMP").Count(&stores).Error; err != nil {
		t.Fatal(err)
	}
	if stores != 0 {
		t.Fatalf("expected no partial store write, got %d", stores)
	}
}

func TestPaymentConfigurationDoesNotExposeSecrets(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	config, err := fx.service.UpsertPaymentConfiguration(context.Background(), fx.actor, PaymentConfigurationInput{
		Provider:     "paystack",
		DisplayName:  "Paystack",
		Enabled:      true,
		PublicConfig: `{"mode":"test"}`,
		SecretSource: "environment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.SecretSource != "environment" || config.Enabled != true {
		t.Fatalf("unexpected config: %+v", config)
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret_config") {
		t.Fatalf("payment config JSON exposed secret field: %s", string(body))
	}
	tested, err := fx.service.TestPaymentConfiguration(context.Background(), fx.actor, "paystack")
	if err != nil {
		t.Fatal(err)
	}
	if tested.Status != "needs_attention" || tested.TestedAt == nil {
		t.Fatalf("expected unavailable Paystack config without credentials, got %+v", tested)
	}
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}, IdempotencyKey: "missing-paystack-order"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "paystack", Email: "customer@example.com", IdempotencyKey: "missing-paystack-payment"}); err == nil {
		t.Fatal("expected Paystack initialization to fail when configuration test is not active")
	}
}

func TestPaymentConfigurationStatusMatchesProviderResolution(t *testing.T) {
	t.Run("safe test provider active when configured as test", func(t *testing.T) {
		fx := newCommerceFixture(t, 5)
		if _, err := fx.service.UpsertPaymentConfiguration(context.Background(), fx.actor, PaymentConfigurationInput{Provider: "test", DisplayName: "Test provider", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		tested, err := fx.service.TestPaymentConfiguration(context.Background(), fx.actor, "test")
		if err != nil {
			t.Fatal(err)
		}
		if tested.Status != StatusActive {
			t.Fatalf("expected safe test provider to be active, got %+v", tested)
		}
		order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}, IdempotencyKey: "test-provider-order"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "test", Email: "customer@example.com", IdempotencyKey: "test-provider-payment"}); err != nil {
			t.Fatalf("expected active test provider to initialize, got %v", err)
		}
	})

	t.Run("environment paystack active only when provider is usable", func(t *testing.T) {
		fx := newCommerceFixture(t, 5)
		fx.service.paymentProvider = namedPaymentProvider{name: "paystack", amountMinor: fx.variant.PriceMinor, currency: "NGN"}
		if _, err := fx.service.UpsertPaymentConfiguration(context.Background(), fx.actor, PaymentConfigurationInput{Provider: "paystack", DisplayName: "Paystack", Enabled: true, SecretSource: "environment"}); err != nil {
			t.Fatal(err)
		}
		tested, err := fx.service.TestPaymentConfiguration(context.Background(), fx.actor, "paystack")
		if err != nil {
			t.Fatal(err)
		}
		if tested.Status != StatusActive {
			t.Fatalf("expected environment-backed Paystack to be active, got %+v", tested)
		}
		order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}, IdempotencyKey: "env-paystack-order"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "paystack", Email: "customer@example.com", IdempotencyKey: "env-paystack-payment"}); err != nil {
			t.Fatalf("expected active environment Paystack to initialize, got %v", err)
		}
	})

	t.Run("blank paystack provider is not usable", func(t *testing.T) {
		fx := newCommerceFixture(t, 5)
		fx.service.paymentProvider = NewPaystackProvider("")
		if _, err := fx.service.UpsertPaymentConfiguration(context.Background(), fx.actor, PaymentConfigurationInput{Provider: "paystack", DisplayName: "Paystack", Enabled: true, SecretSource: "environment"}); err != nil {
			t.Fatal(err)
		}
		tested, err := fx.service.TestPaymentConfiguration(context.Background(), fx.actor, "paystack")
		if err != nil {
			t.Fatal(err)
		}
		if tested.Status != "needs_attention" {
			t.Fatalf("expected blank Paystack provider to need attention, got %+v", tested)
		}
	})
}

func TestPaymentConfigurationStoresMerchantSecretEncryptedAndUsesIt(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	secretStore, err := NewEncryptedPaymentSecretStore(fx.db, []byte("12345678901234567890123456789012"), "test")
	if err != nil {
		t.Fatal(err)
	}
	fx.service.ConfigurePaymentSecretStore(secretStore)

	config, err := fx.service.UpsertPaymentConfiguration(context.Background(), fx.actor, PaymentConfigurationInput{
		Provider:     "paystack",
		DisplayName:  "Paystack",
		Enabled:      true,
		PublicConfig: `{"public_key":"pk_test_public"}`,
		SecretConfig: `{"secret_key":"sk_test_private"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.SecretSource != "merchant_secret" || !config.HasSecret {
		t.Fatalf("expected merchant secret config, got %+v", config)
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "sk_test_private") || strings.Contains(string(body), "secret_config") {
		t.Fatalf("payment config response exposed secret material: %s", string(body))
	}
	var stored PaymentProviderSecret
	if err := fx.db.Where("organization_id = ? AND provider = ? AND secret_name = ?", fx.actor.OrganizationID, "paystack", "secret_key").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Ciphertext, "sk_test_private") {
		t.Fatalf("secret was stored in plaintext: %+v", stored)
	}
	decrypted, err := secretStore.GetSecret(context.Background(), fx.actor.OrganizationID, "paystack", "secret_key")
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "sk_test_private" {
		t.Fatalf("expected decrypted merchant secret, got %q", decrypted)
	}
	configs, err := fx.service.ListPaymentConfigurations(context.Background(), fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 || !configs[0].HasSecret {
		t.Fatalf("expected listed config to show masked secret presence, got %+v", configs)
	}

	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}, IdempotencyKey: "merchant-secret-order"})
	if err != nil {
		t.Fatal(err)
	}
	usedSecret := ""
	fx.service.paystackProviderFactory = func(secret string) PaymentProvider {
		usedSecret = secret
		return namedPaymentProvider{name: "paystack", amountMinor: order.TotalMinor, currency: "NGN"}
	}
	payment, err := fx.service.InitializePayment(context.Background(), fx.actor, PaymentInput{OrderID: order.ID, Provider: "paystack", Email: "customer@example.com", IdempotencyKey: "merchant-secret-pay"})
	if err != nil {
		t.Fatal(err)
	}
	if usedSecret != "sk_test_private" || payment.Provider != "paystack" || payment.AuthorizationURL == "" {
		t.Fatalf("expected merchant secret-backed Paystack initialization, usedSecret=%q payment=%+v", usedSecret, payment)
	}
	tested, err := fx.service.TestPaymentConfiguration(context.Background(), fx.actor, "paystack")
	if err != nil {
		t.Fatal(err)
	}
	if tested.Status != StatusActive {
		t.Fatalf("expected merchant secret Paystack config to test active, got %+v", tested)
	}
}

func TestUpdateChannelMergesSecretsWithoutExposingThem(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	channel, err := fx.service.CreateChannel(context.Background(), fx.actor, ChannelInput{
		Provider:      "whatsapp",
		DisplayName:   "WhatsApp",
		PhoneNumberID: "phone-id",
		DisplayNumber: "+2348012345678",
		Status:        StatusActive,
		Config:        `{"bot_id":"` + uuid.NewString() + `","verify_token":"verify-token"}`,
		SecretConfig:  `{"access_token":"old-token","app_secret":"app-secret"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := fx.service.UpdateChannel(context.Background(), fx.actor, channel.ID, ChannelInput{
		SecretConfig: `{"access_token":"new-token","app_secret":""}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SecretConfig != `{"access_token":"new-token","app_secret":"app-secret"}` && updated.SecretConfig != `{"app_secret":"app-secret","access_token":"new-token"}` {
		t.Fatalf("unexpected merged secret config: %s", updated.SecretConfig)
	}
	body, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "new-token") || strings.Contains(string(body), "app-secret") || strings.Contains(string(body), "secret_config") {
		t.Fatalf("channel response exposed secret material: %s", string(body))
	}
	tested, err := fx.service.TestChannel(context.Background(), fx.actor, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tested["status"] != "ok" {
		t.Fatalf("expected complete channel to test ok, got %+v", tested)
	}
}

func TestUpdateVariantPrice(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	price := int64(500000)
	updated, err := fx.service.UpdateVariant(context.Background(), fx.actor, fx.variant.ID, VariantUpdateInput{PriceMinor: &price})
	if err != nil {
		t.Fatal(err)
	}
	if updated.PriceMinor != 500000 {
		t.Fatalf("expected updated price 500000, got %d", updated.PriceMinor)
	}
	if updated.Name != fx.variant.Name {
		t.Fatalf("expected variant name to stay %q, got %q", fx.variant.Name, updated.Name)
	}
}

func TestSkipUndeliverableNotificationsIsTenantScoped(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	channelID := uuid.New()
	otherOrgID := uuid.New()
	notifications := []CommerceNotification{
		{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, NotificationType: "seed_one", Recipient: "2348000000000", Status: "queued", Payload: "{}"},
		{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: &channelID, NotificationType: "seed_two", Recipient: "", Status: "failed", Payload: "{}"},
		{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: &channelID, NotificationType: "real", Recipient: "2348000000001", Status: "queued", Payload: "{}"},
		{ID: uuid.New(), OrganizationID: otherOrgID, NotificationType: "other_tenant", Recipient: "2348000000002", Status: "queued", Payload: "{}"},
	}
	if err := fx.db.Create(&notifications).Error; err != nil {
		t.Fatal(err)
	}
	count, err := fx.service.SkipUndeliverableNotifications(context.Background(), fx.actor.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two undeliverable notifications to be skipped, got %d", count)
	}
	var stored []CommerceNotification
	if err := fx.db.Where("id IN ?", []uuid.UUID{notifications[0].ID, notifications[1].ID, notifications[2].ID, notifications[3].ID}).Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	statuses := map[uuid.UUID]string{}
	for _, notification := range stored {
		statuses[notification.ID] = notification.Status
	}
	if statuses[notifications[0].ID] != "skipped" || statuses[notifications[1].ID] != "skipped" {
		t.Fatalf("expected only undeliverable tenant notifications skipped, got %+v", statuses)
	}
	if statuses[notifications[2].ID] != "queued" || statuses[notifications[3].ID] != "queued" {
		t.Fatalf("deliverable or cross-tenant notification was changed: %+v", statuses)
	}
}

type namedPaymentProvider struct {
	name        string
	paid        bool
	amountMinor int64
	currency    string
	status      string
}

func (p namedPaymentProvider) Name() string { return p.name }

func (p namedPaymentProvider) Initialize(_ context.Context, req PaymentInitializeRequest) (PaymentInitializeResponse, error) {
	return PaymentInitializeResponse{Reference: req.Reference, AuthorizationURL: "https://pay.example/" + req.Reference, ProviderMetadata: "{}"}, nil
}

func (p namedPaymentProvider) Verify(_ context.Context, reference string) (PaymentVerification, error) {
	paid := p.paid
	if p.status == "" && !p.paid {
		paid = true
	}
	return PaymentVerification{Reference: reference, Paid: paid, Status: defaultString(p.status, boolPaymentStatus(paid)), AmountMinor: p.amountMinor, Currency: p.currency, ProviderMetadata: "{}"}, nil
}

func paystackTestSignature(secret string, body []byte) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
