package auth

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

const currentUserKey = "current_user"

type CurrentUser struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Role           authz.Role
}

func SetCurrentUser(c *fiber.Ctx, user CurrentUser) {
	c.Locals(currentUserKey, user)
}

func GetCurrentUser(c *fiber.Ctx) (CurrentUser, error) {
	user, ok := c.Locals(currentUserKey).(CurrentUser)
	if !ok {
		return CurrentUser{}, httperror.Unauthorized("Authentication is required")
	}
	return user, nil
}
