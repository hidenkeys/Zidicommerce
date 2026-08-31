package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type InboundProcessor interface {
	ProcessChannelInbound(context.Context, uuid.UUID, channelplatform.InboundEvent) error
}

type Handler struct {
	service   *Service
	platform  *channelplatform.Service
	adapter   *Adapter
	processor InboundProcessor
}

func NewHandler(service *Service, platform *channelplatform.Service, adapter *Adapter, processor InboundProcessor) *Handler {
	return &Handler{service: service, platform: platform, adapter: adapter, processor: processor}
}

func (h *Handler) RegisterPublic(router fiber.Router) {
	router.Get("/runtime/webhooks/whatsapp", h.verifyWebhook)
	router.Post("/runtime/webhooks/whatsapp", h.receiveWebhook)
}

func (h *Handler) RegisterProtected(router fiber.Router) {
	router.Get("/channel-platform/connections/:id/whatsapp", auth.RequirePermission(authz.PermissionChannelsView), h.getConfiguration)
	router.Patch("/channel-platform/connections/:id/whatsapp", auth.RequirePermission(authz.PermissionChannelsManage), h.updateConfiguration)
	router.Post("/channel-platform/connections/:id/whatsapp/credentials/rotate", auth.RequirePermission(authz.PermissionChannelsManage), h.rotateCredential)
	router.Post("/channel-platform/connections/:id/whatsapp/test-message", auth.RequirePermission(authz.PermissionChannelsManage), h.sendTestMessage)
	router.Post("/channel-platform/connections/:id/whatsapp/complete", auth.RequirePermission(authz.PermissionChannelsManage), h.completeSetup)
	router.Post("/channel-platform/connections/:id/whatsapp/migrate-legacy", auth.RequirePermission(authz.PermissionChannelsManage), h.migrateLegacy)
	router.Post("/channel-platform/connections/:id/whatsapp/health-check", auth.RequirePermission(authz.PermissionChannelsManage), h.checkHealth)
	router.Post("/channel-platform/connections/:id/whatsapp/embedded-signup/initiate", auth.RequirePermission(authz.PermissionChannelsManage), h.initiateEmbeddedSignup)
	router.Post("/channel-platform/connections/:id/whatsapp/embedded-signup/complete", auth.RequirePermission(authz.PermissionChannelsManage), h.completeEmbeddedSignup)
	router.Post("/channel-platform/connections/:id/whatsapp/assisted-setup", auth.RequirePermission(authz.PermissionChannelsManage), h.updateAssistedSetup)
	router.Get("/channel-platform/connections/:id/whatsapp/contacts", auth.RequirePermission(authz.PermissionChannelsView), h.listContactStates)
	router.Patch("/channel-platform/connections/:id/whatsapp/contacts/:contact_id", auth.RequirePermission(authz.PermissionChannelsManage), h.updateContactConsent)
	router.Get("/channel-platform/connections/:id/whatsapp/templates", auth.RequirePermission(authz.PermissionChannelsView), h.listTemplates)
	router.Post("/channel-platform/connections/:id/whatsapp/templates", auth.RequirePermission(authz.PermissionChannelsManage), h.createTemplate)
	router.Get("/channel-platform/connections/:id/whatsapp/templates/:template_id", auth.RequirePermission(authz.PermissionChannelsView), h.getTemplate)
	router.Patch("/channel-platform/connections/:id/whatsapp/templates/:template_id", auth.RequirePermission(authz.PermissionChannelsManage), h.updateTemplate)
	router.Delete("/channel-platform/connections/:id/whatsapp/templates/:template_id", auth.RequirePermission(authz.PermissionChannelsManage), h.archiveTemplate)
	router.Post("/channel-platform/connections/:id/whatsapp/templates/:template_id/preview", auth.RequirePermission(authz.PermissionChannelsView), h.previewTemplate)
	router.Get("/channel-platform/connections/:id/whatsapp/advanced-metrics", auth.RequirePermission(authz.PermissionChannelsView), h.getAdvancedMetrics)
	router.Get("/channel-platform/connections/:id/whatsapp/policy-blocks", auth.RequirePermission(authz.PermissionChannelsView), h.listPolicyBlocks)
	router.Get("/channel-platform/connections/:id/whatsapp/conversations/:conversation_id/policy-context", auth.RequirePermission(authz.PermissionConversationsView), h.getConversationPolicyContext)
}

func (h *Handler) listContactStates(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.ListContactStates(c.UserContext(), currentUser(c), id, c.Query("consent"), c.QueryInt("limit", 100))
	return respond(c, data, err)
}

func (h *Handler) updateContactConsent(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	contactID, err := uuid.Parse(c.Params("contact_id"))
	if err != nil {
		return httperror.BadRequest("Invalid WhatsApp contact state ID")
	}
	var input ContactStateInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid WhatsApp contact consent payload")
	}
	data, err := h.service.UpdateContactConsent(c.UserContext(), currentUser(c), id, contactID, input)
	return respond(c, data, err)
}

func (h *Handler) listTemplates(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.ListTemplates(c.UserContext(), currentUser(c), id, c.Query("status"), c.Query("category"))
	return respond(c, data, err)
}

func (h *Handler) getTemplate(c *fiber.Ctx) error {
	id, templateID, err := connectionAndTemplateIDs(c)
	if err != nil {
		return err
	}
	data, err := h.service.GetTemplate(c.UserContext(), currentUser(c), id, templateID)
	return respond(c, data, err)
}

func (h *Handler) createTemplate(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input MessageTemplateInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid WhatsApp template payload")
	}
	data, err := h.service.CreateTemplate(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) updateTemplate(c *fiber.Ctx) error {
	id, templateID, err := connectionAndTemplateIDs(c)
	if err != nil {
		return err
	}
	var input MessageTemplateUpdate
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid WhatsApp template payload")
	}
	data, err := h.service.UpdateTemplate(c.UserContext(), currentUser(c), id, templateID, input)
	return respond(c, data, err)
}

func (h *Handler) archiveTemplate(c *fiber.Ctx) error {
	id, templateID, err := connectionAndTemplateIDs(c)
	if err != nil {
		return err
	}
	data, err := h.service.ArchiveTemplate(c.UserContext(), currentUser(c), id, templateID)
	return respond(c, data, err)
}

func (h *Handler) previewTemplate(c *fiber.Ctx) error {
	id, templateID, err := connectionAndTemplateIDs(c)
	if err != nil {
		return err
	}
	var input TemplatePreviewInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid WhatsApp template preview payload")
	}
	data, err := h.service.PreviewTemplate(c.UserContext(), currentUser(c), id, templateID, input.Variables)
	return respond(c, data, err)
}

func (h *Handler) getAdvancedMetrics(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.GetAdvancedMetrics(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listPolicyBlocks(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.ListPolicyBlocks(c.UserContext(), currentUser(c), id, c.QueryInt("limit", 25))
	return respond(c, data, err)
}

func (h *Handler) getConversationPolicyContext(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	conversationID, err := uuid.Parse(c.Params("conversation_id"))
	if err != nil {
		return httperror.BadRequest("Invalid conversation ID")
	}
	data, err := h.service.GetConversationPolicyContext(c.UserContext(), currentUser(c), id, conversationID)
	return respond(c, data, err)
}

func (h *Handler) getConfiguration(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.GetConfiguration(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateConfiguration(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input ConfigurationInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid WhatsApp configuration payload")
	}
	data, err := h.service.UpdateConfiguration(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) rotateCredential(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input CredentialRotationInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid credential rotation payload")
	}
	data, err := h.service.RotateCredential(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) sendTestMessage(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input TestMessageInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid WhatsApp test message payload")
	}
	data, err := h.service.SendTestMessage(c.UserContext(), currentUser(c), id, input, h.adapter)
	return respond(c, data, err)
}

func (h *Handler) completeSetup(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.CompleteSetup(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) migrateLegacy(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.MigrateLegacyCredentials(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) checkHealth(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.EvaluateHealth(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) initiateEmbeddedSignup(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.InitiateEmbeddedSignup(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) completeEmbeddedSignup(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input EmbeddedSignupCompletionInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid Meta Embedded Signup completion payload")
	}
	data, err := h.service.CompleteEmbeddedSignup(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) updateAssistedSetup(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input AssistedSetupInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid assisted setup payload")
	}
	data, err := h.service.UpdateAssistedSetup(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) verifyWebhook(c *fiber.Ctx) error {
	attemptConnectionID, _ := uuid.Parse(strings.TrimSpace(c.Query("connection_id")))
	if c.Query("hub.mode") != "subscribe" || strings.TrimSpace(c.Query("hub.challenge")) == "" {
		h.service.RecordWebhookVerificationFailure(c.UserContext(), attemptConnectionID, "invalid_request")
		return httperror.Forbidden("Webhook verification failed")
	}
	if attemptConnectionID == uuid.Nil && h.service.VerifyPlatformWebhookToken(c.Query("hub.verify_token")) {
		c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
		return c.SendString(c.Query("hub.challenge"))
	}
	configuration, _, ok := h.service.ResolveVerifyTokenForConnection(c.UserContext(), attemptConnectionID, c.Query("hub.verify_token"))
	if !ok {
		h.service.RecordWebhookVerificationFailure(c.UserContext(), attemptConnectionID, "verify_token_mismatch")
		return httperror.Forbidden("Webhook verification failed")
	}
	if err := h.service.MarkWebhookVerified(c.UserContext(), configuration); err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	return c.SendString(c.Query("hub.challenge"))
}

func (h *Handler) receiveWebhook(c *fiber.Ctx) error {
	body := append([]byte(nil), c.BodyRaw()...)
	payload, err := decodeWebhook(body)
	if err != nil {
		return httperror.BadRequest("Invalid WhatsApp webhook payload")
	}
	phoneIDs := webhookPhoneNumberIDs(payload)
	if len(phoneIDs) == 0 {
		return httperror.BadRequest("WhatsApp webhook does not identify a configured phone number")
	}
	processed := 0
	duplicates := 0
	ignored := 0
	deliveryUpdates := 0
	for _, phoneID := range phoneIDs {
		configuration, connection, resolveErr := h.service.ResolveByPhoneNumberID(c.UserContext(), phoneID)
		if resolveErr != nil {
			return resolveErr
		}
		envelope := channelplatform.ProviderEnvelope{Provider: Provider, ConnectionID: connection.ID, Headers: map[string]string{"X-Hub-Signature-256": c.Get("X-Hub-Signature-256")}, RawPayload: body, ReceivedAt: time.Now().UTC()}
		if verifyErr := h.adapter.VerifySignature(c.UserContext(), envelope); verifyErr != nil {
			_ = h.service.MarkSignatureRejected(c.UserContext(), configuration)
			eventID := rejectedEventID(body, connection.ID)
			_, _, _ = h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, Provider: Provider, EventType: "webhook_rejected", ProviderEventID: eventID, IdempotencyKey: "whatsapp:rejected:" + eventID, NormalizedStatus: "rejected", ProcessingError: "Webhook signature verification failed", PayloadMetadata: map[string]any{"reason": "invalid_signature"}})
			_ = h.service.AuditProviderActivity(c.UserContext(), configuration, "whatsapp_inbound_event_rejected", map[string]any{"reason": "invalid_signature"})
			return httperror.Forbidden("Webhook signature verification failed")
		}
		inbound, normalizeErr := h.adapter.NormalizeInbound(c.UserContext(), envelope)
		if normalizeErr != nil {
			return httperror.BadRequest("WhatsApp webhook normalization failed")
		}
		for _, event := range inbound {
			providerEvent, created, recordErr := h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, Provider: Provider, EventType: "inbound_message", ProviderEventID: event.ProviderEventID, IdempotencyKey: "whatsapp:message:" + event.ExternalMessageID, NormalizedStatus: "processing", PayloadMetadata: event.Metadata, ReceivedAt: event.Timestamp})
			if recordErr != nil {
				return recordErr
			}
			if !created {
				claimed, claimErr := h.service.ClaimProviderEventRetry(c.UserContext(), providerEvent.ID, connection.OrganizationID)
				if claimErr != nil {
					return claimErr
				}
				if !claimed {
					duplicates++
					continue
				}
			}
			if _, _, contactErr := h.service.RecordInboundContact(c.UserContext(), configuration, event); contactErr != nil {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, connection.OrganizationID, "failed", "WhatsApp contact policy state could not be updated")
				return contactErr
			}
			if h.processor == nil {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, connection.OrganizationID, "failed", "Inbound runtime processor is unavailable")
				return httperror.BadRequest("Inbound runtime processor is unavailable")
			}
			if processErr := h.processor.ProcessChannelInbound(c.UserContext(), connection.OrganizationID, event); processErr != nil {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, connection.OrganizationID, "failed", "Conversation processing failed")
				return processErr
			}
			_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, connection.OrganizationID, "processed", "")
			_, _ = h.service.MarkInboundTest(c.UserContext(), configuration, event.ExternalCustomerID)
			_, _ = h.platform.IncrementMetric(c.UserContext(), channelplatform.MetricInput{OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, InboundCount: 1, Metadata: map[string]any{"provider": Provider}})
			_ = h.service.AuditProviderActivity(c.UserContext(), configuration, "whatsapp_inbound_event_accepted", map[string]any{"provider_event_id": event.ProviderEventID, "message_type": event.MessageType})
			processed++
		}
		unsupported, unsupportedErr := h.adapter.UnsupportedInbound(c.UserContext(), envelope)
		if unsupportedErr != nil {
			return unsupportedErr
		}
		for _, event := range unsupported {
			_, created, recordErr := h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, Provider: Provider, EventType: "inbound_unsupported", ProviderEventID: event.ProviderEventID, IdempotencyKey: "whatsapp:unsupported:" + event.ProviderEventID, NormalizedStatus: "ignored", PayloadMetadata: map[string]any{"message_type": event.MessageType}})
			if recordErr != nil {
				return recordErr
			}
			if created {
				if _, _, contactErr := h.service.RecordInboundContact(c.UserContext(), configuration, channelplatform.InboundEvent{Provider: Provider, ChannelConnectionID: connection.ID, ProviderEventID: event.ProviderEventID, ExternalMessageID: event.ProviderEventID, ExternalCustomerID: event.ExternalCustomerID, Timestamp: envelope.ReceivedAt}); contactErr != nil {
					return contactErr
				}
				ignored++
			} else {
				duplicates++
			}
		}
		deliveries, deliveryErr := h.adapter.NormalizeDelivery(c.UserContext(), envelope)
		if deliveryErr != nil {
			return deliveryErr
		}
		for _, update := range deliveries {
			eventIdentity := statusEventID(update)
			_, created, recordErr := h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, Provider: Provider, EventType: "delivery_status", ProviderEventID: eventIdentity, IdempotencyKey: "whatsapp:delivery:" + eventIdentity, NormalizedStatus: update.Status, ReceivedAt: update.OccurredAt, PayloadMetadata: map[string]any{"provider_message_id": update.ProviderMessageID, "status": update.Status, "error_code": update.ErrorCode}})
			if recordErr != nil {
				return recordErr
			}
			if !created {
				duplicates++
				continue
			}
			updated, applyErr := h.service.ApplyDeliveryUpdate(c.UserContext(), configuration, update)
			if applyErr != nil {
				return applyErr
			}
			if updated {
				deliveryUpdates++
			}
		}
		if len(inbound) > 0 {
			_ = h.service.MarkInbound(c.UserContext(), configuration)
			_, _ = h.platform.RecordHealthCheck(c.UserContext(), connection.OrganizationID, connection.ID, channelplatform.HealthCheckInput{Status: channelplatform.HealthHealthy, Metadata: jsonValue(map[string]any{"check_type": "inbound_webhook"})})
		}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"processed": processed, "duplicates": duplicates, "ignored": ignored, "delivery_updates": deliveryUpdates}})
}

func webhookPhoneNumberIDs(payload webhookPayload) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			phoneID := strings.TrimSpace(change.Value.Metadata.PhoneNumberID)
			if phoneID != "" && !seen[phoneID] {
				seen[phoneID] = true
				result = append(result, phoneID)
			}
		}
	}
	return result
}

func rejectedEventID(body []byte, connectionID uuid.UUID) string {
	sum := sha256.Sum256(append(append([]byte(nil), body...), connectionID.String()...))
	return hex.EncodeToString(sum[:16])
}

func currentUser(c *fiber.Ctx) auth.CurrentUser {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		panic(err)
	}
	return user
}

func connectionID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, httperror.BadRequest("Invalid channel connection ID")
	}
	return id, nil
}

func connectionAndTemplateIDs(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	connectionID, err := connectionID(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	templateID, err := uuid.Parse(c.Params("template_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, httperror.BadRequest("Invalid WhatsApp template ID")
	}
	return connectionID, templateID, nil
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
