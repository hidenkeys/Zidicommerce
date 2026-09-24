package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

type MessagingAdapter struct {
	provider     string
	service      *Service
	platform     *channelplatform.Service
	graphBaseURL string
	graphVersion string
	appSecret    string
	client       HTTPClient
	now          func() time.Time
}

func NewMessagingAdapter(provider string, service *Service, platform *channelplatform.Service, graphBaseURL, graphVersion, appSecret string, client HTTPClient) *MessagingAdapter {
	provider = normalize(provider)
	graphBaseURL = strings.TrimRight(strings.TrimSpace(graphBaseURL), "/")
	if graphBaseURL == "" {
		if provider == ProviderInstagram {
			graphBaseURL = "https://graph.instagram.com"
		} else {
			graphBaseURL = "https://graph.facebook.com"
		}
	}
	graphVersion = strings.Trim(strings.TrimSpace(graphVersion), "/")
	if graphVersion == "" {
		graphVersion = "v24.0"
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &MessagingAdapter{provider: provider, service: service, platform: platform, graphBaseURL: graphBaseURL, graphVersion: graphVersion, appSecret: strings.TrimSpace(appSecret), client: client, now: func() time.Time { return time.Now().UTC() }}
}

func (a *MessagingAdapter) Provider() string { return a.provider }

func (a *MessagingAdapter) VerifySignature(_ context.Context, envelope channelplatform.ProviderEnvelope) error {
	if a.appSecret == "" {
		return errors.New("Meta webhook app secret is not configured")
	}
	signature := strings.TrimSpace(headerValue(envelope.Headers, "x-hub-signature-256"))
	if !strings.HasPrefix(signature, "sha256=") {
		return errors.New("Meta webhook signature is missing or malformed")
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return errors.New("Meta webhook signature is malformed")
	}
	mac := hmac.New(sha256.New, []byte(a.appSecret))
	_, _ = mac.Write(envelope.RawPayload)
	if !hmac.Equal(mac.Sum(nil), provided) {
		return errors.New("Meta webhook signature verification failed")
	}
	return nil
}

func (a *MessagingAdapter) NormalizeInbound(ctx context.Context, envelope channelplatform.ProviderEnvelope) ([]channelplatform.InboundEvent, error) {
	resolved, err := a.service.ResolveConnection(ctx, uuid.Nil, envelope.ConnectionID, a.provider)
	if err != nil {
		return nil, err
	}
	payload, err := decodeWebhook(envelope.RawPayload)
	if err != nil {
		return nil, err
	}
	events := []channelplatform.InboundEvent{}
	for _, entry := range payload.Entry {
		if entry.ID != resolved.Identity.ProviderIdentityID {
			continue
		}
		for _, messaging := range entry.Messaging {
			if event, ok := normalizeInboundEvent(a.provider, envelope.ConnectionID, entry, messaging, envelope.ReceivedAt); ok {
				events = append(events, event)
			}
		}
	}
	return events, nil
}

func (a *MessagingAdapter) UnsupportedInbound(ctx context.Context, envelope channelplatform.ProviderEnvelope) ([]UnsupportedEvent, error) {
	resolved, err := a.service.ResolveConnection(ctx, uuid.Nil, envelope.ConnectionID, a.provider)
	if err != nil {
		return nil, err
	}
	payload, err := decodeWebhook(envelope.RawPayload)
	if err != nil {
		return nil, err
	}
	unsupported := []UnsupportedEvent{}
	for _, entry := range payload.Entry {
		if entry.ID != resolved.Identity.ProviderIdentityID {
			continue
		}
		for _, messaging := range entry.Messaging {
			if _, ok := normalizeInboundEvent(a.provider, envelope.ConnectionID, entry, messaging, envelope.ReceivedAt); ok || messaging.Delivery != nil || messaging.Read != nil {
				continue
			}
			unsupported = append(unsupported, UnsupportedEvent{ProviderEventID: messagingEventID(entry, messaging, "unsupported"), ExternalCustomerID: messaging.Sender.ID, MessageType: "unsupported"})
		}
	}
	return unsupported, nil
}

func (a *MessagingAdapter) BuildOutbound(_ context.Context, command channelplatform.OutboundCommand) (channelplatform.ProviderRequest, error) {
	if command.ChannelConnectionID == uuid.Nil || strings.TrimSpace(command.ExternalCustomerID) == "" || strings.TrimSpace(command.IdempotencyKey) == "" {
		return channelplatform.ProviderRequest{}, errors.New("Meta outbound command is incomplete")
	}
	if command.Template != nil {
		return channelplatform.ProviderRequest{}, errors.New("WhatsApp templates are not supported on Meta social messaging channels")
	}
	message := map[string]any{}
	messageType := normalize(command.MessageType)
	switch {
	case len(command.Media) > 0:
		media := command.Media[0]
		mediaType := normalize(media.Type)
		if mediaType == "document" {
			mediaType = "file"
		}
		if mediaType != "image" && mediaType != "video" && mediaType != "audio" && mediaType != "file" {
			return channelplatform.ProviderRequest{}, errors.New("Meta media type is not supported")
		}
		message["attachment"] = map[string]any{"type": mediaType, "payload": map[string]any{"url": strings.TrimSpace(media.Reference), "is_reusable": true}}
	case len(command.Actions) > 0 || messageType == "buttons":
		if strings.TrimSpace(command.Body) == "" {
			return channelplatform.ProviderRequest{}, errors.New("Meta quick replies require message text")
		}
		quickReplies := make([]map[string]any, 0, len(command.Actions))
		for _, action := range command.Actions {
			quickReplies = append(quickReplies, map[string]any{"content_type": "text", "title": action.Label, "payload": action.ID})
		}
		message["text"] = command.Body
		message["quick_replies"] = quickReplies
	default:
		if strings.TrimSpace(command.Body) == "" {
			return channelplatform.ProviderRequest{}, errors.New("Meta text message cannot be empty")
		}
		message["text"] = command.Body
	}
	payload := map[string]any{"recipient": map[string]any{"id": strings.TrimSpace(command.ExternalCustomerID)}, "message": message, "messaging_type": "RESPONSE"}
	return channelplatform.ProviderRequest{Provider: a.provider, ConnectionID: command.ChannelConnectionID, Payload: payload}, nil
}

func (a *MessagingAdapter) NormalizeDelivery(ctx context.Context, envelope channelplatform.ProviderEnvelope) ([]channelplatform.DeliveryUpdate, error) {
	resolved, err := a.service.ResolveConnection(ctx, uuid.Nil, envelope.ConnectionID, a.provider)
	if err != nil {
		return nil, err
	}
	payload, err := decodeWebhook(envelope.RawPayload)
	if err != nil {
		return nil, err
	}
	updates := []channelplatform.DeliveryUpdate{}
	for _, entry := range payload.Entry {
		if entry.ID != resolved.Identity.ProviderIdentityID {
			continue
		}
		for _, messaging := range entry.Messaging {
			occurredAt := metaTimestamp(messaging.Timestamp, entry.Time, envelope.ReceivedAt)
			if messaging.Delivery != nil {
				for _, messageID := range messaging.Delivery.MessageIDs {
					updates = append(updates, channelplatform.DeliveryUpdate{ProviderMessageID: messageID, Status: "delivered", OccurredAt: occurredAt})
				}
			}
			if messaging.Read != nil {
				updates = append(updates, channelplatform.DeliveryUpdate{ProviderMessageID: "watermark:" + strconv.FormatInt(messaging.Read.Watermark, 10), Status: "read", OccurredAt: occurredAt})
			}
		}
	}
	return updates, nil
}

func (a *MessagingAdapter) Send(ctx context.Context, command channelplatform.OutboundCommand) (channelplatform.SendResult, error) {
	resolved, err := a.service.ResolveForSend(ctx, commandMetadataOrganizationID(command), command.ChannelConnectionID, a.provider)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	if err := a.service.RequireReplyWindow(ctx, resolved, command.ExternalCustomerID); err != nil {
		return channelplatform.SendResult{}, err
	}
	requestBody, err := a.BuildOutbound(ctx, command)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	body, err := json.Marshal(requestBody.Payload)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	endpoint := a.graphBaseURL + "/" + a.graphVersion + "/" + url.PathEscape(resolved.Identity.ProviderIdentityID) + "/messages"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+resolved.Token)
	request.Header.Set("Content-Type", "application/json")
	started := a.now()
	response, err := a.client.Do(request)
	latency := a.now().Sub(started).Milliseconds()
	if err != nil {
		a.recordSendFailure(ctx, resolved, command, 0, latency)
		return channelplatform.SendResult{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	var decoded struct {
		MessageID string `json:"message_id"`
		Recipient string `json:"recipient_id"`
		Error     struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(responseBody, &decoded)
	metadata := map[string]any{"status_code": response.StatusCode, "recipient_confirmed": decoded.Recipient != ""}
	if decoded.Error.Code != 0 {
		metadata["error_code"] = decoded.Error.Code
	}
	result := channelplatform.SendResult{ProviderMessageID: decoded.MessageID, StatusCode: response.StatusCode, ResponseMetadata: metadata}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		a.recordSendFailure(ctx, resolved, command, response.StatusCode, latency)
		return result, MetaProviderError{Provider: a.provider, StatusCode: response.StatusCode, Code: decoded.Error.Code}
	}
	_ = a.service.MarkOutbound(ctx, resolved, "")
	_, _, _ = a.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: a.provider, EventType: "outbound_send", ProviderEventID: first(decoded.MessageID, "outbound:"+command.IdempotencyKey), IdempotencyKey: a.provider + ":outbound:" + command.IdempotencyKey, NormalizedStatus: "sent", PayloadMetadata: metadata})
	_, _ = a.platform.IncrementMetric(ctx, channelplatform.MetricInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, OutboundCount: 1, AverageResponseMS: latency, Metadata: map[string]any{"provider": a.provider}})
	_, _ = a.platform.RecordHealthCheck(ctx, resolved.Connection.OrganizationID, resolved.Connection.ID, channelplatform.HealthCheckInput{Status: channelplatform.HealthHealthy, LatencyMS: latency, Metadata: `{"check_type":"outbound_send"}`})
	return result, nil
}

func (a *MessagingAdapter) recordSendFailure(ctx context.Context, resolved ResolvedConnection, command channelplatform.OutboundCommand, statusCode int, latency int64) {
	message := "Meta provider request failed"
	_ = a.service.MarkOutbound(ctx, resolved, message)
	providerEventID := "outbound-failed:" + command.IdempotencyKey
	_, _, _ = a.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: a.provider, EventType: "outbound_send", ProviderEventID: providerEventID, IdempotencyKey: a.provider + ":" + providerEventID, NormalizedStatus: "failed", PayloadMetadata: map[string]any{"status_code": statusCode}, ProcessingError: message})
	_, _ = a.platform.IncrementMetric(ctx, channelplatform.MetricInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, FailedOutboundCount: 1, ProviderErrorCount: 1, AverageResponseMS: latency, Metadata: map[string]any{"provider": a.provider, "status_code": statusCode}})
}

type UnsupportedEvent struct {
	ProviderEventID    string
	ExternalCustomerID string
	MessageType        string
}

type MetaProviderError struct {
	Provider   string
	StatusCode int
	Code       int
}

func (e MetaProviderError) Error() string {
	if e.StatusCode == http.StatusTooManyRequests {
		return "Meta provider rate limit reached"
	}
	if e.Code != 0 {
		return fmt.Sprintf("Meta provider error %d", e.Code)
	}
	return fmt.Sprintf("Meta provider request failed with status %d", e.StatusCode)
}

func commandMetadataOrganizationID(command channelplatform.OutboundCommand) uuid.UUID {
	if value, ok := command.Metadata["organization_id"].(string); ok {
		id, _ := uuid.Parse(value)
		return id
	}
	return uuid.Nil
}
