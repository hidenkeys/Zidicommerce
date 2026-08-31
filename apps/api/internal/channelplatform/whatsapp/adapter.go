package whatsapp

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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Adapter struct {
	service         *Service
	platform        *channelplatform.Service
	graphBaseURL    string
	client          HTTPClient
	signatureBypass bool
	now             func() time.Time
}

func NewAdapter(service *Service, platform *channelplatform.Service, graphBaseURL string, client HTTPClient, signatureBypass bool) *Adapter {
	graphBaseURL = strings.TrimRight(strings.TrimSpace(graphBaseURL), "/")
	if graphBaseURL == "" {
		graphBaseURL = "https://graph.facebook.com"
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &Adapter{service: service, platform: platform, graphBaseURL: graphBaseURL, client: client, signatureBypass: signatureBypass, now: func() time.Time { return time.Now().UTC() }}
}

func (a *Adapter) Provider() string { return Provider }

func (a *Adapter) VerifySignature(ctx context.Context, envelope channelplatform.ProviderEnvelope) error {
	if a.signatureBypass {
		return nil
	}
	configuration, err := a.configurationByConnection(ctx, envelope.ConnectionID)
	if err != nil {
		return err
	}
	secret, err := a.service.ResolveCredential(ctx, configuration.OrganizationID, configuration.ConnectionID, CredentialAppSecret)
	if err != nil || strings.TrimSpace(secret) == "" {
		return errors.New("WhatsApp app secret is not configured")
	}
	signature := strings.TrimSpace(headerValue(envelope.Headers, "x-hub-signature-256"))
	if !strings.HasPrefix(signature, "sha256=") {
		return errors.New("WhatsApp signature is missing or malformed")
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return errors.New("WhatsApp signature is malformed")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(envelope.RawPayload)
	if !hmac.Equal(mac.Sum(nil), provided) {
		return errors.New("WhatsApp signature verification failed")
	}
	return a.service.MarkSignatureVerified(ctx, configuration)
}

func (a *Adapter) NormalizeInbound(ctx context.Context, envelope channelplatform.ProviderEnvelope) ([]channelplatform.InboundEvent, error) {
	configuration, err := a.configurationByConnection(ctx, envelope.ConnectionID)
	if err != nil {
		return nil, err
	}
	payload, err := decodeWebhook(envelope.RawPayload)
	if err != nil {
		return nil, err
	}
	events := []channelplatform.InboundEvent{}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Value.Metadata.PhoneNumberID != configuration.PhoneNumberID {
				continue
			}
			for _, message := range change.Value.Messages {
				event, supported := normalizeMessage(message, entry.ID, change.Value.Metadata, envelope.ConnectionID, envelope.ReceivedAt)
				if supported {
					events = append(events, event)
				}
			}
		}
	}
	return events, nil
}

func (a *Adapter) BuildOutbound(_ context.Context, command channelplatform.OutboundCommand) (channelplatform.ProviderRequest, error) {
	if command.ChannelConnectionID == uuid.Nil || strings.TrimSpace(command.ExternalCustomerID) == "" || strings.TrimSpace(command.MessageType) == "" {
		return channelplatform.ProviderRequest{}, errors.New("WhatsApp outbound command is incomplete")
	}
	payload := map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": strings.TrimSpace(command.ExternalCustomerID)}
	if command.Template != nil {
		if command.Template.ID == uuid.Nil || strings.TrimSpace(command.Template.Name) == "" || strings.TrimSpace(command.Template.Language) == "" {
			return channelplatform.ProviderRequest{}, errors.New("WhatsApp template command is incomplete")
		}
		parameters := make([]map[string]any, 0, len(command.Template.VariableOrder))
		for _, name := range command.Template.VariableOrder {
			value := strings.TrimSpace(command.Template.Variables[name])
			if value == "" {
				return channelplatform.ProviderRequest{}, errors.New("WhatsApp template variables are incomplete")
			}
			parameters = append(parameters, map[string]any{"type": "text", "text": value})
		}
		components := []map[string]any{}
		if len(parameters) > 0 {
			components = append(components, map[string]any{"type": "body", "parameters": parameters})
		}
		payload["type"] = "template"
		payload["template"] = map[string]any{"name": command.Template.Name, "language": map[string]any{"code": command.Template.Language}, "components": components}
		return channelplatform.ProviderRequest{Provider: Provider, ConnectionID: command.ChannelConnectionID, Payload: payload}, nil
	}
	switch strings.ToLower(strings.TrimSpace(command.MessageType)) {
	case "buttons":
		buttons := make([]map[string]any, 0, len(command.Actions))
		for _, action := range command.Actions {
			buttons = append(buttons, map[string]any{"type": "reply", "reply": map[string]any{"id": action.ID, "title": action.Label}})
		}
		payload["type"] = "interactive"
		payload["interactive"] = map[string]any{"type": "button", "body": map[string]any{"text": command.Body}, "action": map[string]any{"buttons": buttons}}
	case "list":
		rows := make([]map[string]any, 0, len(command.Actions))
		for _, action := range command.Actions {
			rows = append(rows, map[string]any{"id": action.ID, "title": action.Label, "description": action.Description})
		}
		payload["type"] = "interactive"
		payload["interactive"] = map[string]any{"type": "list", "body": map[string]any{"text": command.Body}, "action": map[string]any{"button": "Choose", "sections": []map[string]any{{"title": "Options", "rows": rows}}}}
	case "image", "document", "audio", "video":
		mediaType := strings.ToLower(strings.TrimSpace(command.MessageType))
		if len(command.Media) == 0 || strings.TrimSpace(command.Media[0].Reference) == "" {
			return channelplatform.ProviderRequest{}, errors.New("WhatsApp media command requires a media reference")
		}
		media := map[string]any{"link": command.Media[0].Reference}
		if command.Body != "" && mediaType != "audio" {
			media["caption"] = command.Body
		}
		payload["type"] = mediaType
		payload[mediaType] = media
	default:
		payload["type"] = "text"
		payload["text"] = map[string]any{"body": command.Body}
	}
	return channelplatform.ProviderRequest{Provider: Provider, ConnectionID: command.ChannelConnectionID, Payload: payload}, nil
}

func (a *Adapter) NormalizeDelivery(ctx context.Context, envelope channelplatform.ProviderEnvelope) ([]channelplatform.DeliveryUpdate, error) {
	configuration, err := a.configurationByConnection(ctx, envelope.ConnectionID)
	if err != nil {
		return nil, err
	}
	payload, err := decodeWebhook(envelope.RawPayload)
	if err != nil {
		return nil, err
	}
	updates := []channelplatform.DeliveryUpdate{}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Value.Metadata.PhoneNumberID != configuration.PhoneNumberID {
				continue
			}
			for _, status := range change.Value.Statuses {
				occurredAt := unixTimestamp(status.Timestamp, envelope.ReceivedAt)
				update := channelplatform.DeliveryUpdate{ProviderMessageID: status.ID, Status: strings.ToLower(strings.TrimSpace(status.Status)), OccurredAt: occurredAt}
				if len(status.Errors) > 0 {
					update.ErrorCode = strconv.FormatInt(status.Errors[0].Code, 10)
					update.ErrorMessage = strings.TrimSpace(status.Errors[0].Title)
				}
				updates = append(updates, update)
			}
		}
	}
	return updates, nil
}

func (a *Adapter) UnsupportedInbound(ctx context.Context, envelope channelplatform.ProviderEnvelope) ([]UnsupportedMessage, error) {
	configuration, err := a.configurationByConnection(ctx, envelope.ConnectionID)
	if err != nil {
		return nil, err
	}
	payload, err := decodeWebhook(envelope.RawPayload)
	if err != nil {
		return nil, err
	}
	unsupported := []UnsupportedMessage{}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Value.Metadata.PhoneNumberID != configuration.PhoneNumberID {
				continue
			}
			for _, message := range change.Value.Messages {
				if _, supported := normalizeMessage(message, entry.ID, change.Value.Metadata, envelope.ConnectionID, envelope.ReceivedAt); !supported {
					unsupported = append(unsupported, UnsupportedMessage{ProviderEventID: message.ID, MessageType: message.Type, ExternalCustomerID: message.From})
				}
			}
		}
	}
	return unsupported, nil
}

func (a *Adapter) Send(ctx context.Context, command channelplatform.OutboundCommand) (channelplatform.SendResult, error) {
	configuration, err := a.configurationByConnection(ctx, command.ChannelConnectionID)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	decision, template, err := a.service.EvaluateAndRecordPolicy(ctx, configuration, command)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	if !decision.Allowed {
		return channelplatform.SendResult{}, PolicyBlockedError{Decision: decision}
	}
	if template != nil && command.Template != nil {
		view := templateView(*template)
		command.Template.Name = template.Name
		command.Template.Language = template.Language
		command.Template.VariableOrder = view.VariableSchema
	}
	if strings.TrimSpace(configuration.PhoneNumberID) == "" {
		return channelplatform.SendResult{}, errors.New("WhatsApp phone number ID is not configured")
	}
	token, err := a.service.ResolveCredential(ctx, configuration.OrganizationID, configuration.ConnectionID, CredentialAccessToken)
	if err != nil || strings.TrimSpace(token) == "" {
		return channelplatform.SendResult{}, errors.New("WhatsApp access token is not configured")
	}
	request, err := a.BuildOutbound(ctx, command)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	body, err := json.Marshal(request.Payload)
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	endpoint := fmt.Sprintf("%s/%s/%s/messages", a.graphBaseURL, configuration.GraphAPIVersion, configuration.PhoneNumberID)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return channelplatform.SendResult{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	started := a.now()
	response, err := a.client.Do(httpRequest)
	latency := a.now().Sub(started).Milliseconds()
	if err != nil {
		a.recordSendFailure(ctx, configuration, command, 0, nil, latency, err)
		return channelplatform.SendResult{}, err
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		a.recordSendFailure(ctx, configuration, command, response.StatusCode, nil, latency, readErr)
		return channelplatform.SendResult{}, readErr
	}
	safeMetadata, providerMessageID, providerError := decodeMetaResponse(response.StatusCode, responseBody)
	result := channelplatform.SendResult{ProviderMessageID: providerMessageID, StatusCode: response.StatusCode, ResponseMetadata: safeMetadata}
	if response.StatusCode >= 400 {
		providerError.RetryAfter = retryAfter(response.Header.Get("Retry-After"), a.now())
		a.recordSendFailure(ctx, configuration, command, response.StatusCode, safeMetadata, latency, providerError)
		return result, providerError
	}
	_ = a.service.MarkOutbound(ctx, configuration, false, nil)
	_ = a.service.RecordOutboundContact(ctx, configuration, command.ExternalCustomerID)
	operationalMetric := OperationalMetricDaily{FreeformSendCount: 1}
	if command.Template != nil {
		operationalMetric = OperationalMetricDaily{TemplateSendCount: 1}
	}
	_ = a.service.incrementOperationalMetric(ctx, configuration.OrganizationID, configuration.ConnectionID, operationalMetric)
	_, _, _ = a.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider, EventType: "outbound_send", ProviderEventID: firstNonEmpty(providerMessageID, "outbound:"+command.IdempotencyKey), IdempotencyKey: "whatsapp:outbound:" + command.IdempotencyKey, NormalizedStatus: "sent", PayloadMetadata: safeMetadata})
	_, _ = a.platform.IncrementMetric(ctx, channelplatform.MetricInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, OutboundCount: 1, AverageResponseMS: latency, Metadata: map[string]any{"provider": Provider}})
	_, _ = a.platform.RecordHealthCheck(ctx, configuration.OrganizationID, configuration.ConnectionID, channelplatform.HealthCheckInput{Status: channelplatform.HealthHealthy, LatencyMS: latency, Metadata: jsonValue(map[string]any{"check_type": "outbound_send"})})
	return result, nil
}

func (a *Adapter) recordSendFailure(ctx context.Context, configuration Configuration, command channelplatform.OutboundCommand, statusCode int, metadata map[string]any, latency int64, sendErr error) {
	var rateLimitedUntil *time.Time
	if providerErr, ok := sendErr.(ProviderError); ok && providerErr.RetryAfter != nil {
		rateLimitedUntil = providerErr.RetryAfter
	}
	_ = a.service.MarkOutbound(ctx, configuration, true, rateLimitedUntil)
	operationalMetric := OperationalMetricDaily{}
	if statusCode == http.StatusTooManyRequests {
		operationalMetric.RateLimitCount = 1
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		operationalMetric.InvalidCredentialCount = 1
	}
	if statusCode == http.StatusBadRequest {
		operationalMetric.InvalidRecipientCount = 1
	}
	_ = a.service.incrementOperationalMetric(ctx, configuration.OrganizationID, configuration.ConnectionID, operationalMetric)
	providerEventID := "outbound-failed:" + command.IdempotencyKey
	_, _, _ = a.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider, EventType: "outbound_send", ProviderEventID: providerEventID, IdempotencyKey: "whatsapp:" + providerEventID, NormalizedStatus: "failed", PayloadMetadata: metadata, ProcessingError: publicProviderError(sendErr)})
	_, _ = a.platform.IncrementMetric(ctx, channelplatform.MetricInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, FailedOutboundCount: 1, ProviderErrorCount: 1, AverageResponseMS: latency, Metadata: map[string]any{"provider": Provider, "status_code": statusCode}})
	_, _ = a.platform.RecordHealthCheck(ctx, configuration.OrganizationID, configuration.ConnectionID, channelplatform.HealthCheckInput{Status: channelplatform.HealthDegraded, LatencyMS: latency, ErrorCode: strconv.Itoa(statusCode), ErrorMessage: publicProviderError(sendErr), Metadata: jsonValue(map[string]any{"check_type": "outbound_send"})})
}

func (a *Adapter) configurationByConnection(ctx context.Context, connectionID uuid.UUID) (Configuration, error) {
	var configuration Configuration
	if err := a.service.db.WithContext(ctx).Where("channel_connection_id = ?", connectionID).First(&configuration).Error; err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}

type UnsupportedMessage struct {
	ProviderEventID    string
	MessageType        string
	ExternalCustomerID string
}

type ProviderError struct {
	StatusCode int
	Code       int64
	Message    string
	RetryAfter *time.Time
}

func (e ProviderError) Error() string {
	if e.StatusCode == http.StatusTooManyRequests {
		return "WhatsApp provider rate limit reached"
	}
	if e.Code != 0 {
		return fmt.Sprintf("WhatsApp provider error %d", e.Code)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("WhatsApp provider request failed with status %d", e.StatusCode)
	}
	return "WhatsApp provider request failed"
}

type webhookPayload struct {
	Object string         `json:"object"`
	Entry  []webhookEntry `json:"entry"`
}

type webhookEntry struct {
	ID      string          `json:"id"`
	Changes []webhookChange `json:"changes"`
}

type webhookChange struct {
	Field string       `json:"field"`
	Value webhookValue `json:"value"`
}

type webhookValue struct {
	Metadata struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		PhoneNumberID      string `json:"phone_number_id"`
	} `json:"metadata"`
	Messages []webhookMessage `json:"messages"`
	Statuses []webhookStatus  `json:"statuses"`
}

type webhookMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Button struct {
		Payload string `json:"payload"`
		Text    string `json:"text"`
	} `json:"button"`
	Interactive struct {
		Type        string `json:"type"`
		ButtonReply struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"button_reply"`
		ListReply struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"list_reply"`
	} `json:"interactive"`
	Image    webhookMedia `json:"image"`
	Document webhookMedia `json:"document"`
	Audio    webhookMedia `json:"audio"`
	Video    webhookMedia `json:"video"`
}

type webhookMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
}

type webhookStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Recipient string `json:"recipient_id"`
	Errors    []struct {
		Code  int64  `json:"code"`
		Title string `json:"title"`
	} `json:"errors"`
}

func decodeWebhook(body []byte) (webhookPayload, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookPayload{}, errors.New("invalid WhatsApp webhook payload")
	}
	if payload.Object != "whatsapp_business_account" {
		return webhookPayload{}, errors.New("unsupported WhatsApp webhook object")
	}
	return payload, nil
}

func normalizeMessage(message webhookMessage, entryID string, metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}, connectionID uuid.UUID, fallback time.Time) (channelplatform.InboundEvent, bool) {
	messageType := strings.ToLower(strings.TrimSpace(message.Type))
	body := ""
	media := []channelplatform.MediaReference{}
	safeMetadata := map[string]any{"source": Provider, "message_type": messageType, "waba_entry_id": entryID, "phone_number_id": metadata.PhoneNumberID}
	switch messageType {
	case "text":
		body = message.Text.Body
	case "interactive":
		if message.Interactive.ButtonReply.ID != "" {
			body = message.Interactive.ButtonReply.ID
			safeMetadata["reply_title"] = message.Interactive.ButtonReply.Title
			safeMetadata["interactive_type"] = "button_reply"
		} else if message.Interactive.ListReply.ID != "" {
			body = message.Interactive.ListReply.ID
			safeMetadata["reply_title"] = message.Interactive.ListReply.Title
			safeMetadata["interactive_type"] = "list_reply"
		} else {
			return channelplatform.InboundEvent{}, false
		}
	case "button":
		body = firstNonEmpty(message.Button.Payload, message.Button.Text)
	case "image", "document", "audio", "video":
		item := mediaForMessage(message, messageType)
		if item.ID == "" {
			return channelplatform.InboundEvent{}, false
		}
		body = item.Caption
		media = append(media, channelplatform.MediaReference{Type: messageType, Reference: "meta-media://" + item.ID, ContentType: item.MimeType})
		if item.Filename != "" {
			safeMetadata["filename"] = item.Filename
		}
	default:
		return channelplatform.InboundEvent{}, false
	}
	timestamp := unixTimestamp(message.Timestamp, fallback)
	return channelplatform.InboundEvent{Provider: Provider, ChannelConnectionID: connectionID, ProviderEventID: message.ID, ExternalCustomerID: message.From, ExternalMessageID: message.ID, ExternalConversationID: message.From, MessageType: messageType, Body: body, Media: media, Timestamp: timestamp, Metadata: safeMetadata}, true
}

func mediaForMessage(message webhookMessage, messageType string) webhookMedia {
	switch messageType {
	case "image":
		return message.Image
	case "document":
		return message.Document
	case "audio":
		return message.Audio
	case "video":
		return message.Video
	default:
		return webhookMedia{}
	}
}

func unixTimestamp(value string, fallback time.Time) time.Time {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err == nil && seconds > 0 {
		return time.Unix(seconds, 0).UTC()
	}
	if fallback.IsZero() {
		return time.Now().UTC()
	}
	return fallback.UTC()
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func decodeMetaResponse(statusCode int, body []byte) (map[string]any, string, ProviderError) {
	var payload struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int64  `json:"code"`
			Subcode int64  `json:"error_subcode"`
			TraceID string `json:"fbtrace_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)
	metadata := map[string]any{"status_code": statusCode}
	providerMessageID := ""
	if len(payload.Messages) > 0 {
		providerMessageID = payload.Messages[0].ID
		metadata["provider_message_id"] = providerMessageID
	}
	providerErr := ProviderError{StatusCode: statusCode}
	if payload.Error != nil {
		providerErr.Code = payload.Error.Code
		providerErr.Message = payload.Error.Message
		metadata["error_code"] = payload.Error.Code
		metadata["error_subcode"] = payload.Error.Subcode
		metadata["error_type"] = payload.Error.Type
		metadata["trace_id"] = payload.Error.TraceID
	}
	return metadata, providerMessageID, providerErr
}

func retryAfter(value string, now time.Time) *time.Time {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return nil
	}
	result := now.Add(time.Duration(seconds) * time.Second)
	return &result
}

func publicProviderError(err error) string {
	if err == nil {
		return ""
	}
	if providerErr, ok := err.(ProviderError); ok {
		return providerErr.Error()
	}
	return "WhatsApp provider request failed"
}
