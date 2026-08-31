package ai

import (
	"time"

	"github.com/google/uuid"
)

type StartInput struct {
	OrganizationID uuid.UUID  `json:"organization_id"`
	CustomerID     *uuid.UUID `json:"customer_id,omitempty"`
}

type MessageInput struct {
	SessionID uuid.UUID `json:"session_id"`
	Text      string    `json:"text"`
}

type SessionResponse struct {
	SessionID      uuid.UUID  `json:"session_id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	CustomerID     *uuid.UUID `json:"customer_id,omitempty"`
}

type ChatResponse struct {
	SessionID      uuid.UUID        `json:"session_id"`
	OrganizationID uuid.UUID        `json:"organization_id"`
	CustomerID     *uuid.UUID       `json:"customer_id,omitempty"`
	Message        ConversationRow  `json:"message"`
	Debug          InteractionDebug `json:"debug"`
}

type ConversationRow struct {
	ID        uuid.UUID `json:"id"`
	Role      string    `json:"role"`
	Body      string    `json:"body"`
	Metadata  string    `json:"metadata"`
	CreatedAt time.Time `json:"created_at"`
}

type InteractionDebug struct {
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	LatencyMS int64           `json:"latency_ms"`
	Tools     []ToolCallDebug `json:"tools"`
	Error     string          `json:"error,omitempty"`
}

type ToolCallDebug struct {
	Name      string         `json:"name"`
	Inputs    map[string]any `json:"inputs"`
	Result    map[string]any `json:"result,omitempty"`
	Status    string         `json:"status"`
	Error     string         `json:"error,omitempty"`
	LatencyMS int64          `json:"latency_ms"`
}
