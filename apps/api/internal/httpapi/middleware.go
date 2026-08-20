package httpapi

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
)

func requestLogger(log *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set("X-Request-ID", requestID)

		start := time.Now()
		err := c.Next()

		attrs := []any{
			"request_id", requestID,
			"method", c.Method(),
			"route", c.Route().Path,
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if user, userErr := auth.GetCurrentUser(c); userErr == nil {
			attrs = append(attrs, "organization_id", user.OrganizationID.String(), "user_id", user.ID.String())
		}

		if err != nil {
			attrs = append(attrs, "error", err.Error())
			log.Warn("request failed", attrs...)
			return err
		}
		log.Info("request completed", attrs...)
		return nil
	}
}
