package httpapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func loginHandler(service *auth.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req loginRequest
		if err := c.BodyParser(&req); err != nil {
			return httperror.BadRequest("Invalid login payload")
		}

		token, user, err := service.Login(c.UserContext(), req.Email, req.Password)
		if err != nil {
			return err
		}

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"access_token": token,
				"user": fiber.Map{
					"id":              user.ID,
					"organization_id": user.OrganizationID,
					"email":           user.Email,
					"role":            user.Role,
				},
			},
		})
	}
}

func registerHandler(service *auth.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req auth.RegisterInput
		if err := c.BodyParser(&req); err != nil {
			return httperror.BadRequest("Invalid registration payload")
		}

		token, user, err := service.Register(c.UserContext(), req)
		if err != nil {
			return err
		}

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"access_token": token,
				"user": fiber.Map{
					"id":              user.ID,
					"organization_id": user.OrganizationID,
					"email":           user.Email,
					"role":            user.Role,
				},
			},
		})
	}
}

func meHandler(c *fiber.Ctx) error {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"id":              user.ID,
			"organization_id": user.OrganizationID,
			"role":            user.Role,
		},
	})
}
