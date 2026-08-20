package runtime

import "fmt"

const (
	ErrBotNotFound               = "BOT_NOT_FOUND"
	ErrNoPublishedVersion        = "NO_PUBLISHED_VERSION"
	ErrSessionNotFound           = "SESSION_NOT_FOUND"
	ErrInvalidInput              = "INVALID_INPUT"
	ErrInvalidStep               = "INVALID_STEP"
	ErrInvalidVariable           = "INVALID_VARIABLE"
	ErrActionNotFound            = "ACTION_NOT_FOUND"
	ErrActionFailed              = "ACTION_FAILED"
	ErrIntegrationNotConfigured  = "INTEGRATION_NOT_CONFIGURED"
	ErrSessionConflict           = "SESSION_CONFLICT"
	ErrRuntimeConfigurationError = "RUNTIME_CONFIGURATION_ERROR"
	ErrChannelNotFound           = "CHANNEL_NOT_FOUND"
	ErrUnauthorizedWebhook       = "UNAUTHORIZED_WEBHOOK"
)

type RuntimeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"-"`
}

func (e RuntimeError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func runtimeError(code, message string) RuntimeError {
	return RuntimeError{Code: code, Message: message}
}

func runtimeErrorf(code, message, format string, args ...any) RuntimeError {
	return RuntimeError{Code: code, Message: message, Detail: fmt.Sprintf(format, args...)}
}
