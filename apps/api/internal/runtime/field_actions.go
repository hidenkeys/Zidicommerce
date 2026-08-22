package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/fieldservice"
)

func (s *Service) RegisterFieldService(field *fieldservice.Service) {
	if field == nil {
		return
	}
	s.lifecycleCancel = func(ctx context.Context, runtimeContext RuntimeContext) (bool, string, error) {
		requestIDText := stringValue(runtimeContext.Variables["request_id"])
		if requestIDText == "" {
			return false, "", nil
		}
		requestID, err := uuid.Parse(requestIDText)
		if err != nil {
			return true, "", runtimeErrorf(ErrInvalidInput, "I couldn't identify that request.", "invalid field-service request id: %v", err)
		}
		customerID := uuid.Nil
		if runtimeContext.Session.CustomerID != nil {
			customerID = *runtimeContext.Session.CustomerID
		}
		message, err := field.RuntimeCancelRequest(ctx, runtimeContext.Session.OrganizationID, customerID, runtimeContext.Session.ID, requestID)
		if err != nil {
			return true, "", err
		}
		return true, message, nil
	}
	s.RegisterAction("get_service_welcome", func(ctx context.Context, runtimeContext RuntimeContext, _ map[string]any) (map[string]any, error) {
		return field.RuntimeWelcome(ctx, runtimeContext.Session.OrganizationID)
	})
	s.RegisterAction("list_service_pools", func(ctx context.Context, runtimeContext RuntimeContext, _ map[string]any) (map[string]any, error) {
		return field.RuntimeListPools(ctx, runtimeContext.Session.OrganizationID)
	})
	s.RegisterAction("select_service_pool", func(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
		options, _ := inputs["service_options"].([]map[string]any)
		if options == nil {
			if raw, ok := runtimeContext.Variables["service_options"].([]any); ok {
				for _, row := range raw {
					if mapped, ok := row.(map[string]any); ok {
						options = append(options, mapped)
					}
				}
			}
		}
		return field.RuntimeSelectPool(ctx, runtimeContext.Session.OrganizationID, stringValue(inputs["selection"]), options)
	})
	s.RegisterAction("create_service_request", func(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
		poolID, err := uuid.Parse(stringValue(first(inputs["pool_id"], runtimeContext.Variables["pool_id"])))
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Please choose a service first.")
		}
		customerID := uuid.Nil
		if runtimeContext.Session.CustomerID != nil {
			customerID = *runtimeContext.Session.CustomerID
		}
		lat, lng := locationFrom(inputs, runtimeContext)
		area := stringValue(first(inputs["area"], runtimeContext.Variables["area"], runtimeContext.Variables["location_name"]))
		if area == "" {
			area = stringValue(runtimeContext.Variables["location_address"])
		}
		return field.RuntimeCreateRequest(ctx, runtimeContext.Session.OrganizationID, customerID, &runtimeContext.Session.ID, &runtimeContext.Session.ChannelID, fieldservice.CreateRequestInput{
			PoolID:        poolID,
			CustomerName:  stringValue(first(inputs["customer_name"], runtimeContext.Variables["customer_name"])),
			CustomerPhone: stringValue(first(inputs["customer_phone"], runtimeContext.Variables["customer_phone"], runtimeContext.Session.ExternalConversationID)),
			Area:          area,
			Address:       stringValue(first(inputs["address"], runtimeContext.Variables["address"], runtimeContext.Variables["location_address"])),
			Latitude:      lat,
			Longitude:     lng,
			Description:   stringValue(first(inputs["description"], runtimeContext.Variables["issue"])),
			PreferredAt:   stringValue(first(inputs["preferred_at"], runtimeContext.Variables["preferred_at"])),
		})
	})
	s.RegisterAction("initialize_booking_fee", func(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
		requestID, err := uuid.Parse(stringValue(first(inputs["request_id"], runtimeContext.Variables["request_id"])))
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "I couldn't find that booking.")
		}
		return field.RuntimeStartBookingFee(ctx, runtimeContext.Session.OrganizationID, requestID)
	})
	s.RegisterAction("check_booking_payment", func(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
		return field.RuntimeCheckBookingPayment(ctx, runtimeContext.Session.OrganizationID, stringValue(first(inputs["payment_reference"], runtimeContext.Variables["payment_reference"])))
	})
	s.RegisterAction("submit_service_rating", func(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
		requestID, err := uuid.Parse(stringValue(runtimeContext.Variables["request_id"]))
		if err != nil {
			return map[string]any{"ok": false}, nil
		}
		score := int(int64Value(inputs["score"]))
		if score == 0 {
			score = parseStar(stringValue(inputs["rating"]))
		}
		actor := auth.CurrentUser{OrganizationID: runtimeContext.Session.OrganizationID, Role: authz.MerchantAdmin}
		if err := field.SubmitRating(ctx, actor, requestID, score, stringValue(inputs["feedback"])); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "message": "Thank you for the rating."}, nil
	})
}

func first(values ...any) any {
	for _, value := range values {
		if stringValue(value) != "" {
			return value
		}
	}
	return ""
}

func locationFrom(inputs map[string]any, runtimeContext RuntimeContext) (*float64, *float64) {
	parse := func(raw any) *float64 {
		switch typed := raw.(type) {
		case float64:
			v := typed
			return &v
		}
		return nil
	}
	lat := parse(first(inputs["latitude"], runtimeContext.Variables["latitude"]))
	lng := parse(first(inputs["longitude"], runtimeContext.Variables["longitude"]))
	if loc, ok := runtimeContext.Variables["location"].(map[string]any); ok {
		if lat == nil {
			if value, ok := loc["latitude"].(float64); ok {
				lat = &value
			}
		}
		if lng == nil {
			if value, ok := loc["longitude"].(float64); ok {
				lng = &value
			}
		}
	}
	return lat, lng
}

func parseStar(text string) int {
	if len(text) == 1 && text[0] >= '1' && text[0] <= '5' {
		return int(text[0] - '0')
	}
	return 0
}
