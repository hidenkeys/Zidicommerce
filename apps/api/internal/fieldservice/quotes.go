package fieldservice

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type QuoteInput struct {
	LabourMinor     int64       `json:"labour_minor"`
	MaterialsMinor  int64       `json:"materials_minor"`
	AdditionalMinor int64       `json:"additional_minor"`
	DiscountMinor   int64       `json:"discount_minor"`
	Notes           string      `json:"notes"`
	Items           []QuoteItem `json:"items"`
}

func (s *Service) CreateQuote(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID, input QuoteInput) (Quote, error) {
	provider, err := s.assignedActor(ctx, actor, requestID)
	if err != nil {
		return Quote{}, err
	}
	var assignment Assignment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND request_id = ?", actor.OrganizationID, requestID).First(&assignment).Error; err != nil {
		return Quote{}, httperror.NotFound("Assignment not found")
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return Quote{}, err
	}
	if !canQuote(request.Status) {
		return Quote{}, httperror.BadRequest("A quote cannot be sent for a job that is " + strings.ReplaceAll(request.Status, "_", " ") + ".")
	}
	if input.LabourMinor < 0 || input.MaterialsMinor < 0 || input.AdditionalMinor < 0 || input.DiscountMinor < 0 {
		return Quote{}, httperror.BadRequest("Quote amounts cannot be negative")
	}
	total := input.LabourMinor + input.MaterialsMinor + input.AdditionalMinor - input.DiscountMinor
	if total <= 0 {
		return Quote{}, httperror.BadRequest("Quote total must be greater than zero")
	}
	settings, _ := s.ensureSettings(ctx, actor.OrganizationID)
	expires := s.now().Add(48 * time.Hour)
	quote := Quote{
		ID:              uuid.New(),
		OrganizationID:  actor.OrganizationID,
		RequestID:       requestID,
		AssignmentID:    assignment.ID,
		ProviderID:      provider.ID,
		PublicCode:      s.nextCode(ctx, actor.OrganizationID, "QTE"),
		LabourMinor:     input.LabourMinor,
		MaterialsMinor:  input.MaterialsMinor,
		AdditionalMinor: input.AdditionalMinor,
		DiscountMinor:   input.DiscountMinor,
		TotalMinor:      total,
		Currency:        settings.Currency,
		Notes:           strings.TrimSpace(input.Notes),
		Status:          QuoteSent,
		ExpiresAt:       &expires,
		Metadata:        "{}",
	}
	if err := s.db.WithContext(ctx).Create(&quote).Error; err != nil {
		return Quote{}, err
	}
	items := input.Items
	if len(items) == 0 {
		items = []QuoteItem{
			{Kind: "labour", Description: "Labour", AmountMinor: input.LabourMinor, SortOrder: 1},
			{Kind: "materials", Description: "Materials", AmountMinor: input.MaterialsMinor, SortOrder: 2},
		}
		if input.AdditionalMinor > 0 {
			items = append(items, QuoteItem{Kind: "additional", Description: "Additional charges", AmountMinor: input.AdditionalMinor, SortOrder: 3})
		}
		if input.DiscountMinor > 0 {
			items = append(items, QuoteItem{Kind: "discount", Description: "Discount", AmountMinor: -input.DiscountMinor, SortOrder: 4})
		}
	}
	for index, item := range items {
		item.ID = uuid.New()
		item.OrganizationID = actor.OrganizationID
		item.QuoteID = quote.ID
		item.SortOrder = index
		_ = s.db.WithContext(ctx).Create(&item).Error
	}
	_ = s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", requestID).Updates(map[string]any{"status": RequestQuoteSent, "updated_at": s.now()}).Error
	s.audit(ctx, actor, "service_request", requestID, "quote_created", fmt.Sprintf(`{"total_minor":%d}`, total))
	quoteMessage := fmt.Sprintf("%s submitted quote %s for your %s job.\nLabour: %s\nMaterials: %s\nTotal: %s\nReply APPROVE & PAY, DECLINE, or ASK A QUESTION.",
		provider.Name, quote.PublicCode, request.Pool.Name, formatMoney(input.LabourMinor, quote.Currency), formatMoney(input.MaterialsMinor, quote.Currency), formatMoney(total, quote.Currency))
	s.saveMessage(ctx, actor.OrganizationID, request.ID, "provider", &actor.ID, quoteMessage)
	_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, quoteMessage)
	return s.getQuote(ctx, actor.OrganizationID, quote.ID)
}

func (s *Service) ListQuotes(ctx context.Context, actor auth.CurrentUser) ([]Quote, error) {
	if err := s.actorOrg(actor); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Preload("Items").Order("created_at DESC")
	if actor.Role.IsServiceProvider() {
		provider, err := s.providerForUser(ctx, actor)
		if err != nil {
			return nil, err
		}
		query = query.Where("provider_id = ?", provider.ID)
	}
	var quotes []Quote
	return quotes, query.Find(&quotes).Error
}

// ApproveQuote is the customer's decision. It is reachable from the WhatsApp
// handoff (which acts with an organization actor) and from the owner dashboard.
// A provider must never be able to approve their own quote.
func (s *Service) ApproveQuote(ctx context.Context, actor auth.CurrentUser, quoteID uuid.UUID) (core.Payment, error) {
	if err := s.requireCustomerDecision(actor); err != nil {
		return core.Payment{}, err
	}
	quote, err := s.getQuote(ctx, actor.OrganizationID, quoteID)
	if err != nil {
		return core.Payment{}, err
	}
	switch quote.Status {
	case QuotePaid:
		return core.Payment{}, httperror.BadRequest("This quote is already paid")
	case QuoteDeclined:
		return core.Payment{}, httperror.BadRequest("This quote was declined. Ask your professional for a new one.")
	case QuoteApproved:
		// Re-approving re-issues the payment link, which is what a customer who
		// lost the message expects.
	case QuoteSent:
	default:
		return core.Payment{}, httperror.BadRequest("This quote is not ready to approve")
	}
	if quote.ExpiresAt != nil && quote.ExpiresAt.Before(s.now()) {
		_ = s.db.WithContext(ctx).Model(&Quote{}).Where("id = ?", quote.ID).Update("status", QuoteExpired).Error
		return core.Payment{}, httperror.BadRequest("This quote has expired. Ask your professional for a new one.")
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, quote.RequestID)
	if err != nil {
		return core.Payment{}, err
	}
	if request.Status == RequestCancelled {
		return core.Payment{}, httperror.BadRequest("This job was cancelled")
	}
	_, payment, err := s.createPaymentOrder(ctx, actor, request, quote.TotalMinor, quote.Currency, "quote", quote.ID)
	if err != nil {
		return core.Payment{}, err
	}
	if err := s.db.WithContext(ctx).Model(&Quote{}).Where("id = ?", quote.ID).Updates(map[string]any{"status": QuoteApproved, "payment_order_id": payment.OrderID, "updated_at": s.now()}).Error; err != nil {
		return core.Payment{}, err
	}
	_ = s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{"status": RequestQuoteApproved, "updated_at": s.now()}).Error
	s.audit(ctx, actor, "service_request", request.ID, "quote_approved", "{}")
	s.audit(ctx, actor, "service_request", request.ID, "payment_initiated", "{}")
	return payment, nil
}

// DeclineQuote is also the customer's decision, so it carries the same guard as
// ApproveQuote.
func (s *Service) DeclineQuote(ctx context.Context, actor auth.CurrentUser, quoteID uuid.UUID) error {
	if err := s.requireCustomerDecision(actor); err != nil {
		return err
	}
	quote, err := s.getQuote(ctx, actor.OrganizationID, quoteID)
	if err != nil {
		return err
	}
	if quote.Status == QuotePaid {
		return httperror.BadRequest("This quote is already paid")
	}
	if quote.Status == QuoteDeclined {
		return nil
	}
	if err := s.db.WithContext(ctx).Model(&Quote{}).Where("id = ?", quote.ID).Updates(map[string]any{"status": QuoteDeclined, "updated_at": s.now()}).Error; err != nil {
		return err
	}
	s.audit(ctx, actor, "service_request", quote.RequestID, "quote_rejected", "{}")
	request, _ := s.getRequest(ctx, actor.OrganizationID, quote.RequestID)
	if request.AssignedProviderID != nil {
		var provider Provider
		if s.db.WithContext(ctx).Where("id = ?", *request.AssignedProviderID).First(&provider).Error == nil {
			_ = s.notify(ctx, actor.OrganizationID, firstNonEmpty(provider.WhatsAppNumber, provider.Phone), "The customer declined the latest quote. Open the portal to follow up.")
		}
	}
	return nil
}

func (s *Service) confirmQuotePayment(ctx context.Context, actor auth.CurrentUser, quoteID, orderID uuid.UUID) error {
	quote, err := s.getQuote(ctx, actor.OrganizationID, quoteID)
	if err != nil {
		return nil
	}
	if quote.Status == QuotePaid {
		return nil
	}
	if err := s.db.WithContext(ctx).Model(&Quote{}).Where("id = ?", quote.ID).Updates(map[string]any{"status": QuotePaid, "payment_order_id": orderID, "updated_at": s.now()}).Error; err != nil {
		return err
	}
	_ = s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", quote.RequestID).Updates(map[string]any{"status": RequestPaymentConfirmed, "updated_at": s.now()}).Error
	s.audit(ctx, actor, "service_request", quote.RequestID, "payment_completed", "{}")
	request, _ := s.getRequest(ctx, actor.OrganizationID, quote.RequestID)
	confirmation := s.paymentConfirmationMessage(ctx, actor, request, quote)
	s.saveMessage(ctx, actor.OrganizationID, request.ID, "system", nil, confirmation)
	_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, confirmation)
	if request.AssignedProviderID != nil {
		var provider Provider
		if s.db.WithContext(ctx).Where("id = ?", *request.AssignedProviderID).First(&provider).Error == nil {
			_ = s.notify(ctx, actor.OrganizationID, firstNonEmpty(provider.WhatsAppNumber, provider.Phone), "Customer payment confirmed. You can continue the job.")
		}
	}
	return nil
}

func (s *Service) getQuote(ctx context.Context, organizationID, id uuid.UUID) (Quote, error) {
	var quote Quote
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, id).Preload("Items").First(&quote).Error
	if err != nil {
		return Quote{}, httperror.NotFound("Quote not found")
	}
	return quote, nil
}

// formatMoney renders an amount for a customer-facing message. Naira gets its
// symbol; any other currency is shown with its code so the amount is never
// ambiguous.
func formatMoney(minor int64, currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" || currency == "NGN" {
		return formatNaira(minor)
	}
	return fmt.Sprintf("%s %s", currency, strings.TrimPrefix(formatNaira(minor), "₦"))
}

// formatNaira renders a minor-unit amount as naira with thousands separators,
// e.g. 3250000 becomes ₦32,500.
func formatNaira(minor int64) string {
	negative := minor < 0
	if negative {
		minor = -minor
	}
	whole := strconv.FormatInt(minor/100, 10)
	var grouped strings.Builder
	for index, digit := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	if negative {
		return "-₦" + grouped.String()
	}
	return "₦" + grouped.String()
}

// requireCustomerDecision blocks a provider from acting on the customer's
// behalf. Quote approval and decline belong to the customer; the runtime and
// the owner dashboard both act with an organization-level actor.
func (s *Service) requireCustomerDecision(actor auth.CurrentUser) error {
	if err := s.actorOrg(actor); err != nil {
		return err
	}
	if actor.Role.IsServiceProvider() {
		return httperror.Forbidden("Only the customer can accept or decline a quote")
	}
	if !actor.Role.CanManageOrganization() {
		return httperror.Forbidden("You cannot act on this quote")
	}
	return nil
}

func canQuote(status string) bool {
	switch status {
	case RequestAssigned, RequestOnTheWay, RequestArrived, RequestInProgress, RequestQuoteSent, RequestQuoteApproved:
		return true
	default:
		return false
	}
}
