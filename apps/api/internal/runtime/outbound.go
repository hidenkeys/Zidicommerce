package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
)

type ChannelSender interface {
	Send(ctx context.Context, channel core.Channel, recipient string, message OutboundMessage, payload map[string]any) (ProviderSendResult, error)
}

type ProviderSendResult struct {
	ProviderMessageID string
	Response          map[string]any
}

func (s *Service) RegisterChannelSender(provider string, sender ChannelSender) {
	if s.senders == nil {
		s.senders = map[string]ChannelSender{}
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "" && sender != nil {
		s.senders[provider] = sender
	}
}

func (s *Service) DispatchOutbound(ctx context.Context, channel core.Channel, input InboundMessage, result RuntimeResult) ([]ChannelOutboundMessage, error) {
	deliveries := make([]ChannelOutboundMessage, 0, len(result.Messages))
	for _, message := range result.Messages {
		payload := s.channelPayload(channel, input.Sender, message)
		sessionID := result.ConversationID
		delivery := ChannelOutboundMessage{
			ID:                     uuid.New(),
			OrganizationID:         channel.OrganizationID,
			ChannelID:              channel.ID,
			SessionID:              &sessionID,
			ExternalConversationID: input.ExternalConversationID,
			Recipient:              input.Sender,
			Provider:               channel.Provider,
			MessageType:            message.Type,
			Status:                 OutboundQueued,
			Payload:                jsonValue(payload),
			ProviderResponse:       "{}",
		}
		if err := s.db.WithContext(ctx).Create(&delivery).Error; err != nil {
			return deliveries, err
		}
		sender, ok := s.senders[strings.ToLower(channel.Provider)]
		if !ok {
			now := s.now()
			delivery.Status = OutboundSkipped
			delivery.ErrorMessage = "channel sender is not configured"
			delivery.UpdatedAt = now
			if err := s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": delivery.Status, "error_message": delivery.ErrorMessage, "updated_at": now}).Error; err != nil {
				return deliveries, err
			}
			deliveries = append(deliveries, delivery)
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		providerResult, err := sender.Send(sendCtx, channel, input.Sender, message, payload)
		cancel()
		if err != nil {
			now := s.now()
			nextAttempt := now.Add(2 * time.Minute)
			delivery.Status = OutboundFailed
			delivery.ErrorMessage = publicSendError(err)
			delivery.Attempts = 1
			delivery.NextAttemptAt = &nextAttempt
			delivery.UpdatedAt = now
			if updateErr := s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": delivery.Status, "error_message": delivery.ErrorMessage, "attempts": delivery.Attempts, "next_attempt_at": &nextAttempt, "updated_at": now}).Error; updateErr != nil {
				return deliveries, updateErr
			}
			s.log.Warn("runtime outbound send failed", "organization_id", channel.OrganizationID, "channel_id", channel.ID, "provider", channel.Provider, "delivery_id", delivery.ID, "error", delivery.ErrorMessage)
			deliveries = append(deliveries, delivery)
			continue
		}
		now := s.now()
		delivery.Status = OutboundSent
		delivery.Attempts = 1
		delivery.ProviderMessageID = providerResult.ProviderMessageID
		delivery.ProviderResponse = jsonValue(providerResult.Response)
		delivery.SentAt = &now
		delivery.UpdatedAt = now
		if err := s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": delivery.Status, "attempts": delivery.Attempts, "provider_message_id": delivery.ProviderMessageID, "provider_response": delivery.ProviderResponse, "sent_at": &now, "updated_at": now}).Error; err != nil {
			return deliveries, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, nil
}

func (s *Service) ListOutboundMessages(ctx context.Context, organizationID uuid.UUID, limit int) ([]ChannelOutboundMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []ChannelOutboundMessage
	err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *Service) channelPayload(channel core.Channel, recipient string, message OutboundMessage) map[string]any {
	if strings.EqualFold(channel.Provider, "whatsapp") {
		payload := s.TranslateWhatsAppOutbound(message)
		payload["messaging_product"] = "whatsapp"
		payload["recipient_type"] = "individual"
		payload["to"] = recipient
		return payload
	}
	return map[string]any{"to": recipient, "type": message.Type, "text": message.Text, "options": message.Options, "media_url": message.MediaURL}
}

type WhatsAppCloudSender struct {
	graphBaseURL string
	client       *http.Client
	log          *slog.Logger
}

func NewWhatsAppCloudSender(graphBaseURL string, logger *slog.Logger) *WhatsAppCloudSender {
	if logger == nil {
		logger = slog.Default()
	}
	graphBaseURL = strings.TrimRight(strings.TrimSpace(graphBaseURL), "/")
	if graphBaseURL == "" {
		graphBaseURL = "https://graph.facebook.com"
	}
	return &WhatsAppCloudSender{graphBaseURL: graphBaseURL, client: &http.Client{Timeout: 12 * time.Second}, log: logger}
}

func (s *WhatsAppCloudSender) Send(ctx context.Context, channel core.Channel, recipient string, _ OutboundMessage, payload map[string]any) (ProviderSendResult, error) {
	secrets := parseJSONMap(channel.SecretConfig)
	config := parseJSONMap(channel.Config)
	token := strings.TrimSpace(stringValue(secrets["access_token"]))
	if token == "" {
		return ProviderSendResult{}, errors.New("whatsapp access token is not configured")
	}
	phoneNumberID := strings.TrimSpace(channel.PhoneNumberID)
	if phoneNumberID == "" {
		return ProviderSendResult{}, errors.New("whatsapp phone number id is not configured")
	}
	graphVersion := strings.TrimSpace(stringValue(config["graph_version"]))
	if graphVersion == "" {
		graphVersion = "v20.0"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProviderSendResult{}, err
	}
	endpoint := fmt.Sprintf("%s/%s/%s/messages", s.graphBaseURL, graphVersion, phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ProviderSendResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return ProviderSendResult{}, err
	}
	defer resp.Body.Close()
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ProviderSendResult{}, err
	}
	if resp.StatusCode >= 400 {
		return ProviderSendResult{Response: decoded}, fmt.Errorf("whatsapp send failed with status %d", resp.StatusCode)
	}
	return ProviderSendResult{ProviderMessageID: extractWhatsAppMessageID(decoded), Response: decoded}, nil
}

func extractWhatsAppMessageID(response map[string]any) string {
	messages, ok := response["messages"].([]any)
	if !ok || len(messages) == 0 {
		return ""
	}
	first, ok := messages[0].(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(first["id"])
}

func publicSendError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 240 {
		return message[:240]
	}
	return message
}

type MockChannelSender struct {
	Responses []ProviderSendResult
	Errors    []error
	Sent      []map[string]any
}

func (m *MockChannelSender) Send(_ context.Context, _ core.Channel, recipient string, _ OutboundMessage, payload map[string]any) (ProviderSendResult, error) {
	m.Sent = append(m.Sent, map[string]any{"recipient": recipient, "payload": payload})
	index := len(m.Sent) - 1
	if index < len(m.Errors) && m.Errors[index] != nil {
		return ProviderSendResult{}, m.Errors[index]
	}
	if index < len(m.Responses) {
		return m.Responses[index], nil
	}
	return ProviderSendResult{ProviderMessageID: uuid.NewString(), Response: map[string]any{"ok": true}}, nil
}

func ensureOutboundMigrated(db *gorm.DB) error {
	return db.AutoMigrate(&ChannelOutboundMessage{})
}
