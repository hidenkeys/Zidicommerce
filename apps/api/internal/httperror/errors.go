package httperror

import "github.com/gofiber/fiber/v2"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e APIError) Error() string {
	return e.Message
}

func New(status int, code string, message string) APIError {
	return APIError{StatusCode: status, Code: code, Message: message}
}

func BadRequest(message string) APIError {
	return New(fiber.StatusBadRequest, "BAD_REQUEST", message)
}

func Unauthorized(message string) APIError {
	return New(fiber.StatusUnauthorized, "UNAUTHORIZED", message)
}

func Forbidden(message string) APIError {
	return New(fiber.StatusForbidden, "FORBIDDEN", message)
}

func NotFound(message string) APIError {
	return New(fiber.StatusNotFound, "RESOURCE_NOT_FOUND", message)
}

func Conflict(message string) APIError {
	return New(fiber.StatusConflict, "CONFLICT", message)
}

func Unavailable(message string) APIError {
	return New(fiber.StatusServiceUnavailable, "EMAIL_DELIVERY_FAILED", message)
}

func Internal(message string) APIError {
	return New(fiber.StatusInternalServerError, "INTERNAL_ERROR", message)
}

func Handler(c *fiber.Ctx, err error) error {
	apiErr, ok := err.(APIError)
	if !ok {
		apiErr = Internal("An unexpected error occurred")
	}

	return c.Status(apiErr.StatusCode).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    apiErr.Code,
			"message": apiErr.Message,
		},
	})
}
