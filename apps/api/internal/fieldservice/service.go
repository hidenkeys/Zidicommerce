package fieldservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Dispatcher interface {
	SendText(ctx context.Context, organizationID uuid.UUID, recipient, text string) error
	AssignHandoff(ctx context.Context, organizationID, sessionID, customerID, assignedUserID, requestID uuid.UUID) (uuid.UUID, error)
}

type Service struct {
	db         *gorm.DB
	commerce   *core.Service
	jobs       *jobs.Service
	dispatcher Dispatcher
	log        *slog.Logger
	now        func() time.Time
}

func NewService(db *gorm.DB, commerce *core.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, commerce: commerce, log: logger, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ConfigureJobs(jobService *jobs.Service)    { s.jobs = jobService }
func (s *Service) ConfigureDispatcher(dispatcher Dispatcher) { s.dispatcher = dispatcher }

func (s *Service) actorOrg(actor auth.CurrentUser) error {
	if actor.OrganizationID == uuid.Nil {
		return httperror.Unauthorized("Organization context is required")
	}
	return nil
}

func (s *Service) requireOwner(actor auth.CurrentUser) error {
	if err := s.actorOrg(actor); err != nil {
		return err
	}
	if !actor.Role.CanManageOrganization() {
		return httperror.Forbidden("You cannot manage this workspace")
	}
	return nil
}

func (s *Service) GetSettings(ctx context.Context, actor auth.CurrentUser) (Settings, error) {
	if err := s.actorOrg(actor); err != nil {
		return Settings{}, err
	}
	return s.ensureSettings(ctx, actor.OrganizationID)
}

func (s *Service) UpdateSettings(ctx context.Context, actor auth.CurrentUser, input Settings) (Settings, error) {
	if err := s.requireOwner(actor); err != nil {
		return Settings{}, err
	}
	current, err := s.ensureSettings(ctx, actor.OrganizationID)
	if err != nil {
		return Settings{}, err
	}
	if input.BookingFeeMinor > 0 {
		current.BookingFeeMinor = input.BookingFeeMinor
	}
	if strings.TrimSpace(input.Currency) != "" {
		current.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	}
	current.RequireBookingFee = input.RequireBookingFee
	current.BookingFeeRefundable = input.BookingFeeRefundable
	if strings.TrimSpace(input.DispatchStrategy) != "" {
		current.DispatchStrategy = strings.ToLower(strings.TrimSpace(input.DispatchStrategy))
	}
	if input.AcceptanceWindowSeconds > 0 {
		current.AcceptanceWindowSeconds = input.AcceptanceWindowSeconds
	}
	if input.MaxDistanceKM > 0 {
		current.MaxDistanceKM = input.MaxDistanceKM
	}
	if input.WeightService+input.WeightAvailability+input.WeightDistance+input.WeightRating+input.WeightExperience > 0 {
		current.WeightService = input.WeightService
		current.WeightAvailability = input.WeightAvailability
		current.WeightDistance = input.WeightDistance
		current.WeightRating = input.WeightRating
		current.WeightExperience = input.WeightExperience
	}
	if input.WelcomeMessage != "" {
		current.WelcomeMessage = input.WelcomeMessage
	}
	if input.CompanyDisplayName != "" {
		current.CompanyDisplayName = input.CompanyDisplayName
	}
	if input.ProviderPortalBaseURL != "" {
		current.ProviderPortalBaseURL = strings.TrimRight(input.ProviderPortalBaseURL, "/")
	}
	current.UpdatedAt = s.now()
	if err := s.db.WithContext(ctx).Save(&current).Error; err != nil {
		return Settings{}, err
	}
	s.audit(ctx, actor, "settings", current.OrganizationID, "service_settings_updated", "{}")
	return current, nil
}

func (s *Service) ensureSettings(ctx context.Context, organizationID uuid.UUID) (Settings, error) {
	var settings Settings
	err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).First(&settings).Error
	if err == nil {
		return settings, nil
	}
	if err != gorm.ErrRecordNotFound {
		return Settings{}, err
	}
	var org organization.Organization
	name := "Home services"
	if s.db.WithContext(ctx).Select("name").Where("id = ?", organizationID).First(&org).Error == nil && org.Name != "" {
		name = org.Name
	}
	settings = Settings{
		OrganizationID:          organizationID,
		BookingFeeMinor:         500000,
		Currency:                "NGN",
		RequireBookingFee:       true,
		DispatchStrategy:        "sequential",
		AcceptanceWindowSeconds: 120,
		MaxDistanceKM:           25,
		WeightService:           0.40,
		WeightAvailability:      0.20,
		WeightDistance:          0.20,
		WeightRating:            0.15,
		WeightExperience:        0.05,
		CompanyDisplayName:      name,
		WelcomeMessage:          fmt.Sprintf("Hi 👋 Welcome to %s.\nWe help you find trusted professionals for home services across Lagos.\n\nWhat service do you need today?", name),
		Metadata:                "{}",
	}
	if err := s.db.WithContext(ctx).Create(&settings).Error; err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s *Service) ListPools(ctx context.Context, actor auth.CurrentUser) ([]Pool, error) {
	if err := s.actorOrg(actor); err != nil {
		return nil, err
	}
	var pools []Pool
	err := s.db.WithContext(ctx).Where("organization_id = ? AND status = ?", actor.OrganizationID, "active").Order("sort_order ASC, name ASC").Find(&pools).Error
	return pools, err
}

// ListManageablePools includes inactive categories so an owner can reactivate
// them. Customer-facing runtime actions continue to use ListPools, which only
// returns active services.
func (s *Service) ListManageablePools(ctx context.Context, actor auth.CurrentUser) ([]Pool, error) {
	if err := s.requireOwner(actor); err != nil {
		return nil, err
	}
	var pools []Pool
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("sort_order ASC, name ASC").Find(&pools).Error
	return pools, err
}

func (s *Service) UpsertPool(ctx context.Context, actor auth.CurrentUser, input Pool) (Pool, error) {
	if err := s.requireOwner(actor); err != nil {
		return Pool{}, err
	}
	input.OrganizationID = actor.OrganizationID
	input.Slug = slugify(input.Slug, input.Name)
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return Pool{}, httperror.BadRequest("Service name is required")
	}
	if input.Status == "" {
		input.Status = "active"
	}
	input.Metadata = defaultJSON(input.Metadata)
	if input.ID == uuid.Nil {
		input.ID = uuid.New()
		if err := s.db.WithContext(ctx).Create(&input).Error; err != nil {
			return Pool{}, err
		}
		s.audit(ctx, actor, "service_pool", input.ID, "service_pool_created", fmt.Sprintf(`{"name":%q}`, input.Name))
		return input, nil
	}
	if err := s.db.WithContext(ctx).Model(&Pool{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, input.ID).Updates(map[string]any{
		"name": input.Name, "slug": input.Slug, "description": input.Description, "status": input.Status, "sort_order": input.SortOrder, "updated_at": s.now(),
	}).Error; err != nil {
		return Pool{}, err
	}
	return s.getPool(ctx, actor.OrganizationID, input.ID)
}

func (s *Service) ListProviders(ctx context.Context, actor auth.CurrentUser) ([]Provider, error) {
	if err := s.actorOrg(actor); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Preload("Pools").Order("name ASC")
	if actor.Role == authz.ServiceProvider {
		query = query.Where("user_id = ?", actor.ID)
	}
	var providers []Provider
	return providers, query.Find(&providers).Error
}

func (s *Service) UpsertProvider(ctx context.Context, actor auth.CurrentUser, input Provider, poolIDs []uuid.UUID, password string) (Provider, error) {
	if err := s.requireOwner(actor); err != nil {
		return Provider{}, err
	}
	input.OrganizationID = actor.OrganizationID
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if input.Name == "" {
		return Provider{}, httperror.BadRequest("Provider name is required")
	}
	if input.PublicCode == "" {
		input.PublicCode = slugify("", input.Name)
	}
	if input.Availability == "" {
		input.Availability = AvailabilityAvailable
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.CurrentJobStatus == "" {
		input.CurrentJobStatus = "idle"
	}
	input.Metadata = defaultJSON(input.Metadata)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.ID == uuid.Nil {
			input.ID = uuid.New()
			if password != "" || input.UserID == nil {
				userID, err := s.ensureProviderUserTx(tx, actor, input, password)
				if err != nil {
					return err
				}
				input.UserID = &userID
			}
			if err := tx.Create(&input).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&Provider{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, input.ID).Updates(map[string]any{
				"name": input.Name, "phone": input.Phone, "whatsapp_number": input.WhatsAppNumber, "area": input.Area, "address": input.Address,
				"latitude": input.Latitude, "longitude": input.Longitude, "availability": input.Availability, "status": input.Status,
				"profile_image_url": input.ProfileImageURL, "rating_average": input.RatingAverage, "updated_at": s.now(),
			}).Error; err != nil {
				return err
			}
			if input.UserID == nil {
				var existing Provider
				if err := tx.Select("user_id").Where("organization_id = ? AND id = ?", actor.OrganizationID, input.ID).First(&existing).Error; err != nil {
					return err
				}
				input.UserID = existing.UserID
			}
			if input.UserID != nil {
				userStatus := "active"
				if input.Status == "inactive" {
					userStatus = "disabled"
				}
				updates := map[string]any{"status": userStatus, "updated_at": s.now()}
				if input.Email != "" {
					updates["email"] = input.Email
				}
				if err := tx.Model(&organization.User{}).Where("id = ? AND organization_id = ?", *input.UserID, actor.OrganizationID).Updates(updates).Error; err != nil {
					return err
				}
				if err := tx.Model(&organization.OrganizationMembership{}).Where("organization_id = ? AND user_id = ?", actor.OrganizationID, *input.UserID).Updates(map[string]any{"status": userStatus, "updated_at": s.now()}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Where("provider_id = ?", input.ID).Delete(&ProviderPool{}).Error; err != nil {
			return err
		}
		for _, poolID := range poolIDs {
			if err := tx.Create(&ProviderPool{OrganizationID: actor.OrganizationID, ProviderID: input.ID, PoolID: poolID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Provider{}, err
	}
	s.audit(ctx, actor, "service_provider", input.ID, "service_provider_upserted", fmt.Sprintf(`{"name":%q}`, input.Name))
	return s.getProvider(ctx, actor.OrganizationID, input.ID)
}

func (s *Service) SetProviderAvailability(ctx context.Context, actor auth.CurrentUser, providerID uuid.UUID, availability string) (Provider, error) {
	if err := s.actorOrg(actor); err != nil {
		return Provider{}, err
	}
	availability = strings.ToLower(strings.TrimSpace(availability))
	if availability != AvailabilityAvailable && availability != AvailabilityBusy && availability != AvailabilityOffline {
		return Provider{}, httperror.BadRequest("Availability must be available, busy, or offline")
	}
	query := s.db.WithContext(ctx).Model(&Provider{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, providerID)
	if actor.Role == authz.ServiceProvider {
		query = query.Where("user_id = ?", actor.ID)
	} else if !actor.Role.CanManageOrganization() {
		return Provider{}, httperror.Forbidden("You cannot update availability")
	}
	if err := query.Updates(map[string]any{"availability": availability, "updated_at": s.now()}).Error; err != nil {
		return Provider{}, err
	}
	return s.getProvider(ctx, actor.OrganizationID, providerID)
}

type CreateRequestInput struct {
	CustomerID    uuid.UUID
	PoolID        uuid.UUID
	SessionID     *uuid.UUID
	ChannelID     *uuid.UUID
	CustomerName  string
	CustomerPhone string
	Area          string
	Address       string
	Latitude      *float64
	Longitude     *float64
	Description   string
	PreferredAt   string
	MediaURL      string
}

func (s *Service) CreateRequest(ctx context.Context, actor auth.CurrentUser, input CreateRequestInput) (Request, error) {
	if err := s.actorOrg(actor); err != nil {
		return Request{}, err
	}
	if input.PoolID == uuid.Nil || input.CustomerID == uuid.Nil {
		return Request{}, httperror.BadRequest("Service and customer are required")
	}
	pool, err := s.getPool(ctx, actor.OrganizationID, input.PoolID)
	if err != nil {
		return Request{}, err
	}
	settings, err := s.ensureSettings(ctx, actor.OrganizationID)
	if err != nil {
		return Request{}, err
	}
	request := Request{
		ID:                    uuid.New(),
		OrganizationID:        actor.OrganizationID,
		PublicCode:            s.nextCode(ctx, actor.OrganizationID, "REQ"),
		CustomerID:            input.CustomerID,
		PoolID:                pool.ID,
		SessionID:             input.SessionID,
		ChannelID:             input.ChannelID,
		CustomerName:          strings.TrimSpace(input.CustomerName),
		CustomerPhone:         strings.TrimSpace(input.CustomerPhone),
		Area:                  strings.TrimSpace(input.Area),
		Address:               strings.TrimSpace(input.Address),
		Latitude:              input.Latitude,
		Longitude:             input.Longitude,
		Description:           strings.TrimSpace(input.Description),
		PreferredAt:           strings.TrimSpace(input.PreferredAt),
		MediaURL:              strings.TrimSpace(input.MediaURL),
		Status:                RequestDraft,
		BookingFeeMinor:       settings.BookingFeeMinor,
		ConversationSessionID: input.SessionID,
		Metadata:              "{}",
	}
	if request.Latitude == nil || request.Longitude == nil {
		if lat, lng, ok := GeocodeLagos(request.Area + " " + request.Address); ok {
			request.Latitude = &lat
			request.Longitude = &lng
		}
	}
	customerUpdates := map[string]any{"updated_at": s.now()}
	if request.CustomerName != "" {
		customerUpdates["name"] = request.CustomerName
	}
	if request.CustomerPhone != "" {
		customerUpdates["phone"] = request.CustomerPhone
	}
	if err := s.db.WithContext(ctx).Model(&core.Customer{}).
		Where("organization_id = ? AND id = ?", actor.OrganizationID, input.CustomerID).
		Updates(customerUpdates).Error; err != nil {
		return Request{}, err
	}
	if err := s.db.WithContext(ctx).Create(&request).Error; err != nil {
		return Request{}, err
	}
	s.audit(ctx, actor, "service_request", request.ID, "service_request_created", fmt.Sprintf(`{"service":%q}`, pool.Name))
	return s.getRequest(ctx, actor.OrganizationID, request.ID)
}

func (s *Service) GetRequest(ctx context.Context, actor auth.CurrentUser, id uuid.UUID) (Request, error) {
	if err := s.actorOrg(actor); err != nil {
		return Request{}, err
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, id)
	if err != nil {
		return Request{}, err
	}
	if actor.Role == authz.ServiceProvider {
		provider, err := s.providerForUser(ctx, actor)
		if err != nil {
			return Request{}, err
		}
		if request.AssignedProviderID == nil || *request.AssignedProviderID != provider.ID {
			var attempt DispatchAttempt
			if err := s.db.WithContext(ctx).Where("organization_id = ? AND request_id = ? AND provider_id = ? AND status = ?", actor.OrganizationID, id, provider.ID, DispatchNotified).First(&attempt).Error; err != nil {
				return Request{}, httperror.Forbidden("You cannot view this request")
			}
			// A provider who has only been offered the job sees enough to decide,
			// not the customer's identity, phone number or exact address.
			return redactForProvider(request), nil
		}
	}
	return request, nil
}

// redactForProvider strips customer contact details from a request that a
// provider has been offered but has not accepted.
func redactForProvider(request Request) Request {
	request.CustomerName = ""
	request.CustomerPhone = ""
	request.Address = ""
	request.MediaURL = ""
	request.Latitude = nil
	request.Longitude = nil
	return request
}

func (s *Service) ListRequests(ctx context.Context, actor auth.CurrentUser, status string) ([]Request, error) {
	if err := s.actorOrg(actor); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Preload("Pool").Preload("AssignedProvider").Order("created_at DESC").Limit(200)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if actor.Role == authz.ServiceProvider {
		provider, err := s.providerForUser(ctx, actor)
		if err != nil {
			return nil, err
		}
		query = query.Where("assigned_provider_id = ?", provider.ID)
	}
	var requests []Request
	return requests, query.Find(&requests).Error
}

func (s *Service) ListProviderInbox(ctx context.Context, actor auth.CurrentUser) ([]DispatchAttempt, error) {
	if err := s.actorOrg(actor); err != nil {
		return nil, err
	}
	provider, err := s.providerForUser(ctx, actor)
	if err != nil {
		return nil, err
	}
	var attempts []DispatchAttempt
	err = s.db.WithContext(ctx).
		Where("organization_id = ? AND provider_id = ? AND status = ? AND expires_at > ?", actor.OrganizationID, provider.ID, DispatchNotified, s.now()).
		Preload("Provider").
		Preload("Request").
		Preload("Request.Pool").
		Order("created_at DESC").
		Find(&attempts).Error
	for index := range attempts {
		attempts[index].Request = redactForProvider(attempts[index].Request)
	}
	return attempts, err
}

func (s *Service) InitializeBookingFee(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) (Request, *core.Payment, error) {
	request, err := s.GetRequest(ctx, actor, requestID)
	if err != nil {
		return Request{}, nil, err
	}
	settings, err := s.ensureSettings(ctx, actor.OrganizationID)
	if err != nil {
		return Request{}, nil, err
	}
	if !settings.RequireBookingFee {
		if err := s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{"status": RequestMatching, "updated_at": s.now()}).Error; err != nil {
			return Request{}, nil, err
		}
		if err := s.StartMatching(ctx, actor, request.ID); err != nil {
			return Request{}, nil, err
		}
		updated, _ := s.getRequest(ctx, actor.OrganizationID, request.ID)
		return updated, nil, nil
	}
	order, payment, err := s.createPaymentOrder(ctx, actor, request, settings.BookingFeeMinor, settings.Currency, "booking_fee", request.ID)
	if err != nil {
		return Request{}, nil, err
	}
	if err := s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{
		"status": RequestAwaitingPayment, "booking_order_id": order.ID, "booking_fee_minor": settings.BookingFeeMinor, "updated_at": s.now(),
	}).Error; err != nil {
		return Request{}, nil, err
	}
	s.audit(ctx, actor, "service_request", request.ID, "booking_fee_initiated", fmt.Sprintf(`{"amount_minor":%d}`, settings.BookingFeeMinor))
	updated, _ := s.getRequest(ctx, actor.OrganizationID, request.ID)
	return updated, &payment, nil
}

func (s *Service) OnPaymentPaid(ctx context.Context, organizationID, orderID uuid.UUID, metadata string) error {
	meta := parseMap(metadata)
	requestID, err := uuid.Parse(strings.TrimSpace(stringValue(meta["service_request_id"])))
	if err != nil {
		return nil
	}
	purpose := stringValue(meta["service_payment_purpose"])
	actor := auth.CurrentUser{ID: uuid.Nil, OrganizationID: organizationID, Role: authz.MerchantAdmin}
	switch purpose {
	case "booking_fee":
		return s.confirmBookingFee(ctx, actor, requestID, orderID)
	case "quote":
		quoteID, parseErr := uuid.Parse(strings.TrimSpace(stringValue(meta["service_quote_id"])))
		if parseErr != nil {
			return nil
		}
		return s.confirmQuotePayment(ctx, actor, quoteID, orderID)
	default:
		return nil
	}
}

func (s *Service) confirmBookingFee(ctx context.Context, actor auth.CurrentUser, requestID, orderID uuid.UUID) error {
	var request Request
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, requestID).First(&request).Error; err != nil {
		return nil
	}
	if request.Status != RequestAwaitingPayment && request.Status != RequestDraft {
		return nil
	}
	order, err := s.commerce.GetOrder(ctx, actor, orderID)
	if err != nil || order.Status != core.OrderPaid {
		return nil
	}
	if err := s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{"status": RequestMatching, "booking_order_id": orderID, "updated_at": s.now()}).Error; err != nil {
		return err
	}
	s.audit(ctx, actor, "service_request", request.ID, "booking_fee_paid", "{}")
	if request.CustomerPhone != "" {
		_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, "Payment received. We're matching you with a nearby professional now.")
	}
	return s.StartMatching(ctx, actor, request.ID)
}

// HandleHandoffInbound interprets a customer message that arrives while the
// conversation is in human handoff. It only recognises an intent when that
// intent is actually available — quote replies are read only while a quote is
// awaiting a decision, and a rating only after the job is complete — so an
// ordinary sentence like "how do I pay?" is passed through to the provider
// instead of silently approving a quote.
func (s *Service) HandleHandoffInbound(ctx context.Context, organizationID, sessionID uuid.UUID, text string) (bool, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return false, "", nil
	}
	var request Request
	err := s.db.WithContext(ctx).Where("organization_id = ? AND conversation_session_id = ?", organizationID, sessionID).Order("created_at DESC").First(&request).Error
	if err != nil {
		return false, "", nil
	}
	actor := auth.CurrentUser{ID: uuid.Nil, OrganizationID: organizationID, Role: authz.MerchantAdmin}
	s.saveMessage(ctx, organizationID, request.ID, "customer", nil, text)
	intent := parseCustomerIntent(text)

	if quote, quoteErr := s.latestQuote(ctx, request.ID); quoteErr == nil {
		if isPaymentConfirmation(text) {
			switch quote.Status {
			case QuotePaid:
				return true, s.paymentConfirmationMessage(ctx, actor, request, quote), nil
			case QuoteApproved:
				if quote.PaymentOrderID == nil {
					return true, "I haven't confirmed that payment yet. Open the link, then reply I HAVE PAID.", nil
				}
				payments, err := s.commerce.ListPayments(ctx, actor)
				if err != nil {
					return true, friendlyError(err), nil
				}
				for _, payment := range payments {
					if payment.OrderID != *quote.PaymentOrderID {
						continue
					}
					if _, err := s.commerce.VerifyPayment(ctx, actor, core.PaymentVerifyInput{Reference: payment.Reference}); err != nil {
						return true, "I haven't confirmed that payment yet. Open the link, then reply I HAVE PAID.", nil
					}
					return true, s.paymentConfirmationMessage(ctx, actor, request, quote), nil
				}
				return true, "I haven't confirmed that payment yet. Open the link, then reply I HAVE PAID.", nil
			}
		}
		if quote.Status != QuoteSent {
			return true, "", nil
		}
		switch intent {
		case intentApprove:
			payment, err := s.ApproveQuote(ctx, actor, quote.ID)
			if err != nil {
				return true, friendlyError(err), nil
			}
			return true, fmt.Sprintf("Great — here's your secure payment link for %s:\n%s", formatMoney(quote.TotalMinor, quote.Currency), payment.AuthorizationURL), nil
		case intentDecline:
			if err := s.DeclineQuote(ctx, actor, quote.ID); err != nil {
				return true, friendlyError(err), nil
			}
			return true, "Quote declined. You can keep chatting here if you'd like to adjust the job.", nil
		case intentQuestion:
			return true, "Sure — send your question here and your professional will reply.", nil
		}
	}

	if request.Status == RequestCompleted {
		if score := parseStar(strings.ToLower(text)); score > 0 {
			if err := s.SubmitRating(ctx, actor, request.ID, score, ""); err != nil {
				return true, friendlyError(err), nil
			}
			return true, "Thank you for the rating. We hope to help you again.", nil
		}
	}

	// Anything else is a message for the provider, already stored above.
	return true, "", nil
}

type customerIntent int

const (
	intentNone customerIntent = iota
	intentApprove
	intentDecline
	intentQuestion
)

// parseCustomerIntent matches the whole message against known replies rather
// than searching for keywords inside it, so "I'd rather not decline yet" is not
// read as a decline.
func parseCustomerIntent(text string) customerIntent {
	normalized := normalizeReply(text)
	switch normalized {
	case "approve", "approve pay", "approve and pay", "approve the quote", "approve quote", "accept", "accept quote", "pay", "pay now", "proceed", "go ahead", "yes", "y", "ok", "okay", "1":
		return intentApprove
	case "decline", "decline quote", "reject", "reject quote", "no", "n", "cancel", "2":
		return intentDecline
	case "ask a question", "ask question", "question", "i have a question", "3":
		return intentQuestion
	}
	return intentNone
}

func isPaymentConfirmation(text string) bool {
	switch normalizeReply(text) {
	case "i have paid", "ive paid", "paid", "payment made", "done":
		return true
	default:
		return false
	}
}

func (s *Service) paymentConfirmationMessage(ctx context.Context, actor auth.CurrentUser, request Request, quote Quote) string {
	if current, err := s.getRequest(ctx, actor.OrganizationID, request.ID); err == nil {
		request = current
	}
	providerName := "Assigned professional"
	if request.AssignedProvider != nil && strings.TrimSpace(request.AssignedProvider.Name) != "" {
		providerName = request.AssignedProvider.Name
	}
	serviceName := request.Pool.Name
	if serviceName == "" {
		serviceName = "Home service"
	}
	return fmt.Sprintf("%s payment confirmation\nService: %s\nHandyman: %s\nQuote: %s\nWork: %s\nAmount: %s\nPayment: Confirmed\nJob status: %s",
		s.companyName(ctx, actor.OrganizationID), serviceName, providerName, quote.PublicCode, request.Description,
		formatMoney(quote.TotalMinor, quote.Currency), strings.ReplaceAll(request.Status, "_", " "))
}

// normalizeReply lowercases, drops punctuation and collapses whitespace so
// "APPROVE & PAY" and "approve and pay" reach the same key.
func normalizeReply(text string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '&':
			builder.WriteString(" and ")
		default:
			builder.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

// friendlyError surfaces the message from an httperror without leaking internal
// detail into a WhatsApp reply.
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	var apiErr httperror.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
		return apiErr.Message
	}
	return "Sorry, I couldn't do that just now. A coordinator will follow up."
}

func (s *Service) audit(ctx context.Context, actor auth.CurrentUser, targetType string, targetID uuid.UUID, action, metadata string) {
	orgID := actor.OrganizationID
	log := organization.AuditLog{ID: uuid.New(), OrganizationID: &orgID, TargetType: targetType, TargetID: &targetID, Action: action, Metadata: defaultJSON(metadata)}
	if actor.ID != uuid.Nil {
		log.ActorUserID = &actor.ID
	}
	_ = s.db.WithContext(ctx).Create(&log).Error
}

func (s *Service) getPool(ctx context.Context, organizationID, id uuid.UUID) (Pool, error) {
	var pool Pool
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, id).First(&pool).Error
	if err == gorm.ErrRecordNotFound {
		return Pool{}, httperror.NotFound("Service not found")
	}
	return pool, err
}

func (s *Service) getProvider(ctx context.Context, organizationID, id uuid.UUID) (Provider, error) {
	var provider Provider
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, id).Preload("Pools").First(&provider).Error
	if err == gorm.ErrRecordNotFound {
		return Provider{}, httperror.NotFound("Provider not found")
	}
	return provider, err
}

func (s *Service) getRequest(ctx context.Context, organizationID, id uuid.UUID) (Request, error) {
	var request Request
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, id).Preload("Pool").Preload("AssignedProvider").First(&request).Error
	if err == gorm.ErrRecordNotFound {
		return Request{}, httperror.NotFound("Request not found")
	}
	return request, err
}

func (s *Service) providerForUser(ctx context.Context, actor auth.CurrentUser) (Provider, error) {
	var provider Provider
	err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", actor.OrganizationID, actor.ID).Preload("Pools").First(&provider).Error
	if err == gorm.ErrRecordNotFound {
		return Provider{}, httperror.Forbidden("No provider profile is linked to this account")
	}
	return provider, err
}

// nextCode derives the next human-readable code from the highest code already
// issued rather than from a row count, so cancelled or deleted rows cannot make
// it hand back a code that already exists.
func (s *Service) nextCode(ctx context.Context, organizationID uuid.UUID, prefix string) string {
	var codes []string
	model := any(&Request{})
	if prefix == "QTE" {
		model = &Quote{}
	}
	_ = s.db.WithContext(ctx).Model(model).
		Where("organization_id = ? AND public_code LIKE ?", organizationID, prefix+"-%").
		Pluck("public_code", &codes).Error
	highest := 0
	for _, code := range codes {
		suffix := strings.TrimPrefix(code, prefix+"-")
		value, err := strconv.Atoi(suffix)
		if err == nil && value > highest {
			highest = value
		}
	}
	return fmt.Sprintf("%s-%04d", prefix, highest+1)
}

func (s *Service) notify(ctx context.Context, organizationID uuid.UUID, recipient, text string) error {
	if s.dispatcher == nil || strings.TrimSpace(recipient) == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	return s.dispatcher.SendText(ctx, organizationID, recipient, text)
}

func (s *Service) ensureProviderUserTx(tx *gorm.DB, actor auth.CurrentUser, provider Provider, password string) (uuid.UUID, error) {
	email := strings.ToLower(strings.TrimSpace(provider.Email))
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(provider.PublicCode)) + "@providers.zidicommerce.local"
	}
	if !strings.Contains(email, "@") {
		return uuid.Nil, httperror.BadRequest("A valid provider email is required")
	}
	if password == "" {
		password = "ChangeMeSoon1!"
	}
	var user organization.User
	err := tx.Where("lower(email) = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return uuid.Nil, hashErr
		}
		orgID := actor.OrganizationID
		user = organization.User{ID: uuid.New(), OrganizationID: &orgID, Email: email, FirstName: provider.Name, PasswordHash: string(hash), Role: authz.ServiceProvider, Status: "active"}
		if err := tx.Create(&user).Error; err != nil {
			return uuid.Nil, err
		}
	} else if err != nil {
		return uuid.Nil, err
	}
	membership := organization.OrganizationMembership{ID: uuid.New(), OrganizationID: actor.OrganizationID, UserID: user.ID, Role: authz.ServiceProvider, Status: "active"}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"role": authz.ServiceProvider, "status": "active", "updated_at": s.now()})}).Create(&membership).Error; err != nil {
		return uuid.Nil, err
	}
	return user.ID, nil
}

func slugify(slug, name string) string {
	value := strings.ToLower(strings.TrimSpace(slug))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(name))
	}
	value = strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(value)
	return strings.Trim(value, "-")
}

func parseMap(raw string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(defaultJSON(raw)), &out)
	return out
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func defaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func parseStar(text string) int {
	text = strings.TrimSpace(text)
	if len(text) == 1 && text[0] >= '1' && text[0] <= '5' {
		return int(text[0] - '0')
	}
	for n := 5; n >= 1; n-- {
		if strings.Contains(text, fmt.Sprintf("%d star", n)) {
			return n
		}
	}
	return 0
}

func (s *Service) saveMessage(ctx context.Context, organizationID, requestID uuid.UUID, authorType string, author *uuid.UUID, body string) {
	msg := Message{ID: uuid.New(), OrganizationID: organizationID, RequestID: requestID, AuthorType: authorType, AuthorUserID: author, Body: body, Metadata: "{}"}
	_ = s.db.WithContext(ctx).Create(&msg).Error
}

func (s *Service) latestQuote(ctx context.Context, requestID uuid.UUID) (Quote, error) {
	var quote Quote
	err := s.db.WithContext(ctx).Where("request_id = ?", requestID).Order("created_at DESC").First(&quote).Error
	if err == gorm.ErrRecordNotFound {
		return Quote{}, httperror.NotFound("Quote not found")
	}
	return quote, err
}
