package runtime

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(router fiber.Router) {
	router.Post("/runtime/test/start", h.startTest)
	router.Post("/runtime/test/message", h.testMessage)
	router.Get("/runtime/conversations", h.listConversations)
	router.Get("/runtime/conversations/:id", h.getConversation)
	router.Get("/runtime/conversations/:id/messages", h.listConversationMessages)
}

func (h *Handler) RegisterPublic(router fiber.Router) {
	router.Get("/runtime/webhooks/whatsapp", h.verifyWhatsApp)
	router.Post("/runtime/webhooks/whatsapp", h.whatsAppWebhook)
}

func (h *Handler) startTest(c *fiber.Ctx) error {
	var input RuntimeStartInput
	if err := bind(c, &input); err != nil {
		return err
	}
	session, err := h.service.StartTestSession(c.UserContext(), currentUser(c), input)
	return respond(c, session, err)
}

func (h *Handler) testMessage(c *fiber.Ctx) error {
	var input RuntimeMessageInput
	if err := bind(c, &input); err != nil {
		return err
	}
	result, err := h.service.ProcessTestMessage(c.UserContext(), currentUser(c), input)
	return respond(c, result, err)
}

func (h *Handler) listConversations(c *fiber.Ctx) error {
	data, err := h.service.ListConversations(c.UserContext(), currentUser(c))
	return respond(c, data, err)
}

func (h *Handler) getConversation(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetConversation(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listConversationMessages(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListConversationMessages(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) verifyWhatsApp(c *fiber.Ctx) error {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")
	if mode != "subscribe" || !h.service.VerifyWhatsAppToken(c.UserContext(), token) {
		return httperror.Forbidden("Webhook verification failed")
	}
	return c.SendString(challenge)
}

func (h *Handler) whatsAppWebhook(c *fiber.Ctx) error {
	var payload WhatsAppWebhookPayload
	body := c.BodyRaw()
	if err := json.Unmarshal(body, &payload); err != nil {
		return httperror.BadRequest("Invalid JSON payload")
	}
	inputs := h.service.ParseWhatsAppWebhook(payload)
	results := make([]RuntimeResult, 0, len(inputs))
	signature := c.Get("X-Hub-Signature-256")
	for _, input := range inputs {
		channel, err := h.service.GetWhatsAppChannel(c.UserContext(), input.ProviderChannelID)
		if err != nil {
			return err
		}
		if !h.service.VerifyWhatsAppRequest(channel, signature, body) {
			return httperror.Forbidden("Webhook signature verification failed")
		}
		input.ChannelID = &channel.ID
		result, err := h.service.ProcessMessage(c.UserContext(), input)
		if err != nil {
			return err
		}
		if _, err := h.service.DispatchOutbound(c.UserContext(), channel, input, result); err != nil {
			h.service.log.Warn("runtime outbound dispatch failed", "channel_id", channel.ID, "error", err)
		}
		results = append(results, result)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"processed": len(results), "results": results}})
}

func currentUser(c *fiber.Ctx) auth.CurrentUser {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		panic(err)
	}
	return user
}

func bind(c *fiber.Ctx, target any) error {
	if err := c.BodyParser(target); err != nil {
		return httperror.BadRequest("Invalid JSON payload")
	}
	return nil
}

func paramID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, httperror.BadRequest("Invalid ID")
	}
	return id, nil
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
