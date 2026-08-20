package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
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
	for index, message := range result.Messages {
		payload := s.channelPayload(channel, input.Sender, message)
		sessionID := result.ConversationID
		idempotencyKey := outboundIdempotencyKey(input, result, index)
		if existing, ok, err := s.findOutboundByIdempotency(ctx, channel, idempotencyKey); err != nil || ok {
			if err != nil {
				return deliveries, err
			}
			deliveries = append(deliveries, existing)
			continue
		}
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
			IdempotencyKey:         idempotencyKey,
			ProviderResponse:       "{}",
		}
		if err := s.db.WithContext(ctx).Create(&delivery).Error; err != nil {
			if existing, ok, findErr := s.findOutboundByIdempotency(ctx, channel, idempotencyKey); findErr == nil && ok {
				deliveries = append(deliveries, existing)
				continue
			}
			return deliveries, err
		}
		if s.jobs != nil {
			_, err := s.jobs.Enqueue(ctx, jobs.EnqueueInput{
				OrganizationID: &channel.OrganizationID,
				JobType:        jobs.JobTypeChannelOutbound,
				Payload:        map[string]any{"outbound_message_id": delivery.ID.String()},
				IdempotencyKey: "outbound:" + delivery.ID.String(),
				CorrelationID:  delivery.ID.String(),
				MaxAttempts:    5,
			})
			if err != nil {
				return deliveries, err
			}
			deliveries = append(deliveries, delivery)
			continue
		}
		if err := s.SendOutboundNow(ctx, delivery.ID, 1, 5); err != nil {
			return deliveries, err
		}
		_ = s.db.WithContext(ctx).Where("id = ?", delivery.ID).First(&delivery).Error
		deliveries = append(deliveries, delivery)
	}
	return deliveries, nil
}

func (s *Service) ProcessOutboundJob(ctx context.Context, job jobs.Job) error {
	var payload struct {
		OutboundMessageID string `json:"outbound_message_id"`
	}
	if err := json.Unmarshal([]byte(defaultObject(job.Payload)), &payload); err != nil {
		return jobs.PermanentError{Err: err}
	}
	id, err := uuid.Parse(strings.TrimSpace(payload.OutboundMessageID))
	if err != nil {
		return jobs.PermanentError{Err: err}
	}
	return s.SendOutboundNow(ctx, id, job.Attempts+1, job.MaxAttempts)
}

func (s *Service) ProcessNotificationJob(ctx context.Context, job jobs.Job) error {
	var payload struct {
		NotificationID string `json:"notification_id"`
	}
	if err := json.Unmarshal([]byte(defaultObject(job.Payload)), &payload); err != nil {
		return jobs.PermanentError{Err: err}
	}
	notificationID, err := uuid.Parse(strings.TrimSpace(payload.NotificationID))
	if err != nil {
		return jobs.PermanentError{Err: err}
	}
	var notification core.CommerceNotification
	if err := s.db.WithContext(ctx).Where("id = ?", notificationID).First(&notification).Error; err != nil {
		return jobs.PermanentError{Err: err}
	}
	if notification.Status == "sent" || notification.Status == "skipped" {
		return nil
	}
	if notification.ChannelID == nil || strings.TrimSpace(notification.Recipient) == "" {
		now := s.now()
		return s.db.WithContext(ctx).Model(&notification).Updates(map[string]any{"status": "skipped", "error_message": "notification has no channel or recipient", "updated_at": now}).Error
	}
	var channel core.Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", notification.OrganizationID, *notification.ChannelID).First(&channel).Error; err != nil {
		return jobs.PermanentError{Err: err}
	}
	notificationPayload := parseJSONMap(notification.Payload)
	text := strings.TrimSpace(stringValue(notificationPayload["message"]))
	if text == "" {
		text = notification.NotificationType
	}
	outboundPayload := s.channelPayload(channel, notification.Recipient, OutboundMessage{Type: MessageText, Text: text})
	delivery, err := s.notificationDelivery(ctx, notification, channel, outboundPayload)
	if err != nil {
		return err
	}
	if err := s.SendOutboundNow(ctx, delivery.ID, job.Attempts+1, job.MaxAttempts); err != nil {
		now := s.now()
		attempt := job.Attempts + 1
		status := "retry_pending"
		var permanent jobs.PermanentError
		if errors.As(err, &permanent) || (job.MaxAttempts > 0 && attempt >= job.MaxAttempts) {
			status = "failed_permanently"
		}
		_ = s.db.WithContext(ctx).Model(&notification).Updates(map[string]any{"status": status, "attempts": attempt, "error_message": publicSendError(err), "next_attempt_at": now.Add(outboundBackoff(attempt)), "updated_at": now}).Error
		return err
	}
	now := s.now()
	return s.db.WithContext(ctx).Model(&notification).Updates(map[string]any{"status": "sent", "attempts": job.Attempts + 1, "outbound_message_id": delivery.ID, "sent_at": &now, "error_message": "", "updated_at": now}).Error
}

func (s *Service) notificationDelivery(ctx context.Context, notification core.CommerceNotification, channel core.Channel, payload map[string]any) (ChannelOutboundMessage, error) {
	idempotencyKey := "notification:" + notification.ID.String()
	if existing, ok, err := s.findOutboundByIdempotency(ctx, channel, idempotencyKey); err != nil || ok {
		return existing, err
	}
	delivery := ChannelOutboundMessage{
		ID:                     uuid.New(),
		OrganizationID:         notification.OrganizationID,
		ChannelID:              channel.ID,
		SessionID:              nil,
		ExternalConversationID: notification.Recipient,
		Recipient:              notification.Recipient,
		Provider:               channel.Provider,
		MessageType:            MessageText,
		Status:                 OutboundQueued,
		Payload:                jsonValue(payload),
		IdempotencyKey:         idempotencyKey,
		ProviderResponse:       "{}",
	}
	return delivery, s.db.WithContext(ctx).Create(&delivery).Error
}

func (s *Service) SendOutboundNow(ctx context.Context, deliveryID uuid.UUID, attempt, maxAttempts int) error {
	delivery, claimed, err := s.claimOutboundForSend(ctx, deliveryID, attempt)
	if err != nil {
		return jobs.PermanentError{Err: err}
	}
	if !claimed {
		return nil
	}
	var channel core.Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", delivery.OrganizationID, delivery.ChannelID).First(&channel).Error; err != nil {
		return jobs.PermanentError{Err: err}
	}
	sender, ok := s.senders[strings.ToLower(channel.Provider)]
	if !ok {
		now := s.now()
		updates := map[string]any{"status": OutboundSkipped, "error_message": "channel sender is not configured", "updated_at": now}
		_ = s.db.WithContext(ctx).Model(&delivery).Updates(updates).Error
		return jobs.PermanentError{Err: errors.New("channel sender is not configured")}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(defaultObject(delivery.Payload)), &payload); err != nil {
		return jobs.PermanentError{Err: err}
	}
	message := OutboundMessage{Type: delivery.MessageType}
	sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	providerResult, err := sender.Send(sendCtx, channel, delivery.Recipient, message, payload)
	cancel()
	now := s.now()
	if err != nil {
		nextAttempt := now.Add(outboundBackoff(attempt))
		status := OutboundRetryPending
		if maxAttempts > 0 && attempt >= maxAttempts {
			status = OutboundFailedPermanently
		}
		updateErr := s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": status, "error_message": publicSendError(err), "attempts": attempt, "next_attempt_at": &nextAttempt, "updated_at": now}).Error
		if updateErr != nil {
			return updateErr
		}
		s.log.Warn("runtime outbound send failed", "organization_id", channel.OrganizationID, "channel_id", channel.ID, "provider", channel.Provider, "delivery_id", delivery.ID, "error", publicSendError(err))
		return err
	}
	now = s.now()
	return s.db.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": OutboundSent, "attempts": attempt, "provider_message_id": providerResult.ProviderMessageID, "provider_response": jsonValue(providerResult.Response), "sent_at": &now, "next_attempt_at": nil, "error_message": "", "updated_at": now}).Error
}

func (s *Service) claimOutboundForSend(ctx context.Context, deliveryID uuid.UUID, attempt int) (ChannelOutboundMessage, bool, error) {
	now := s.now()
	staleSendingCutoff := now.Add(-15 * time.Minute)
	if s.db.Dialector.Name() == "postgres" {
		var delivery ChannelOutboundMessage
		err := s.db.WithContext(ctx).Raw(`
			UPDATE channel_outbound_messages
			SET status = ?, attempts = ?, error_message = '', updated_at = ?
			WHERE id = ?
			  AND provider_message_id = ''
			  AND (
			    status IN (?, ?, ?)
			    OR (status = ? AND updated_at <= ?)
			  )
			RETURNING *
		`, OutboundSending, attempt, now, deliveryID, OutboundQueued, OutboundFailed, OutboundRetryPending, OutboundSending, staleSendingCutoff).Scan(&delivery).Error
		if err != nil {
			return ChannelOutboundMessage{}, false, err
		}
		if delivery.ID == uuid.Nil {
			return ChannelOutboundMessage{}, false, nil
		}
		return delivery, true, nil
	}
	var delivery ChannelOutboundMessage
	if err := s.db.WithContext(ctx).Where("id = ?", deliveryID).First(&delivery).Error; err != nil {
		return ChannelOutboundMessage{}, false, err
	}
	if delivery.ProviderMessageID != "" || !outboundClaimable(delivery, staleSendingCutoff) {
		return ChannelOutboundMessage{}, false, nil
	}
	result := s.db.WithContext(ctx).
		Model(&ChannelOutboundMessage{}).
		Where("id = ? AND provider_message_id = '' AND (status IN ? OR (status = ? AND updated_at <= ?))", deliveryID, []string{OutboundQueued, OutboundFailed, OutboundRetryPending}, OutboundSending, staleSendingCutoff).
		Updates(map[string]any{"status": OutboundSending, "attempts": attempt, "error_message": "", "updated_at": now})
	if result.Error != nil {
		return ChannelOutboundMessage{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		return ChannelOutboundMessage{}, false, nil
	}
	delivery.Status = OutboundSending
	delivery.Attempts = attempt
	delivery.ErrorMessage = ""
	delivery.UpdatedAt = now
	return delivery, true, nil
}

func outboundClaimable(delivery ChannelOutboundMessage, staleSendingCutoff time.Time) bool {
	switch delivery.Status {
	case OutboundQueued, OutboundFailed, OutboundRetryPending:
		return true
	case OutboundSending:
		return !delivery.UpdatedAt.After(staleSendingCutoff)
	default:
		return false
	}
}

func (s *Service) findOutboundByIdempotency(ctx context.Context, channel core.Channel, idempotencyKey string) (ChannelOutboundMessage, bool, error) {
	var delivery ChannelOutboundMessage
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_id = ? AND idempotency_key = ?", channel.OrganizationID, channel.ID, idempotencyKey).First(&delivery).Error
	if err == nil {
		return delivery, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ChannelOutboundMessage{}, false, nil
	}
	return ChannelOutboundMessage{}, false, err
}

func outboundIdempotencyKey(input InboundMessage, result RuntimeResult, index int) string {
	return strings.Join([]string{input.ExternalMessageID, result.ConversationID.String(), strconv.Itoa(index)}, ":")
}

func outboundBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1<<min(attempt-1, 5)) * 30 * time.Second
	if delay > 30*time.Minute {
		return 30 * time.Minute
	}
	return delay
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
	mu        sync.Mutex
	Delay     time.Duration
	Responses []ProviderSendResult
	Errors    []error
	Sent      []map[string]any
}

func (m *MockChannelSender) Send(ctx context.Context, _ core.Channel, recipient string, _ OutboundMessage, payload map[string]any) (ProviderSendResult, error) {
	if m.Delay > 0 {
		select {
		case <-ctx.Done():
			return ProviderSendResult{}, ctx.Err()
		case <-time.After(m.Delay):
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
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
