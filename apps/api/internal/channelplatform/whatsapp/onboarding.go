package whatsapp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

const webhookPath = "/v1/runtime/webhooks/whatsapp"

type TestMessageSender interface {
	Send(context.Context, channelplatform.OutboundCommand) (channelplatform.SendResult, error)
}

func (s *Service) ConfigureWebhookPublicBaseURL(baseURL string) {
	s.publicURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func (s *Service) WebhookCallbackURL(connectionID uuid.UUID) string {
	values := url.Values{}
	values.Set("connection_id", connectionID.String())
	return s.publicURL + webhookPath + "?" + values.Encode()
}

func (s *Service) ResolveVerifyTokenForConnection(ctx context.Context, connectionID uuid.UUID, supplied string) (Configuration, channelplatform.ChannelConnection, bool) {
	if connectionID == uuid.Nil {
		return s.ResolveVerifyToken(ctx, supplied)
	}
	configuration, connection, err := s.configurationAndConnection(ctx, connectionID)
	if err != nil {
		return Configuration{}, channelplatform.ChannelConnection{}, false
	}
	secret, err := s.resolveCredential(ctx, configuration.OrganizationID, connectionID, CredentialVerifyToken)
	if err != nil || !constantTimeEqual(secret, supplied) {
		return configuration, connection, false
	}
	return configuration, connection, true
}

func (s *Service) RecordWebhookVerificationFailure(ctx context.Context, connectionID uuid.UUID, reason string) {
	if connectionID == uuid.Nil {
		return
	}
	configuration, _, err := s.configurationAndConnection(ctx, connectionID)
	if err != nil {
		return
	}
	now := s.now()
	updates := map[string]any{
		"last_webhook_verification_attempt_at": now,
		"last_webhook_verification_failed_at":  now,
		"last_webhook_verification_error":      safeVerificationReason(reason),
		"updated_at":                           now,
	}
	if configuration.WebhookStatus != WebhookVerified {
		updates["webhook_status"] = WebhookFailed
		updates["setup_state"] = SetupWebhookFailed
	}
	_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, connectionID).Updates(updates).Error; err != nil {
			return err
		}
		return audit(tx, &configuration.OrganizationID, nil, connectionID, "whatsapp_webhook_verification_failed", map[string]any{"reason": safeVerificationReason(reason)})
	})
	_, _, _ = s.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{
		OrganizationID: configuration.OrganizationID, ConnectionID: connectionID, Provider: Provider,
		EventType: "webhook_verification", ProviderEventID: "verification-failed:" + uuid.NewString(),
		IdempotencyKey: "whatsapp:verification-failed:" + uuid.NewString(), NormalizedStatus: "failed",
		ProcessingError: "Webhook verification failed", ReceivedAt: now,
	})
}

func (s *Service) MarkSignatureVerified(ctx context.Context, configuration Configuration) error {
	now := s.now()
	connectionReady := s.connectionReady(ctx, configuration)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Configuration{}).
			Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).
			Updates(map[string]any{"last_signature_verified_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if connectionReady {
			_, err := promoteReadyRecords(tx, configuration, now)
			return err
		}
		return nil
	})
}

func (s *Service) MarkSignatureRejected(ctx context.Context, configuration Configuration) error {
	now := s.now()
	return s.db.WithContext(ctx).Model(&Configuration{}).
		Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).
		Updates(map[string]any{"last_signature_rejected_at": now, "updated_at": now}).Error
}

func (s *Service) RotateCredential(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input CredentialRotationInput) (CredentialRotationResult, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return CredentialRotationResult{}, httperror.Forbidden("You do not have permission to rotate WhatsApp credentials")
	}
	credentialType := strings.ToLower(strings.TrimSpace(input.CredentialType))
	if !knownCredentialType(credentialType) {
		return CredentialRotationResult{}, httperror.BadRequest("Credential type is not supported for WhatsApp")
	}
	referencePresent := input.SecretReference != nil && strings.TrimSpace(*input.SecretReference) != ""
	valuePresent := input.SecretValue != nil && strings.TrimSpace(*input.SecretValue) != ""
	if referencePresent == valuePresent {
		return CredentialRotationResult{}, httperror.BadRequest("Provide exactly one secure secret reference or secret value")
	}
	view, err := s.GetConfiguration(ctx, actor, connectionID)
	if err != nil {
		return CredentialRotationResult{}, err
	}
	if view.ID == uuid.Nil {
		if _, err = s.UpdateConfiguration(ctx, actor, connectionID, ConfigurationInput{}); err != nil {
			return CredentialRotationResult{}, err
		}
	}
	var existing channelplatform.CredentialReference
	findErr := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ?", actor.OrganizationID, connectionID, credentialType).First(&existing).Error
	rotated := findErr == nil && existing.Status != channelplatform.CredentialMissing && existing.SecretRef != ""
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return CredentialRotationResult{}, findErr
	}
	if err := s.saveCredentialInput(ctx, actor, connectionID, credentialType, input.SecretReference, input.SecretValue); err != nil {
		return CredentialRotationResult{}, err
	}
	now := s.now()
	updates := map[string]any{"credential_rotated_at": now, "updated_at": now}
	switch credentialType {
	case CredentialAccessToken:
		updates["last_test_message_at"] = nil
		updates["last_test_message_id"] = ""
		updates["last_test_message_error"] = ""
		updates["test_recipient_hash"] = ""
		updates["test_recipient_display"] = ""
		updates["last_inbound_test_at"] = nil
	case CredentialAppSecret:
		updates["last_signature_verified_at"] = nil
		updates["last_inbound_test_at"] = nil
	case CredentialVerifyToken:
		updates["webhook_status"] = WebhookPending
		updates["last_webhook_verified_at"] = nil
		updates["last_webhook_verification_error"] = ""
		updates["last_inbound_test_at"] = nil
	}
	action := "whatsapp_credential_set"
	if rotated {
		action = "whatsapp_credential_rotated"
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(updates).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, action, map[string]any{"credential_type": credentialType, "rotated": rotated})
	}); err != nil {
		return CredentialRotationResult{}, err
	}
	if err := s.refreshSetupState(ctx, actor, connectionID); err != nil {
		return CredentialRotationResult{}, err
	}
	return CredentialRotationResult{CredentialType: credentialType, Status: channelplatform.CredentialPresent, Configured: true, Rotated: rotated, UpdatedAt: now}, nil
}

func (s *Service) SendTestMessage(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input TestMessageInput, sender TestMessageSender) (TestMessageResult, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return TestMessageResult{}, httperror.Forbidden("You do not have permission to send a WhatsApp test message")
	}
	if sender == nil {
		return TestMessageResult{}, httperror.Unavailable("WhatsApp delivery is unavailable")
	}
	view, err := s.GetConfiguration(ctx, actor, connectionID)
	if err != nil {
		return TestMessageResult{}, err
	}
	if !view.Checklist.TestMessageReady {
		return TestMessageResult{}, httperror.BadRequest("Complete the phone, credential, and webhook setup before sending a test message")
	}
	recipient, err := normalizeRecipient(input.Recipient)
	if err != nil {
		return TestMessageResult{}, err
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = "Zidi WhatsApp connection test. Reply to confirm inbound delivery."
	}
	if len(message) > 1000 {
		return TestMessageResult{}, httperror.BadRequest("Test message must be 1000 characters or fewer")
	}
	command := channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: connectionID, ExternalCustomerID: recipient, MessageType: "text", Body: message, IdempotencyKey: "whatsapp-setup-test:" + uuid.NewString()}
	if input.TemplateID != nil {
		command.MessageType = "template"
		command.Template = &channelplatform.TemplateReference{ID: *input.TemplateID, Variables: input.TemplateVariables}
	}
	providerResult, sendErr := sender.Send(ctx, command)
	now := s.now()
	if sendErr != nil {
		safeError := publicProviderError(sendErr)
		_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"last_test_message_error": safeError, "setup_state": SetupRequiresAttention, "updated_at": now}).Error; err != nil {
				return err
			}
			return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_test_message_failed", map[string]any{"error": safeError})
		})
		return TestMessageResult{}, testMessageAPIError(sendErr)
	}
	display := maskRecipient(recipient)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"last_test_message_at": now, "last_test_message_id": providerResult.ProviderMessageID,
			"last_test_message_error": "", "test_recipient_hash": recipientHash(actor.OrganizationID, recipient),
			"test_recipient_display": display, "last_inbound_test_at": nil,
			"setup_state": SetupTestMessageSent, "updated_at": now,
		}
		if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(updates).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_test_message_sent", map[string]any{"recipient": display, "provider_message_id": providerResult.ProviderMessageID})
	}); err != nil {
		return TestMessageResult{}, err
	}
	return TestMessageResult{Status: SetupTestMessageSent, RecipientDisplay: display, ProviderMessageID: providerResult.ProviderMessageID, SentAt: now}, nil
}

func (s *Service) MarkInboundTest(ctx context.Context, configuration Configuration, externalCustomerID string) (bool, error) {
	if configuration.TestRecipientHash == "" || recipientHash(configuration.OrganizationID, externalCustomerID) != configuration.TestRecipientHash {
		return false, nil
	}
	now := s.now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).Updates(map[string]any{"last_inbound_test_at": now, "setup_state": SetupInboundTestReceived, "updated_at": now}).Error; err != nil {
			return err
		}
		return audit(tx, &configuration.OrganizationID, nil, configuration.ConnectionID, "whatsapp_inbound_test_received", map[string]any{"recipient": configuration.TestRecipientDisplay})
	})
	return err == nil, err
}

func (s *Service) CompleteSetup(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (ConfigurationView, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return ConfigurationView{}, httperror.Forbidden("You do not have permission to complete WhatsApp setup")
	}
	view, err := s.GetConfiguration(ctx, actor, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	if !view.Checklist.ReadyToComplete {
		return ConfigurationView{}, httperror.BadRequest("A successful outbound test and matching inbound reply are required")
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"setup_state": SetupConnected, "updated_at": now}).Error; err != nil {
			return err
		}
		if _, err := promoteReadyRecords(tx, view.Configuration, now); err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_setup_completed", map[string]any{"validated": true})
	}); err != nil {
		return ConfigurationView{}, err
	}
	return s.GetConfiguration(ctx, actor, connectionID)
}

func (s *Service) refreshSetupState(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) error {
	view, err := s.GetConfiguration(ctx, actor, connectionID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"setup_state": view.SetupState, "updated_at": s.now()}).Error
}

func deriveSetupState(configuration Configuration, connection channelplatform.ConnectionView, checklist SetupChecklist, credentials []CredentialStatus) string {
	if configuration.ID == uuid.Nil {
		return SetupNotConnected
	}
	for _, credential := range credentials {
		if credential.Status == channelplatform.CredentialExpired || credential.Status == channelplatform.CredentialRevoked || credential.Status == channelplatform.CredentialRequiresReauthorization {
			return SetupCredentialExpired
		}
	}
	if connection.Status == channelplatform.StatusDisconnected {
		return SetupDisconnected
	}
	if configuration.WebhookStatus == WebhookFailed {
		return SetupWebhookFailed
	}
	if configuration.LastTestMessageError != "" {
		return SetupRequiresAttention
	}
	if connection.Status == channelplatform.StatusRequiresAttention && configuration.SetupState != SetupConnected && configuration.SetupState != SetupHealthy {
		return SetupRequiresAttention
	}
	state := SetupStarted
	if checklist.AccessTokenConfigured && checklist.AppSecretConfigured && checklist.VerifyTokenConfigured {
		state = SetupCredentialsAdded
	}
	if checklist.WebhookVerified {
		state = SetupWebhookVerified
	}
	if checklist.WebhookVerified && checklist.PhoneIdentityConfigured {
		state = SetupPhoneVerified
	}
	if checklist.TestMessageReady {
		state = SetupTestMessageReady
	}
	if checklist.TestMessageSent {
		state = SetupTestMessageSent
	}
	if checklist.InboundTestReceived {
		state = SetupInboundTestReceived
	}
	if checklist.ReadyToComplete && (configuration.SetupState == SetupConnected || configuration.SetupState == SetupHealthy) {
		state = configuration.SetupState
	}
	if state == SetupHealthy && connection.Status == channelplatform.StatusDegraded {
		return SetupDegraded
	}
	return state
}

func nextSetupAction(state string) string {
	switch state {
	case SetupStarted, SetupNotConnected:
		return "Add the three required server credentials."
	case SetupCredentialsAdded:
		return "Add the business and phone references, then verify the Meta webhook."
	case SetupWebhookFailed:
		return "Confirm the callback URL and verify token in Meta, then retry verification."
	case SetupWebhookVerified:
		return "Add or confirm the WhatsApp phone number identity."
	case SetupPhoneVerified, SetupTestMessageReady:
		return "Send a controlled test message to an approved recipient."
	case SetupTestMessageSent:
		return "Reply from the same test recipient and wait for the signed inbound webhook."
	case SetupInboundTestReceived:
		return "Complete setup, then run the final health check."
	case SetupConnected:
		return "Run the final health check."
	case SetupHealthy:
		return "No setup action is required."
	case SetupCredentialExpired:
		return "Rotate the affected credential and validate the channel again."
	case SetupDisconnected, SetupDegraded, SetupRequiresAttention:
		return "Review health issues and recent provider events."
	default:
		return "Review the WhatsApp setup checklist."
	}
}

func safeOperationalEvent(event channelplatform.ProviderEvent) OperationalEvent {
	result := OperationalEvent{Type: event.EventType, Status: event.NormalizedStatus, OccurredAt: event.ReceivedAt}
	switch event.EventType {
	case "webhook_verification":
		if event.NormalizedStatus == "verified" {
			result.Title = "Webhook verified"
		} else {
			result.Title = "Webhook verification failed"
			result.Guidance = "Check the callback URL and verify token."
		}
	case "webhook_rejected":
		result.Title = "Signed webhook rejected"
		result.Guidance = "Check that the configured app secret belongs to the selected Meta app."
	case "inbound_message":
		result.Title = "Inbound message " + event.NormalizedStatus
		if event.NormalizedStatus == "failed" {
			result.Guidance = "Retry the message after checking conversation runtime health."
		}
	case "outbound_send":
		if event.NormalizedStatus == "sent" {
			result.Title = "Message accepted by Meta"
		} else {
			result.Title, result.Guidance = providerFailurePresentation(event)
		}
	case "delivery_status":
		result.Title = "Message " + event.NormalizedStatus
		if event.NormalizedStatus == "failed" {
			result.Guidance = "Check the recipient, template window, and Meta delivery error."
		}
	case "inbound_unsupported":
		result.Title = "Unsupported inbound message ignored"
		result.Guidance = "Ask the customer to send text or supported media."
	default:
		result.Title = "WhatsApp provider activity"
	}
	return result
}

func providerFailurePresentation(event channelplatform.ProviderEvent) (string, string) {
	errorText := strings.ToLower(event.ProcessingError)
	metadata := jsonObject(event.PayloadMetadata)
	statusCode := metadataNumber(metadata["status_code"])
	providerCode := metadataNumber(metadata["error_code"])
	switch {
	case statusCode == http.StatusTooManyRequests || providerCode == 4 || providerCode == 130429 || strings.Contains(errorText, "rate limit"):
		return "Meta rate limited messaging", "Wait for the provider rate-limit window, then retry."
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden || providerCode == 190 || strings.Contains(errorText, "token"):
		return "Credential rejected", "Rotate or validate the access token and confirm its Meta app and WABA permissions."
	case providerCode == 131026 || providerCode == 131030 || strings.Contains(errorText, "recipient"):
		return "Recipient rejected", "Confirm the approved test recipient, international country code, and Meta allow-list."
	case statusCode >= 500 || statusCode == 0:
		return "Meta delivery unavailable", "Check Meta service availability and retry once the provider recovers."
	default:
		return "Message send failed", "Check credentials, recipient eligibility, and Meta service availability."
	}
}

func metadataNumber(value any) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case json.Number:
		result, _ := number.Int64()
		return result
	default:
		return 0
	}
}

func (s *Service) operationalEvents(ctx context.Context, organizationID, connectionID uuid.UUID) ([]OperationalEvent, error) {
	var events []channelplatform.ProviderEvent
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", organizationID, connectionID).Order("received_at DESC").Limit(20).Find(&events).Error; err != nil {
		return nil, err
	}
	result := make([]OperationalEvent, 0, len(events))
	for _, event := range events {
		result = append(result, safeOperationalEvent(event))
	}
	return result, nil
}

func (s *Service) configurationAndConnection(ctx context.Context, connectionID uuid.UUID) (Configuration, channelplatform.ChannelConnection, error) {
	var configuration Configuration
	if err := s.db.WithContext(ctx).Where("channel_connection_id = ?", connectionID).First(&configuration).Error; err != nil {
		return Configuration{}, channelplatform.ChannelConnection{}, err
	}
	var connection channelplatform.ChannelConnection
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ? AND provider = ? AND status <> ?", configuration.OrganizationID, connectionID, Provider, channelplatform.StatusArchived).First(&connection).Error; err != nil {
		return Configuration{}, channelplatform.ChannelConnection{}, err
	}
	return configuration, connection, nil
}

func constantTimeEqual(first, second string) bool {
	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	if first == "" || len(first) != len(second) {
		return false
	}
	firstHash := sha256.Sum256([]byte(first))
	secondHash := sha256.Sum256([]byte(second))
	return subtle.ConstantTimeCompare(firstHash[:], secondHash[:]) == 1
}

func safeVerificationReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "invalid_request":
		return "invalid_request"
	default:
		return "verify_token_mismatch"
	}
}

func knownCredentialType(value string) bool {
	for _, credentialType := range requiredCredentialTypes {
		if value == credentialType {
			return true
		}
	}
	return false
}

func normalizeRecipient(value string) (string, error) {
	replacer := strings.NewReplacer("+", "", " ", "", "-", "", "(", "", ")", "")
	value = replacer.Replace(strings.TrimSpace(value))
	if !regexp.MustCompile(`^[0-9]{8,20}$`).MatchString(value) {
		return "", httperror.BadRequest("Test recipient must include a valid international country code")
	}
	return value, nil
}

func recipientHash(organizationID uuid.UUID, value string) string {
	normalized, err := normalizeRecipient(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(organizationID.String() + ":" + normalized))
	return hex.EncodeToString(sum[:])
}

func maskRecipient(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

func testMessageAPIError(err error) error {
	var policyErr PolicyBlockedError
	if errors.As(err, &policyErr) {
		return httperror.New(http.StatusConflict, "WHATSAPP_POLICY_BLOCKED", policyErr.Decision.Reason)
	}
	var providerErr ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode == http.StatusTooManyRequests {
			return httperror.New(http.StatusTooManyRequests, "PROVIDER_RATE_LIMITED", "Meta rate limited the test message. Wait and retry.")
		}
		if providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden {
			return httperror.BadRequest("Meta rejected the configured access token")
		}
		return httperror.BadRequest("Meta rejected the WhatsApp test message")
	}
	return httperror.Unavailable("WhatsApp test delivery is temporarily unavailable")
}
