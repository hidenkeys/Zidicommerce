package ai

import (
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
	router.Post("/ai/test-chat/start", h.start)
	router.Post("/ai/test-chat/message", h.message)
	router.Get("/ai/test-chat/sessions/:id/messages", h.messages)
}

func (h *Handler) start(c *fiber.Ctx) error {
	var input StartInput
	if len(c.BodyRaw()) > 0 {
		if err := c.BodyParser(&input); err != nil {
			return httperror.BadRequest("Invalid JSON payload")
		}
	}
	data, err := h.service.Start(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) message(c *fiber.Ctx) error {
	var input MessageInput
	if err := c.BodyParser(&input); err != nil {
		return httperror.BadRequest("Invalid JSON payload")
	}
	data, err := h.service.Message(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) messages(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return httperror.BadRequest("Invalid session ID")
	}
	data, err := h.service.Messages(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func currentUser(c *fiber.Ctx) auth.CurrentUser {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		panic(err)
	}
	return user
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
