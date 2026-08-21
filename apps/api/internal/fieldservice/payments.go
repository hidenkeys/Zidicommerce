package fieldservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

func (s *Service) createPaymentOrder(ctx context.Context, actor auth.CurrentUser, request Request, amountMinor int64, currency, purpose string, relatedID uuid.UUID) (core.Order, core.Payment, error) {
	if s.commerce == nil {
		return core.Order{}, core.Payment{}, httperror.Internal("Payments are not configured")
	}
	if amountMinor <= 0 {
		return core.Order{}, core.Payment{}, httperror.BadRequest("Payment amount must be greater than zero")
	}
	storeID, variantID, err := s.ensureFeeCatalog(ctx, actor, currency)
	if err != nil {
		return core.Order{}, core.Payment{}, err
	}
	// The line price is passed explicitly below. Rewriting the shared variant
	// price per payment would race between a booking fee and a quote settled at
	// the same moment.
	metadata, _ := json.Marshal(map[string]any{
		"service_request_id":         request.ID.String(),
		"service_payment_purpose":    purpose,
		"service_quote_id":           relatedID.String(),
		"hide_from_commerce_default": true,
	})
	order, err := s.commerce.CreateOrder(ctx, actor, core.OrderInput{
		StoreID:        storeID,
		CustomerID:     request.CustomerID,
		FulfilmentType: core.FulfilmentPickup,
		Currency:       currency,
		Metadata:       string(metadata),
		IdempotencyKey: fmt.Sprintf("svc:%s:%s:%s", purpose, request.ID, relatedID),
		Items:          []core.OrderItemInput{{VariantID: variantID, Quantity: 1, UnitPriceMinor: &amountMinor}},
	})
	if err != nil {
		return core.Order{}, core.Payment{}, err
	}
	email := request.CustomerPhone + "@customers.zidicommerce.local"
	if strings.Contains(request.CustomerPhone, "@") {
		email = request.CustomerPhone
	}
	payment, err := s.commerce.InitializePayment(ctx, actor, core.PaymentInput{
		OrderID:        order.ID,
		Email:          email,
		IdempotencyKey: "pay:" + order.ID.String(),
	})
	return order, payment, err
}

func (s *Service) ensureFeeCatalog(ctx context.Context, actor auth.CurrentUser, currency string) (uuid.UUID, uuid.UUID, error) {
	stores, err := s.commerce.ListStores(ctx, actor)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	var store core.Store
	if len(stores) == 0 {
		store, err = s.commerce.CreateStore(ctx, actor, core.StoreInput{Name: "Field operations", Code: "FIELD", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	} else {
		store = stores[0]
	}
	products, err := s.commerce.ListProducts(ctx, actor)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	var product core.Product
	for _, candidate := range products {
		if strings.EqualFold(candidate.Slug, "service-booking-fee") {
			product = candidate
			break
		}
	}
	if product.ID == uuid.Nil {
		product, err = s.commerce.CreateProduct(ctx, actor, core.ProductInput{
			Name:     "Service booking fee",
			Slug:     "service-booking-fee",
			Status:   core.StatusActive,
			Variants: []core.VariantInput{{SKU: "SVC-FEE", Name: "Booking fee", PriceMinor: 500000, Currency: currency, Status: core.StatusActive}},
		})
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	}
	if len(product.Variants) == 0 {
		return uuid.Nil, uuid.Nil, httperror.Internal("Booking fee product is missing a price")
	}
	variant := product.Variants[0]
	_, err = s.commerce.UpsertInventory(ctx, actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: variant.ID, OnHand: 1000000})
	return store.ID, variant.ID, err
}

func (s *Service) ConfirmBookingFromReference(ctx context.Context, actor auth.CurrentUser, reference string) error {
	if s.commerce == nil || strings.TrimSpace(reference) == "" {
		return httperror.BadRequest("Payment reference is required")
	}
	_, err := s.commerce.VerifyPayment(ctx, actor, core.PaymentVerifyInput{Reference: reference})
	return err
}

func (s *Service) Overview(ctx context.Context, actor auth.CurrentUser) (map[string]any, error) {
	if err := s.requireOwner(actor); err != nil {
		return nil, err
	}
	count := func(model any, statuses ...string) int64 {
		var total int64
		query := s.db.WithContext(ctx).Model(model).Where("organization_id = ?", actor.OrganizationID)
		if len(statuses) > 0 {
			query = query.Where("status IN ?", statuses)
		}
		_ = query.Count(&total)
		return total
	}
	var available int64
	_ = s.db.WithContext(ctx).Model(&Provider{}).Where("organization_id = ? AND status = ? AND availability = ?", actor.OrganizationID, "active", AvailabilityAvailable).Count(&available)
	var bookingFees int64
	_ = s.db.WithContext(ctx).Model(&Request{}).Where("organization_id = ? AND status NOT IN ?", actor.OrganizationID, []string{RequestDraft, RequestAwaitingPayment, RequestCancelled}).Select("COALESCE(SUM(booking_fee_minor),0)").Scan(&bookingFees)
	var quoteRevenue int64
	_ = s.db.WithContext(ctx).Model(&Quote{}).Where("organization_id = ? AND status = ?", actor.OrganizationID, "paid").Select("COALESCE(SUM(total_minor),0)").Scan(&quoteRevenue)
	var todays int64
	_ = s.db.WithContext(ctx).Model(&Request{}).Where("organization_id = ? AND created_at >= ?", actor.OrganizationID, s.now().Truncate(24*time.Hour)).Count(&todays)
	return map[string]any{
		"todays_requests":           todays,
		"active_jobs":               count(&Request{}, RequestAssigned, RequestOnTheWay, RequestArrived, RequestInProgress, RequestQuoteSent, RequestQuoteApproved, RequestPaymentConfirmed),
		"available_providers":       available,
		"pending_provider_requests": count(&DispatchAttempt{}, DispatchNotified),
		"completed_jobs":            count(&Request{}, RequestCompleted),
		"outstanding_quotes":        count(&Quote{}, QuoteSent),
		"booking_fees_minor":        bookingFees,
		"revenue_minor":             quoteRevenue + bookingFees,
	}, nil
}

func (s *Service) ListTransactions(ctx context.Context, actor auth.CurrentUser) ([]map[string]any, error) {
	if err := s.requireOwner(actor); err != nil {
		return nil, err
	}
	var quotes []Quote
	_ = s.db.WithContext(ctx).Where("organization_id = ? AND status IN ?", actor.OrganizationID, []string{QuoteSent, QuoteApproved, QuotePaid, QuoteDeclined}).Order("created_at DESC").Limit(100).Find(&quotes)
	var requests []Request
	_ = s.db.WithContext(ctx).Where("organization_id = ? AND booking_order_id IS NOT NULL", actor.OrganizationID).Order("created_at DESC").Limit(100).Find(&requests)
	out := make([]map[string]any, 0, len(quotes)+len(requests))
	for _, quote := range quotes {
		out = append(out, map[string]any{
			"kind": "quote", "code": quote.PublicCode, "amount_minor": quote.TotalMinor, "currency": quote.Currency, "status": quote.Status, "created_at": quote.CreatedAt,
		})
	}
	for _, request := range requests {
		status := "booking_fee"
		if request.Status != RequestAwaitingPayment && request.Status != RequestDraft {
			status = "booking_fee_paid"
		}
		out = append(out, map[string]any{
			"kind": "booking_fee", "code": request.PublicCode, "amount_minor": request.BookingFeeMinor, "currency": "NGN", "status": status, "created_at": request.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) ListActiveConversations(ctx context.Context, actor auth.CurrentUser) ([]Request, error) {
	if err := s.requireOwner(actor); err != nil {
		return nil, err
	}
	var requests []Request
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND status IN ?", actor.OrganizationID, []string{
			RequestAssigned, RequestOnTheWay, RequestArrived, RequestInProgress, RequestQuoteSent, RequestQuoteApproved, RequestPaymentConfirmed,
		}).
		Preload("Pool").Preload("AssignedProvider").
		Order("updated_at DESC").
		Find(&requests).Error
	return requests, err
}

func (s *Service) ProviderHome(ctx context.Context, actor auth.CurrentUser) (map[string]any, error) {
	provider, err := s.providerForUser(ctx, actor)
	if err != nil {
		return nil, err
	}
	inbox, _ := s.ListProviderInbox(ctx, actor)
	var current Assignment
	_ = s.db.WithContext(ctx).Where("organization_id = ? AND provider_id = ? AND status NOT IN ?", actor.OrganizationID, provider.ID, []string{RequestCompleted, RequestCancelled}).Preload("Request").Preload("Provider").Order("updated_at DESC").First(&current)
	var completedToday int64
	_ = s.db.WithContext(ctx).Model(&Request{}).Where("organization_id = ? AND assigned_provider_id = ? AND status = ? AND updated_at >= ?", actor.OrganizationID, provider.ID, RequestCompleted, s.now().Truncate(24*time.Hour)).Count(&completedToday)
	return map[string]any{
		"provider":         provider,
		"availability":     provider.Availability,
		"current_job":      current,
		"pending_requests": inbox,
		"completed_today":  completedToday,
		"jobs_completed":   provider.JobsCompleted,
		"rating":           provider.RatingAverage,
		"earnings_minor":   s.providerEarnings(ctx, actor.OrganizationID, provider.ID),
	}, nil
}

func (s *Service) ListMessages(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) ([]Message, error) {
	if _, err := s.GetRequest(ctx, actor, requestID); err != nil {
		return nil, err
	}
	var messages []Message
	err := s.db.WithContext(ctx).Where("organization_id = ? AND request_id = ?", actor.OrganizationID, requestID).Order("created_at ASC").Find(&messages).Error
	return messages, err
}

func (s *Service) PostMessage(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID, body string) (Message, error) {
	request, err := s.GetRequest(ctx, actor, requestID)
	if err != nil {
		return Message{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Message{}, httperror.BadRequest("Message is required")
	}
	authorType := "owner"
	if actor.Role == authz.ServiceProvider {
		authorType = "provider"
		if _, err := s.assignedActor(ctx, actor, requestID); err != nil {
			return Message{}, err
		}
	}
	msg := Message{ID: uuid.New(), OrganizationID: actor.OrganizationID, RequestID: requestID, AuthorType: authorType, AuthorUserID: &actor.ID, Body: body, Metadata: "{}"}
	if err := s.db.WithContext(ctx).Create(&msg).Error; err != nil {
		return Message{}, err
	}
	_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, body)
	return msg, nil
}

func (s *Service) TakeOverConversation(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) error {
	if err := s.requireOwner(actor); err != nil {
		return err
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return err
	}
	s.audit(ctx, actor, "service_request", request.ID, "admin_intervention", `{"action":"take_over"}`)
	_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, "A coordinator from "+s.companyName(ctx, actor.OrganizationID)+" has joined this conversation.")
	return nil
}

func (s *Service) ReleaseToProvider(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) error {
	if err := s.requireOwner(actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "service_request", requestID, "admin_intervention", `{"action":"release_to_provider"}`)
	return nil
}

// CloseConversation ends an active thread. A job that was never finished is
// cancelled rather than completed, so the completed-jobs figure and the
// provider's job count stay honest.
func (s *Service) CloseConversation(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) error {
	if err := s.requireOwner(actor); err != nil {
		return err
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return err
	}
	s.audit(ctx, actor, "service_request", requestID, "admin_intervention", `{"action":"close"}`)
	if request.Status == RequestCompleted || request.Status == RequestCancelled {
		return nil
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&Request{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, requestID).Updates(map[string]any{"status": RequestCancelled, "updated_at": now}).Error; err != nil {
		return err
	}
	_ = s.db.WithContext(ctx).Model(&Assignment{}).Where("organization_id = ? AND request_id = ?", actor.OrganizationID, requestID).Updates(map[string]any{"status": RequestCancelled, "updated_at": now}).Error
	if request.AssignedProviderID != nil {
		_ = s.db.WithContext(ctx).Model(&Provider{}).Where("id = ?", *request.AssignedProviderID).Updates(map[string]any{"availability": AvailabilityAvailable, "current_job_status": "idle", "updated_at": now}).Error
	}
	_ = s.db.WithContext(ctx).Model(&DispatchAttempt{}).Where("request_id = ? AND status = ?", requestID, DispatchNotified).Updates(map[string]any{"status": DispatchCancelled, "updated_at": now}).Error
	return nil
}

// SubmitRating records the customer's rating of the provider. It is reachable
// from the WhatsApp handoff and the owner dashboard; a provider can never rate
// themselves, and only a finished job can be rated.
func (s *Service) SubmitRating(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID, score int, feedback string) error {
	if err := s.requireCustomerDecision(actor); err != nil {
		return err
	}
	if score < 1 || score > 5 {
		return httperror.BadRequest("Rating must be between 1 and 5")
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return err
	}
	if request.AssignedProviderID == nil {
		return httperror.BadRequest("This job has no assigned professional yet")
	}
	if request.Status != RequestCompleted {
		return httperror.BadRequest("You can rate this job once it is completed")
	}
	rating := Rating{ID: uuid.New(), OrganizationID: actor.OrganizationID, RequestID: request.ID, ProviderID: *request.AssignedProviderID, CustomerID: request.CustomerID, Score: score, Feedback: strings.TrimSpace(feedback)}
	if err := s.db.WithContext(ctx).Where("request_id = ?", request.ID).Assign(rating).FirstOrCreate(&rating).Error; err != nil {
		if err := s.db.WithContext(ctx).Create(&rating).Error; err != nil {
			return err
		}
	}
	var avg float64
	_ = s.db.WithContext(ctx).Model(&Rating{}).Where("provider_id = ?", *request.AssignedProviderID).Select("AVG(score)").Scan(&avg)
	_ = s.db.WithContext(ctx).Model(&Provider{}).Where("id = ?", *request.AssignedProviderID).Update("rating_average", avg).Error
	s.audit(ctx, actor, "service_request", request.ID, "service_rated", fmt.Sprintf(`{"rating":%d}`, score))
	return nil
}

// providerEarnings totals the quotes a provider has actually been paid for.
func (s *Service) providerEarnings(ctx context.Context, organizationID, providerID uuid.UUID) int64 {
	var total int64
	_ = s.db.WithContext(ctx).Model(&Quote{}).
		Where("organization_id = ? AND provider_id = ? AND status = ?", organizationID, providerID, QuotePaid).
		Select("COALESCE(SUM(total_minor),0)").Scan(&total)
	return total
}

// ProviderView is a provider plus the operational figures the owner dashboard
// shows next to them, so the UI does not have to stitch several calls together.
type ProviderView struct {
	Provider
	EarningsMinor int64 `json:"earnings_minor"`
	ActiveJobs    int   `json:"active_jobs"`
	OpenRequests  int   `json:"open_requests"`
}

// ListProviderViews returns providers with earnings and current workload.
// Providers themselves see only their own row, matching ListProviders.
func (s *Service) ListProviderViews(ctx context.Context, actor auth.CurrentUser) ([]ProviderView, error) {
	providers, err := s.ListProviders(ctx, actor)
	if err != nil {
		return nil, err
	}
	earnings := map[string]int64{}
	if actor.Role.CanManageOrganization() {
		earnings, err = s.ProviderEarnings(ctx, actor)
		if err != nil {
			return nil, err
		}
	}
	views := make([]ProviderView, 0, len(providers))
	for _, provider := range providers {
		var active, open int64
		_ = s.db.WithContext(ctx).Model(&Assignment{}).
			Where("organization_id = ? AND provider_id = ? AND status NOT IN ?", actor.OrganizationID, provider.ID, []string{RequestCompleted, RequestCancelled}).
			Count(&active)
		_ = s.db.WithContext(ctx).Model(&DispatchAttempt{}).
			Where("organization_id = ? AND provider_id = ? AND status = ?", actor.OrganizationID, provider.ID, DispatchNotified).
			Count(&open)
		earned, ok := earnings[provider.ID.String()]
		if !ok && actor.Role.IsServiceProvider() {
			earned = s.providerEarnings(ctx, actor.OrganizationID, provider.ID)
		}
		views = append(views, ProviderView{Provider: provider, EarningsMinor: earned, ActiveJobs: int(active), OpenRequests: int(open)})
	}
	return views, nil
}

// ProviderEarnings exposes per-provider paid-quote totals to the owner
// dashboard, keyed by provider id.
func (s *Service) ProviderEarnings(ctx context.Context, actor auth.CurrentUser) (map[string]int64, error) {
	if err := s.requireOwner(actor); err != nil {
		return nil, err
	}
	var rows []struct {
		ProviderID uuid.UUID
		Total      int64
	}
	if err := s.db.WithContext(ctx).Model(&Quote{}).
		Where("organization_id = ? AND status = ?", actor.OrganizationID, QuotePaid).
		Select("provider_id, COALESCE(SUM(total_minor),0) AS total").
		Group("provider_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.ProviderID.String()] = row.Total
	}
	return out, nil
}
