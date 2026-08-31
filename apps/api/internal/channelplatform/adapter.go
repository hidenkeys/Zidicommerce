package channelplatform

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type MediaReference struct {
	Type        string `json:"type"`
	Reference   string `json:"reference"`
	ContentType string `json:"content_type,omitempty"`
}

type OutboundAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// TemplateReference remains provider-neutral while allowing adapters to enforce
// provider policy and translate an approved local template at the boundary.
type TemplateReference struct {
	ID            uuid.UUID         `json:"id"`
	Name          string            `json:"name,omitempty"`
	Language      string            `json:"language,omitempty"`
	VariableOrder []string          `json:"variable_order,omitempty"`
	Variables     map[string]string `json:"variables,omitempty"`
}

type InboundEvent struct {
	Provider               string           `json:"provider"`
	ChannelConnectionID    uuid.UUID        `json:"channel_connection_id"`
	ProviderEventID        string           `json:"provider_event_id"`
	ExternalCustomerID     string           `json:"external_customer_id"`
	ExternalMessageID      string           `json:"external_message_id"`
	ExternalConversationID string           `json:"external_conversation_id"`
	MessageType            string           `json:"message_type"`
	Body                   string           `json:"body"`
	Media                  []MediaReference `json:"media,omitempty"`
	Timestamp              time.Time        `json:"timestamp"`
	Metadata               map[string]any   `json:"metadata,omitempty"`
}

type OutboundCommand struct {
	Provider               string             `json:"provider"`
	ChannelConnectionID    uuid.UUID          `json:"channel_connection_id"`
	ConversationID         *uuid.UUID         `json:"conversation_id,omitempty"`
	ExternalCustomerID     string             `json:"external_customer_id"`
	ExternalConversationID string             `json:"external_conversation_id"`
	MessageType            string             `json:"message_type"`
	Body                   string             `json:"body"`
	Media                  []MediaReference   `json:"media,omitempty"`
	Actions                []OutboundAction   `json:"actions,omitempty"`
	Template               *TemplateReference `json:"template,omitempty"`
	IdempotencyKey         string             `json:"idempotency_key"`
	Metadata               map[string]any     `json:"metadata,omitempty"`
}

type ProviderEnvelope struct {
	Provider         string
	ConnectionID     uuid.UUID
	Headers          map[string]string
	RawPayload       []byte
	ReceivedAt       time.Time
	NormalizedEvents []InboundEvent
}

type ProviderRequest struct {
	Provider     string
	ConnectionID uuid.UUID
	Payload      map[string]any
}

type DeliveryUpdate struct {
	ProviderMessageID string
	Status            string
	OccurredAt        time.Time
	ErrorCode         string
	ErrorMessage      string
}

type SendResult struct {
	ProviderMessageID string         `json:"provider_message_id"`
	StatusCode        int            `json:"status_code"`
	ResponseMetadata  map[string]any `json:"response_metadata,omitempty"`
}

type OutboundSender interface {
	Send(context.Context, OutboundCommand) (SendResult, error)
}

type Adapter interface {
	Provider() string
	VerifySignature(context.Context, ProviderEnvelope) error
	NormalizeInbound(context.Context, ProviderEnvelope) ([]InboundEvent, error)
	BuildOutbound(context.Context, OutboundCommand) (ProviderRequest, error)
	NormalizeDelivery(context.Context, ProviderEnvelope) ([]DeliveryUpdate, error)
}

// NoopAdapter defines the Phase J contract without performing network I/O.
// Tests and future providers can supply already-normalized events to verify the
// adapter boundary independently from the conversation engine.
type NoopAdapter struct {
	provider string
}

func NewNoopAdapter(provider string) *NoopAdapter {
	return &NoopAdapter{provider: strings.ToLower(strings.TrimSpace(provider))}
}

func (a *NoopAdapter) Provider() string { return a.provider }

func (a *NoopAdapter) VerifySignature(context.Context, ProviderEnvelope) error { return nil }

func (a *NoopAdapter) NormalizeInbound(_ context.Context, envelope ProviderEnvelope) ([]InboundEvent, error) {
	if strings.TrimSpace(envelope.Provider) == "" || envelope.ConnectionID == uuid.Nil {
		return nil, errors.New("provider and channel connection are required")
	}
	events := make([]InboundEvent, 0, len(envelope.NormalizedEvents))
	for _, event := range envelope.NormalizedEvents {
		event.Provider = strings.ToLower(strings.TrimSpace(envelope.Provider))
		event.ChannelConnectionID = envelope.ConnectionID
		if event.Timestamp.IsZero() {
			event.Timestamp = envelope.ReceivedAt
		}
		if err := validateInboundEvent(event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (a *NoopAdapter) BuildOutbound(_ context.Context, command OutboundCommand) (ProviderRequest, error) {
	if err := validateOutboundCommand(command); err != nil {
		return ProviderRequest{}, err
	}
	return ProviderRequest{
		Provider:     strings.ToLower(strings.TrimSpace(command.Provider)),
		ConnectionID: command.ChannelConnectionID,
		Payload: map[string]any{
			"external_customer_id":     command.ExternalCustomerID,
			"external_conversation_id": command.ExternalConversationID,
			"message_type":             command.MessageType,
			"body":                     command.Body,
			"media":                    command.Media,
			"idempotency_key":          command.IdempotencyKey,
		},
	}, nil
}

func (a *NoopAdapter) NormalizeDelivery(context.Context, ProviderEnvelope) ([]DeliveryUpdate, error) {
	return []DeliveryUpdate{}, nil
}

func validateInboundEvent(event InboundEvent) error {
	if strings.TrimSpace(event.ProviderEventID) == "" || strings.TrimSpace(event.ExternalMessageID) == "" {
		return errors.New("provider event and external message IDs are required")
	}
	if strings.TrimSpace(event.ExternalCustomerID) == "" || strings.TrimSpace(event.ExternalConversationID) == "" {
		return errors.New("external customer and conversation IDs are required")
	}
	if strings.TrimSpace(event.MessageType) == "" {
		return errors.New("message type is required")
	}
	return nil
}

func validateOutboundCommand(command OutboundCommand) error {
	if strings.TrimSpace(command.Provider) == "" || command.ChannelConnectionID == uuid.Nil {
		return errors.New("provider and channel connection are required")
	}
	if strings.TrimSpace(command.ExternalCustomerID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" {
		return errors.New("external customer and idempotency key are required")
	}
	if strings.TrimSpace(command.MessageType) == "" {
		return errors.New("message type is required")
	}
	return nil
}
