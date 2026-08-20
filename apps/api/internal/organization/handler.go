package organization

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/tenant"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Current(c *fiber.Ctx) error {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		return err
	}

	scope, err := tenant.NewScope(user.OrganizationID)
	if err != nil {
		return err
	}

	org, err := h.repo.FindByID(c.UserContext(), scope, user.OrganizationID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": org})
}
