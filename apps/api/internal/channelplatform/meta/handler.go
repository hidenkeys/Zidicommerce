package meta

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

type InboundProcessor interface {
	ProcessChannelInbound(ctx context.Context, organizationID uuid.UUID, event channelplatform.InboundEvent) error
}

type Handler struct {
	service     *Service
	platform    *channelplatform.Service
	processor   InboundProcessor
	verifyToken string
	adapters    map[string]*MessagingAdapter
}

func NewHandler(service *Service, platform *channelplatform.Service, processor InboundProcessor, verifyToken string, adapters ...*MessagingAdapter) *Handler {
	handler := &Handler{service: service, platform: platform, processor: processor, verifyToken: strings.TrimSpace(verifyToken), adapters: map[string]*MessagingAdapter{}}
	for _, adapter := range adapters {
		if adapter != nil {
			handler.adapters[adapter.Provider()] = adapter
		}
	}
	return handler
}

func (h *Handler) RegisterPublic(router fiber.Router) {
	router.Get("/runtime/webhooks/meta", h.verifyWebhook)
	router.Post("/runtime/webhooks/meta", h.receiveWebhook)
}

func (h *Handler) verifyWebhook(c *fiber.Ctx) error {
	if c.Query("hub.mode") != "subscribe" || !constantTimeEqual(h.verifyToken, c.Query("hub.verify_token")) || strings.TrimSpace(c.Query("hub.challenge")) == "" {
		return httperror.Forbidden("Webhook verification failed")
	}
	return c.SendString(c.Query("hub.challenge"))
}

func (h *Handler) receiveWebhook(c *fiber.Ctx) error {
	body := append([]byte(nil), c.BodyRaw()...)
	payload, err := decodeWebhook(body)
	if err != nil {
		return httperror.BadRequest("Invalid Meta webhook payload")
	}
	provider := providerForObject(payload.Object)
	adapter := h.adapters[provider]
	if adapter == nil {
		return httperror.BadRequest("Meta webhook object is not supported")
	}
	identities := uniqueEntryIDs(payload.Entry)
	envelope := channelplatform.ProviderEnvelope{Provider: provider, Headers: map[string]string{"X-Hub-Signature-256": c.Get("X-Hub-Signature-256")}, RawPayload: body, ReceivedAt: time.Now().UTC()}
	if err := adapter.VerifySignature(c.UserContext(), envelope); err != nil {
		for _, identityID := range identities {
			resolved, resolveErr := h.service.ResolveByProviderIdentity(c.UserContext(), provider, identityID)
			if resolveErr != nil {
				continue
			}
			_ = h.service.MarkSignatureRejected(c.UserContext(), resolved)
			eventID := messagingEventID(webhookEntry{ID: identityID, Time: envelope.ReceivedAt.Unix()}, webhookMessaging{}, "rejected")
			_, _, _ = h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: provider, EventType: "webhook_rejected", ProviderEventID: eventID, IdempotencyKey: provider + ":rejected:" + eventID, NormalizedStatus: "rejected", ProcessingError: "Webhook signature verification failed", PayloadMetadata: map[string]any{"reason": "invalid_signature"}})
		}
		return httperror.Forbidden("Webhook signature verification failed")
	}

	processed, duplicates, ignored, deliveries := 0, 0, 0, 0
	for _, identityID := range identities {
		resolved, err := h.service.ResolveByProviderIdentity(c.UserContext(), provider, identityID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ignored++
			continue
		}
		if err != nil {
			return err
		}
		_ = h.service.MarkSignatureVerified(c.UserContext(), resolved)
		envelope.ConnectionID = resolved.Connection.ID
		inbound, err := adapter.NormalizeInbound(c.UserContext(), envelope)
		if err != nil {
			return httperror.BadRequest("Meta webhook normalization failed")
		}
		for _, event := range inbound {
			providerEvent, created, err := h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: provider, EventType: "inbound_message", ProviderEventID: event.ProviderEventID, IdempotencyKey: provider + ":message:" + event.ExternalMessageID, NormalizedStatus: "processing", PayloadMetadata: event.Metadata, ReceivedAt: event.Timestamp})
			if err != nil {
				return err
			}
			if !created {
				claimed, err := h.service.ClaimProviderEventRetry(c.UserContext(), providerEvent.ID, resolved.Connection.OrganizationID)
				if err != nil {
					return err
				}
				if !claimed {
					duplicates++
					continue
				}
			}
			if err := h.service.RecordInboundContact(c.UserContext(), resolved, event.ExternalCustomerID, event.Timestamp); err != nil {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, resolved.Connection.OrganizationID, "failed", "Contact policy state could not be updated")
				return err
			}
			if event.MessageType == "reaction" {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, resolved.Connection.OrganizationID, "processed", "")
				_, _ = h.platform.IncrementMetric(c.UserContext(), channelplatform.MetricInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, InboundCount: 1, Metadata: map[string]any{"provider": provider, "event_type": "reaction"}})
				processed++
				continue
			}
			if h.processor == nil {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, resolved.Connection.OrganizationID, "failed", "Inbound runtime processor is unavailable")
				return httperror.BadRequest("Inbound runtime processor is unavailable")
			}
			if err := h.processor.ProcessChannelInbound(c.UserContext(), resolved.Connection.OrganizationID, event); err != nil {
				_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, resolved.Connection.OrganizationID, "failed", "Conversation processing failed")
				return err
			}
			_ = h.service.MarkProviderEventResult(c.UserContext(), providerEvent.ID, resolved.Connection.OrganizationID, "processed", "")
			_, _ = h.platform.IncrementMetric(c.UserContext(), channelplatform.MetricInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, InboundCount: 1, Metadata: map[string]any{"provider": provider}})
			processed++
		}
		unsupported, err := adapter.UnsupportedInbound(c.UserContext(), envelope)
		if err != nil {
			return err
		}
		for _, event := range unsupported {
			_, created, err := h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: provider, EventType: "inbound_unsupported", ProviderEventID: event.ProviderEventID, IdempotencyKey: provider + ":unsupported:" + event.ProviderEventID, NormalizedStatus: "ignored", PayloadMetadata: map[string]any{"message_type": event.MessageType}})
			if err != nil {
				return err
			}
			if created {
				ignored++
			} else {
				duplicates++
			}
		}
		updates, err := adapter.NormalizeDelivery(c.UserContext(), envelope)
		if err != nil {
			return err
		}
		for _, update := range updates {
			eventID := update.ProviderMessageID + ":" + update.Status
			_, created, err := h.platform.RecordProviderEvent(c.UserContext(), channelplatform.ProviderEventInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: provider, EventType: "delivery_status", ProviderEventID: eventID, IdempotencyKey: provider + ":delivery:" + eventID, NormalizedStatus: update.Status, ReceivedAt: update.OccurredAt, PayloadMetadata: map[string]any{"provider_message_id": update.ProviderMessageID, "status": update.Status}})
			if err != nil {
				return err
			}
			if !created {
				duplicates++
				continue
			}
			updated, err := h.service.ApplyDeliveryUpdate(c.UserContext(), resolved, update)
			if err != nil {
				return err
			}
			if updated {
				deliveries++
			}
		}
		if len(inbound) > 0 {
			_, _ = h.platform.RecordHealthCheck(c.UserContext(), resolved.Connection.OrganizationID, resolved.Connection.ID, channelplatform.HealthCheckInput{Status: channelplatform.HealthHealthy, Metadata: `{"check_type":"inbound_webhook"}`})
		}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"processed": processed, "duplicates": duplicates, "ignored": ignored, "delivery_updates": deliveries}})
}

func uniqueEntryIDs(entries []webhookEntry) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func constantTimeEqual(expected, actual string) bool {
	if expected == "" || len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}
