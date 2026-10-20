package bot

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

func (h *Handler) registerJourneys(router fiber.Router) {
	view := auth.RequirePermission(authz.PermissionBotView)
	edit := auth.RequirePermission(authz.PermissionBotManage)
	publish := auth.RequirePermission(authz.PermissionBotPublish)
	bounded := limiter.New(limiter.Config{Max: 30, Expiration: time.Minute, KeyGenerator: func(c *fiber.Ctx) string {
		u := currentUser(c)
		return u.OrganizationID.String() + ":" + u.ID.String() + ":" + c.Path()
	}})
	router.Get("/journeys/templates", view, func(c *fiber.Ctx) error { return respond(c, JourneyTemplates(), nil) })
	router.Get("/journeys/step-library", view, func(c *fiber.Ctx) error { return respond(c, JourneyStepLibrary(), nil) })
	router.Get("/journeys", view, func(c *fiber.Ctx) error {
		data, err := h.service.ListJourneys(c.UserContext(), currentUser(c))
		return respond(c, data, err)
	})
	router.Post("/journeys", edit, bounded, func(c *fiber.Ctx) error {
		var input struct {
			Template string `json:"template"`
			Name     string `json:"name"`
		}
		if err := journeyBind(c, &input); err != nil {
			return err
		}
		data, err := h.service.CreateJourney(c.UserContext(), currentUser(c), input.Template, input.Name)
		return respond(c, data, err)
	})
	router.Get("/journeys/:id", view, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		var versionID *uuid.UUID
		if c.Query("version") != "" {
			v, err := uuid.Parse(c.Query("version"))
			if err != nil {
				return httperror.BadRequest("Invalid version")
			}
			versionID = &v
		}
		data, err := h.service.GetJourney(c.UserContext(), currentUser(c), id, versionID)
		return respond(c, data, err)
	})
	router.Delete("/journeys/:id", edit, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		return respond(c, map[string]bool{"archived": true}, h.service.ArchiveJourney(c.UserContext(), currentUser(c), id))
	})
	router.Post("/journeys/:id/drafts", edit, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		var input struct {
			SourceVersionID uuid.UUID `json:"source_version_id"`
		}
		if err := journeyBind(c, &input); err != nil {
			return err
		}
		data, err := h.service.CloneJourneyDraft(c.UserContext(), currentUser(c), id, input.SourceVersionID)
		return respond(c, data, err)
	})
	router.Post("/journeys/:id/activate", publish, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		var input struct {
			VersionID uuid.UUID `json:"version_id"`
			Mode      string    `json:"mode"`
			Paused    bool      `json:"paused"`
		}
		if err := journeyBind(c, &input); err != nil {
			return err
		}
		data, err := h.service.ActivateJourney(c.UserContext(), currentUser(c), id, input.VersionID, input.Mode, input.Paused)
		return respond(c, data, err)
	})
	router.Put("/journey-versions/:id", edit, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		var input JourneyDefinition
		if err := journeyBind(c, &input); err != nil {
			return err
		}
		data, err := h.service.SaveJourney(c.UserContext(), currentUser(c), id, input)
		return respond(c, data, err)
	})
	router.Post("/journey-versions/:id/validate", view, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		data, err := h.service.ValidateJourneyVersion(c.UserContext(), currentUser(c), id)
		return respond(c, data, err)
	})
	router.Post("/journey-versions/:id/publish", publish, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		data, err := h.service.PublishJourney(c.UserContext(), currentUser(c), id)
		return respond(c, data, err)
	})
	router.Post("/journey-versions/:id/compile", view, bounded, func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		config, err := h.service.GetConfiguration(c.UserContext(), currentUser(c), id)
		if err != nil {
			return err
		}
		return respond(c, CompileJourneyFlow(JourneyFromConfiguration(config)), nil)
	})
}
func journeyBind(c *fiber.Ctx, target any) error {
	if len(c.Body()) > 256*1024 {
		return httperror.BadRequest("Journey payload exceeds 256 KB")
	}
	return bind(c, target)
}
