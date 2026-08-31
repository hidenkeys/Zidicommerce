package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

func Middleware(tokens *TokenManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := strings.TrimSpace(c.Get("Authorization"))
		if header == "" {
			return httperror.Unauthorized("Missing Authorization header")
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return httperror.Unauthorized("Authorization header must be a Bearer token")
		}

		claims, err := tokens.Parse(parts[1])
		if err != nil {
			return httperror.Unauthorized("Invalid or expired token")
		}

		SetCurrentUser(c, CurrentUser{
			ID:             claims.UserID,
			OrganizationID: claims.OrganizationID,
			Role:           authz.Role(claims.Role),
		})
		return c.Next()
	}
}

func RequireRole(allowed ...authz.Role) fiber.Handler {
	allowedSet := make(map[authz.Role]struct{}, len(allowed))
	for _, role := range allowed {
		allowedSet[role] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		user, err := GetCurrentUser(c)
		if err != nil {
			return err
		}
		if _, ok := allowedSet[user.Role]; !ok {
			return httperror.Forbidden("You do not have permission to perform this action")
		}
		return c.Next()
	}
}

func RequirePermission(permission authz.Permission) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, err := GetCurrentUser(c)
		if err != nil {
			return err
		}
		if !user.Role.HasPermission(permission) {
			return httperror.Forbidden("You do not have permission to perform this action")
		}
		return c.Next()
	}
}
