package runtime

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

func (h *Handler) registerJourneyPreviews(router fiber.Router) {
	view := auth.RequirePermission(authz.PermissionBotView)
	bounded := limiter.New(limiter.Config{Max: 60, Expiration: time.Minute, KeyGenerator: func(c *fiber.Ctx) string {
		u := currentUser(c)
		return u.OrganizationID.String() + ":" + u.ID.String() + ":journey-preview"
	}})
	router.Post("/journey-versions/:id/preview", view, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		var input struct {
			Scenario string `json:"scenario"`
		}
		if err := bind(c, &input); err != nil {
			return err
		}
		if input.Scenario == "" {
			input.Scenario = "normal"
		}
		data, err := h.service.StartJourneyPreview(c.UserContext(), currentUser(c), id, input.Scenario)
		return respond(c, data, err)
	})
	router.Post("/journey-previews/:id/answer", view, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		if len(c.Body()) > 16384 {
			return httperror.BadRequest("Preview answer exceeds the limit")
		}
		var input JourneyAnswer
		if err := bind(c, &input); err != nil {
			return err
		}
		data, err := h.service.AnswerJourneyPreview(c.UserContext(), currentUser(c), id, input)
		return respond(c, data, err)
	})
	router.Post("/journey-versions/:id/controlled-test", auth.RequirePermission(authz.PermissionBotPublish), bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		var input struct {
			Acknowledge bool `json:"acknowledge_test_records"`
		}
		if err := bind(c, &input); err != nil {
			return err
		}
		data, err := h.service.StartControlledJourneyTest(c.UserContext(), currentUser(c), id, input.Acknowledge)
		return respond(c, data, err)
	})
	router.Post("/journey-tests/:id/answer", auth.RequirePermission(authz.PermissionBotPublish), bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		if len(c.Body()) > 16384 {
			return httperror.BadRequest("Test answer exceeds the limit")
		}
		var input JourneyAnswer
		if err := bind(c, &input); err != nil {
			return err
		}
		data, err := h.service.AnswerControlledJourneyTest(c.UserContext(), currentUser(c), id, input)
		return respond(c, data, err)
	})
}
