package whatsapp

import (
	"context"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	runtimeengine "github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"time"
)

type FlowRuntime interface {
	ExchangeJourneyFlow(context.Context, uuid.UUID, uuid.UUID, runtimeengine.JourneyFlowRequest) (map[string]any, error)
}

func (h *Handler) registerFlowRoutes(public, protected fiber.Router) {
	if public != nil {
		public.Post("/journey-flows/:id/data", limiter.New(limiter.Config{Max: 120, Expiration: time.Minute}), h.exchangeFlow)
	}
	if protected == nil {
		return
	}
	limited := limiter.New(limiter.Config{Max: 10, Expiration: time.Minute, KeyGenerator: func(c *fiber.Ctx) string { u := currentUser(c); return u.OrganizationID.String() + ":" + u.ID.String() }})
	protected.Post("/channel-platform/connections/:id/whatsapp/flow-key", auth.RequirePermission(authz.PermissionChannelsManage), limited, func(c *fiber.Ctx) error {
		id, err := connectionID(c)
		if err != nil {
			return err
		}
		err = h.service.RegisterFlowKey(c.UserContext(), currentUser(c), id)
		return respond(c, map[string]bool{"registered": err == nil}, err)
	})
	protected.Post("/channel-platform/connections/:id/whatsapp/journey-flows/:version/:operation", auth.RequirePermission(authz.PermissionBotPublish), auth.RequirePermission(authz.PermissionChannelsManage), limited, func(c *fiber.Ctx) error {
		id, err := connectionID(c)
		if err != nil {
			return err
		}
		version, err := uuid.Parse(c.Params("version"))
		if err != nil {
			return httperror.BadRequest("Invalid journey version")
		}
		var result any
		if c.Params("operation") == "prepare" {
			result, err = h.service.PrepareJourneyFlow(c.UserContext(), currentUser(c), id, version)
		} else {
			result, err = h.service.JourneyFlowOperation(c.UserContext(), currentUser(c), id, version, c.Params("operation"))
		}
		return respond(c, result, err)
	})
}
func (h *Handler) exchangeFlow(c *fiber.Ctx) error {
	if !h.service.flowConfig.Enabled {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	id, err := connectionID(c)
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if len(c.Body()) > 64*1024 {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var config Configuration
	if h.service.db.WithContext(c.UserContext()).Where("channel_connection_id = ?", id).First(&config).Error != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if config.WhatsAppBusinessAccountID != h.service.flowConfig.TestWABAID || config.PhoneNumberID != h.service.flowConfig.TestPhoneID || h.service.flowConfig.TestWABAID == "" {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var connection channelplatform.ChannelConnection
	if h.service.db.WithContext(c.UserContext()).Where("organization_id = ? AND id = ? AND provider = ? AND environment = ?", config.OrganizationID, id, Provider, channelplatform.EnvironmentSandbox).First(&connection).Error != nil {
		return c.SendStatus(fiber.StatusForbidden)
	}
	secret, err := h.service.ResolveCredential(c.UserContext(), config.OrganizationID, id, CredentialAppSecret)
	if err != nil || !flowSignature(c.Body(), c.Get("X-Hub-Signature-256"), secret) {
		return c.SendStatus(432)
	}
	private, err := h.service.ResolveCredential(c.UserContext(), config.OrganizationID, id, flowPrivateKey)
	if err != nil {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	data, cipher, err := decryptFlow(c.Body(), private)
	if err != nil {
		return c.SendStatus(421)
	}
	var request runtimeengine.JourneyFlowRequest
	if json.Unmarshal(data, &request) != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	engine, ok := h.processor.(FlowRuntime)
	if !ok {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	response, err := engine.ExchangeJourneyFlow(c.UserContext(), config.OrganizationID, id, request)
	status := fiber.StatusOK
	if err != nil {
		response = map[string]any{"version": "3.0", "data": map[string]any{"error_message": "This selection could not be accepted. Reopen the journey in this chat."}}
		status = 427
	}
	encrypted, encryptErr := cipher.encrypt(response)
	if encryptErr != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	// Audit only classification, never ciphertext, tokens, submitted answers or secrets.
	if request.Action != "ping" {
		if err := audit(h.service.db.WithContext(c.UserContext()), &config.OrganizationID, nil, id, "journey_flow_exchange", map[string]any{"action": flowAuditAction(request.Action), "accepted": status == fiber.StatusOK}); err != nil {
			return c.SendStatus(fiber.StatusServiceUnavailable)
		}
	}
	c.Set(fiber.HeaderContentType, "text/plain")
	return c.Status(status).SendString(encrypted)
}

func flowAuditAction(action string) string {
	switch action {
	case "ping", "INIT", "BACK", "data_exchange":
		return action
	default:
		return "invalid_action"
	}
}
