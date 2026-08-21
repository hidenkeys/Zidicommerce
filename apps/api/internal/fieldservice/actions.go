package fieldservice

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

func runtimeActor(organizationID uuid.UUID) auth.CurrentUser {
	return auth.CurrentUser{ID: uuid.Nil, OrganizationID: organizationID, Role: authz.MerchantAdmin}
}

func (s *Service) RuntimeWelcome(ctx context.Context, organizationID uuid.UUID) (map[string]any, error) {
	settings, err := s.ensureSettings(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"welcome_message": settings.WelcomeMessage, "company_name": settings.CompanyDisplayName, "message": settings.WelcomeMessage}, nil
}

func (s *Service) RuntimeListPools(ctx context.Context, organizationID uuid.UUID) (map[string]any, error) {
	actor := runtimeActor(organizationID)
	pools, err := s.ListPools(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(pools))
	for _, pool := range pools {
		rows = append(rows, map[string]any{"id": pool.Slug, "pool_id": pool.ID.String(), "name": pool.Name, "label": pool.Name})
	}
	// No "message" key: the menu is handed to the question step so the customer
	// sees one prompt rather than a list followed by a separate instruction.
	return map[string]any{"service_options": rows, "count": len(rows), "menu": numbered(rows)}, nil
}

func (s *Service) RuntimeSelectPool(ctx context.Context, organizationID uuid.UUID, selection string, options []map[string]any) (map[string]any, error) {
	choice := strings.ToLower(strings.TrimSpace(selection))
	// The customer was shown a numbered list, so a bare number is a valid answer.
	if index, err := strconv.Atoi(choice); err == nil && index >= 1 && index <= len(options) {
		row := options[index-1]
		return map[string]any{"pool_id": stringValue(row["pool_id"]), "service_name": stringValue(row["name"]), "message": "Got it — " + stringValue(row["name"]) + "."}, nil
	}
	for _, row := range options {
		name := strings.ToLower(stringValue(row["name"]))
		id := strings.ToLower(stringValue(row["id"]))
		if choice == name || choice == id || strings.Contains(choice, name) {
			return map[string]any{"pool_id": stringValue(row["pool_id"]), "service_name": stringValue(row["name"]), "message": "Got it — " + stringValue(row["name"]) + "."}, nil
		}
	}
	actor := runtimeActor(organizationID)
	pools, err := s.ListPools(ctx, actor)
	if err != nil {
		return nil, err
	}
	for _, pool := range pools {
		if strings.EqualFold(pool.Name, selection) || strings.EqualFold(pool.Slug, selection) || strings.Contains(strings.ToLower(selection), strings.ToLower(pool.Name)) {
			return map[string]any{"pool_id": pool.ID.String(), "service_name": pool.Name, "message": "Got it — " + pool.Name + "."}, nil
		}
	}
	return nil, fmt.Errorf("I didn't catch that service. Please choose one of the options.")
}

func (s *Service) RuntimeCreateRequest(ctx context.Context, organizationID uuid.UUID, customerID uuid.UUID, sessionID, channelID *uuid.UUID, input CreateRequestInput) (map[string]any, error) {
	input.CustomerID = customerID
	input.SessionID = sessionID
	input.ChannelID = channelID
	request, err := s.CreateRequest(ctx, runtimeActor(organizationID), input)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"request_id":       request.ID.String(),
		"request_code":     request.PublicCode,
		"service_name":     request.Pool.Name,
		"location_summary": displayLocation(request),
		"booking_fee":      formatNaira(request.BookingFeeMinor),
		"message":          fmt.Sprintf("Service: %s\nLocation: %s\nBooking fee: %s", request.Pool.Name, displayLocation(request), formatNaira(request.BookingFeeMinor)),
	}, nil
}

func (s *Service) RuntimeStartBookingFee(ctx context.Context, organizationID uuid.UUID, requestID uuid.UUID) (map[string]any, error) {
	request, payment, err := s.InitializeBookingFee(ctx, runtimeActor(organizationID), requestID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"request_id":   request.ID.String(),
		"request_code": request.PublicCode,
		"message":      fmt.Sprintf("Pay %s to continue.", formatNaira(request.BookingFeeMinor)),
	}
	if payment != nil {
		out["payment_url"] = payment.AuthorizationURL
		out["payment_reference"] = payment.Reference
		out["payment_status"] = payment.Status
		out["message"] = fmt.Sprintf("Pay %s to continue.\n\n%s", formatNaira(request.BookingFeeMinor), payment.AuthorizationURL)
	}
	return out, nil
}

func (s *Service) RuntimeCheckBookingPayment(ctx context.Context, organizationID uuid.UUID, reference string) (map[string]any, error) {
	actor := runtimeActor(organizationID)
	if err := s.ConfirmBookingFromReference(ctx, actor, reference); err != nil {
		return map[string]any{"payment_status": "pending", "message": "I haven't confirmed that payment yet. Open the link, then reply I HAVE PAID."}, nil
	}
	return map[string]any{"payment_status": core.PaymentPaid, "message": "Payment confirmed. We're matching you with a professional now."}, nil
}

func numbered(rows []map[string]any) string {
	lines := make([]string, 0, len(rows))
	for index, row := range rows {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, stringValue(row["name"])))
	}
	return strings.Join(lines, "\n")
}
