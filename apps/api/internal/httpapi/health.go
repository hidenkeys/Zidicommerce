package httpapi

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Pinger interface {
	PingContext(ctx context.Context) error
}

func healthHandler(db Pinger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := db.PingContext(ctx); err != nil {
			dbStatus = "unavailable"
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "degraded",
				"checks": fiber.Map{"database": dbStatus},
			})
		}

		return c.JSON(fiber.Map{
			"status": "ok",
			"checks": fiber.Map{"database": dbStatus},
		})
	}
}
