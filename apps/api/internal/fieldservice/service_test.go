package fieldservice

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type captureBridge struct {
	messages []string
}

func (c *captureBridge) SendText(_ context.Context, _ uuid.UUID, recipient, text string) error {
	c.messages = append(c.messages, recipient+": "+text)
	return nil
}

func (c *captureBridge) AssignHandoff(_ context.Context, _, _, _, _, _ uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

type fieldFixture struct {
	db       *gorm.DB
	field    *Service
	commerce *core.Service
	actor    auth.CurrentUser
	bridge   *captureBridge
	plumber  Pool
	painter  Pool
	john     Provider
	far      Provider
	customer core.Customer
}

func newFieldFixture(t *testing.T) fieldFixture {
	t.Helper()
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
	); err != nil {
		t.Fatal(err)
	}
	orgID, userID := uuid.New(), uuid.New()
	if err := db.Create(&organization.Organization{ID: orgID, Name: "Lagos Home Services", Slug: "lhs-" + orgID.String()[:8], Currency: "NGN", Status: "active", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	actor := auth.CurrentUser{ID: userID, OrganizationID: orgID, Role: authz.MerchantAdmin}
	commerce := core.NewService(db, core.SafeTestProvider{})
	field := NewService(db, commerce, nil)
	bridge := &captureBridge{}
	field.ConfigureDispatcher(bridge)
	commerce.ConfigureAfterPaymentPaid(field.OnPaymentPaid)

	store, err := commerce.CreateStore(context.Background(), actor, core.StoreInput{Name: "Ops", Code: "OPS", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = store
	customer, err := commerce.CreateCustomer(context.Background(), actor, core.CustomerInput{Name: "Amaka", Phone: "+2348011111111"})
	if err != nil {
		t.Fatal(err)
	}
	plumber, err := field.UpsertPool(context.Background(), actor, Pool{Name: "Plumber", Slug: "plumber"})
	if err != nil {
		t.Fatal(err)
	}
	painter, err := field.UpsertPool(context.Background(), actor, Pool{Name: "Painter", Slug: "painter"})
	if err != nil {
		t.Fatal(err)
	}
	lekkiLat, lekkiLng := 6.4474, 3.4723
	ikejaLat, ikejaLng := 6.6018, 3.3515
	john, err := field.UpsertProvider(context.Background(), actor, Provider{Name: "John Adeyemi", PublicCode: "john-lekki", Area: "Lekki", Latitude: &lekkiLat, Longitude: &lekkiLng, RatingAverage: 4.8, JobsCompleted: 40, Availability: AvailabilityAvailable, Status: "active", Phone: "john@lagoshome.demo"}, []uuid.UUID{plumber.ID}, "HomeServices1!")
	if err != nil {
		t.Fatal(err)
	}
	far, err := field.UpsertProvider(context.Background(), actor, Provider{Name: "Ibrahim Musa", PublicCode: "ibrahim-ikeja", Area: "Ikeja", Latitude: &ikejaLat, Longitude: &ikejaLng, RatingAverage: 4.3, JobsCompleted: 18, Availability: AvailabilityAvailable, Status: "active", Phone: "ibrahim@lagoshome.demo"}, []uuid.UUID{plumber.ID}, "HomeServices1!")
	if err != nil {
		t.Fatal(err)
	}
	_, err = field.UpsertProvider(context.Background(), actor, Provider{Name: "Chidi Painter", PublicCode: "chidi-paint", Area: "Lekki", Latitude: &lekkiLat, Longitude: &lekkiLng, RatingAverage: 5, JobsCompleted: 80, Availability: AvailabilityAvailable, Status: "active"}, []uuid.UUID{painter.ID}, "HomeServices1!")
	if err != nil {
		t.Fatal(err)
	}
	return fieldFixture{db: db, field: field, commerce: commerce, actor: actor, bridge: bridge, plumber: plumber, painter: painter, john: john, far: far, customer: customer}
}

func TestEndToEndServiceBooking(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, CustomerName: "Amaka", CustomerPhone: "+2348011111111",
		Area: "Lekki Phase 1", Address: "Admiralty Way", Description: "Leaking kitchen pipe", PreferredAt: "today",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, payment, err := fx.field.InitializeBookingFee(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payment == nil || payment.AuthorizationURL == "" {
		t.Fatal("expected booking fee payment link")
	}
	if request.Status != RequestAwaitingPayment {
		t.Fatalf("status %s", request.Status)
	}
	if err := fx.field.ConfirmBookingFromReference(ctx, fx.actor, payment.Reference); err != nil {
		t.Fatal(err)
	}
	matches, err := fx.field.ListMatches(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 || matches[0].ProviderID != fx.john.ID {
		t.Fatalf("expected John first, got %+v", matches)
	}
	var attempt DispatchAttempt
	if err := fx.db.Where("request_id = ? AND status = ?", request.ID, DispatchNotified).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	johnActor := auth.CurrentUser{ID: *fx.john.UserID, OrganizationID: fx.actor.OrganizationID, Role: authz.ServiceProvider}
	if _, err := fx.field.AcceptDispatch(ctx, johnActor, attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.field.PostMessage(ctx, johnActor, request.ID, "I'll be there in 30 minutes."); err != nil {
		t.Fatal(err)
	}
	quote, err := fx.field.CreateQuote(ctx, johnActor, request.ID, QuoteInput{LabourMinor: 2000000, MaterialsMinor: 1250000, Notes: "Includes parts"})
	if err != nil {
		t.Fatal(err)
	}
	pay, err := fx.field.ApproveQuote(ctx, fx.actor, quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.VerifyPayment(ctx, fx.actor, core.PaymentVerifyInput{Reference: pay.Reference}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.field.TransitionJob(ctx, johnActor, request.ID, RequestCompleted, ""); err != nil {
		t.Fatal(err)
	}
	if err := fx.field.SubmitRating(ctx, fx.actor, request.ID, 5, "Excellent"); err != nil {
		t.Fatal(err)
	}
	updated, err := fx.field.GetRequest(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != RequestCompleted {
		t.Fatalf("expected completed, got %s", updated.Status)
	}
	john, _ := fx.field.getProvider(ctx, fx.actor.OrganizationID, fx.john.ID)
	if john.RatingAverage < 4 {
		t.Fatalf("rating not stored: %v", john.RatingAverage)
	}
	joined := strings.Join(fx.bridge.messages, "\n")
	if !strings.Contains(joined, "New service request") || !strings.Contains(joined, "John") {
		t.Fatalf("expected provider/customer notifications, got %s", joined)
	}
}

func TestOwnerCanEditDeactivateAndReactivateServicePool(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()

	updated, err := fx.field.UpsertPool(ctx, fx.actor, Pool{
		ID: fx.painter.ID, Name: "Decorative Painter", Slug: fx.painter.Slug,
		Status: "inactive", SortOrder: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Decorative Painter" || updated.Status != "inactive" || updated.SortOrder != 9 {
		t.Fatalf("pool update was not persisted: %+v", updated)
	}
	active, err := fx.field.ListPools(ctx, fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, pool := range active {
		if pool.ID == updated.ID {
			t.Fatal("inactive service leaked into the customer-facing pool list")
		}
	}
	manageable, err := fx.field.ListManageablePools(ctx, fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pool := range manageable {
		found = found || pool.ID == updated.ID
	}
	if !found {
		t.Fatal("owner cannot find the inactive service to reactivate it")
	}
	updated.Status = "active"
	if _, err := fx.field.UpsertPool(ctx, fx.actor, updated); err != nil {
		t.Fatal(err)
	}
}

func TestProviderProfileUsesOwnerSuppliedLoginAndDeactivationDisablesAccount(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	provider, err := fx.field.UpsertProvider(ctx, fx.actor, Provider{
		Name: "Acceptance Handyman", Email: "acceptance.handyman@example.com", Phone: "+2348000000042",
		Area: "Ikeja", Availability: AvailabilityAvailable, Status: "active", RatingAverage: 4.2,
	}, []uuid.UUID{fx.plumber.ID}, "StrongPilotPassword1!")
	if err != nil {
		t.Fatal(err)
	}
	if provider.UserID == nil || provider.RatingAverage != 4.2 {
		t.Fatalf("provider login or initial rating was not saved: %+v", provider)
	}
	var user organization.User
	if err := fx.db.Where("id = ?", *provider.UserID).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Email != "acceptance.handyman@example.com" || user.Status != "active" {
		t.Fatalf("unexpected provider user: %+v", user)
	}
	provider.Email = "acceptance.updated@example.com"
	provider.Status = "inactive"
	provider.RatingAverage = 4.5
	updated, err := fx.field.UpsertProvider(ctx, fx.actor, provider, []uuid.UUID{fx.plumber.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "inactive" || updated.RatingAverage != 4.5 {
		t.Fatalf("provider profile update failed: %+v", updated)
	}
	if err := fx.db.Where("id = ?", *provider.UserID).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Email != "acceptance.updated@example.com" || user.Status != "disabled" {
		t.Fatalf("provider login was not synchronized: %+v", user)
	}
	var membership organization.OrganizationMembership
	if err := fx.db.Where("organization_id = ? AND user_id = ?", fx.actor.OrganizationID, *provider.UserID).First(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if membership.Status != "disabled" {
		t.Fatalf("provider membership should be disabled, got %s", membership.Status)
	}
}

func TestRuntimeCancelRequestCancelsPendingBookingAndIsIdempotent(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	sessionID := uuid.New()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, SessionID: &sessionID,
		CustomerName: "Amaka", CustomerPhone: "+2348011111111", Area: "Lekki", Address: "Admiralty Way", Description: "Leaking tap",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, payment, err := fx.field.InitializeBookingFee(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payment == nil || request.BookingOrderID == nil {
		t.Fatal("expected a pending booking order and payment")
	}

	message, err := fx.field.RuntimeCancelRequest(ctx, fx.actor.OrganizationID, fx.customer.ID, sessionID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, request.PublicCode) {
		t.Fatalf("expected cancellation confirmation with request code, got %q", message)
	}
	cancelled, err := fx.field.GetRequest(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != RequestCancelled {
		t.Fatalf("expected cancelled request, got %s", cancelled.Status)
	}
	order, err := fx.commerce.GetOrder(ctx, fx.actor, *request.BookingOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != core.OrderCancelled {
		t.Fatalf("expected cancelled booking order, got %s", order.Status)
	}
	storedPayment, err := fx.commerce.GetPaymentByReference(ctx, fx.actor, payment.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if storedPayment.Status != core.PaymentExpired {
		t.Fatalf("expected expired payment, got %s", storedPayment.Status)
	}

	message, err = fx.field.RuntimeCancelRequest(ctx, fx.actor.OrganizationID, fx.customer.ID, sessionID, request.ID)
	if err != nil || !strings.Contains(message, "already cancelled") {
		t.Fatalf("expected idempotent cancellation, got message=%q err=%v", message, err)
	}
}

func TestRuntimeCancelRequestIsCustomerAndConversationScoped(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	sessionID := uuid.New()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, SessionID: &sessionID, Area: "Lekki", Description: "Leaking tap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.field.RuntimeCancelRequest(ctx, fx.actor.OrganizationID, uuid.New(), sessionID, request.ID); err == nil {
		t.Fatal("another customer could cancel the request")
	}
	if _, err := fx.field.RuntimeCancelRequest(ctx, fx.actor.OrganizationID, fx.customer.ID, uuid.New(), request.ID); err == nil {
		t.Fatal("another conversation could cancel the request")
	}
	current, err := fx.field.GetRequest(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != RequestDraft {
		t.Fatalf("unauthorized cancellation changed request status to %s", current.Status)
	}
}

func TestCancelledBookingIsNotRevivedByLatePaymentCallback(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	sessionID := uuid.New()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, SessionID: &sessionID, Area: "Lekki", Description: "Leaking tap",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, payment, err := fx.field.InitializeBookingFee(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.field.RuntimeCancelRequest(ctx, fx.actor.OrganizationID, fx.customer.ID, sessionID, request.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.VerifyPayment(ctx, fx.actor, core.PaymentVerifyInput{Reference: payment.Reference}); err != nil {
		t.Fatal(err)
	}
	current, err := fx.field.GetRequest(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != RequestCancelled {
		t.Fatalf("late payment callback revived request as %s", current.Status)
	}
	order, err := fx.commerce.GetOrder(ctx, fx.actor, *request.BookingOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != core.OrderCancelled {
		t.Fatalf("late payment callback revived order as %s", order.Status)
	}
}

func TestProviderDeclineMovesToNext(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Area: "Lekki", Description: "Pipe burst", CustomerPhone: "0801"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.field.StartMatching(ctx, fx.actor, request.ID); err != nil {
		t.Fatal(err)
	}
	var first DispatchAttempt
	if err := fx.db.Where("request_id = ?", request.ID).Order("created_at ASC").First(&first).Error; err != nil {
		t.Fatal(err)
	}
	johnActor := auth.CurrentUser{ID: *fx.john.UserID, OrganizationID: fx.actor.OrganizationID, Role: authz.ServiceProvider}
	if first.ProviderID == fx.john.ID {
		if err := fx.field.DeclineDispatch(ctx, johnActor, first.ID); err != nil {
			t.Fatal(err)
		}
	}
	var attempts []DispatchAttempt
	_ = fx.db.Where("request_id = ?", request.ID).Order("created_at ASC").Find(&attempts)
	if len(attempts) < 2 {
		t.Fatalf("expected fallback dispatch, got %+v", attempts)
	}
}

func TestDispatchTimeoutAdvances(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, _ := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Area: "Lekki", Description: "Tap", CustomerPhone: "0801"})
	_ = fx.field.StartMatching(ctx, fx.actor, request.ID)
	var attempt DispatchAttempt
	if err := fx.db.Where("request_id = ?", request.ID).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.field.ProcessDispatchTimeout(ctx, jobs.Job{Payload: `{"attempt_id":"` + attempt.ID.String() + `"}`}); err != nil {
		t.Fatal(err)
	}
	var next DispatchAttempt
	if err := fx.db.Where("request_id = ? AND status = ?", request.ID, DispatchNotified).Order("created_at DESC").First(&next).Error; err != nil {
		t.Fatal(err)
	}
	if next.ID == attempt.ID {
		t.Fatal("timeout did not advance")
	}
}

func TestTenantIsolation(t *testing.T) {
	fx := newFieldFixture(t)
	otherOrg := uuid.New()
	_ = fx.db.Create(&organization.Organization{ID: otherOrg, Name: "Other", Slug: "other-" + otherOrg.String()[:8], Status: "active", Metadata: "{}"}).Error
	other := auth.CurrentUser{ID: uuid.New(), OrganizationID: otherOrg, Role: authz.MerchantAdmin}
	if _, err := fx.field.ListPools(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	pools, _ := fx.field.ListPools(context.Background(), other)
	if len(pools) != 0 {
		t.Fatalf("leaked pools: %+v", pools)
	}
}

func TestIdempotentMatching(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, _ := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Area: "Lekki", Description: "Leak"})
	if err := fx.field.StartMatching(ctx, fx.actor, request.ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.field.StartMatching(ctx, fx.actor, request.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	fx.db.Model(&Match{}).Where("request_id = ?", request.ID).Count(&count)
	if count < 1 {
		t.Fatal("expected matches")
	}
}
