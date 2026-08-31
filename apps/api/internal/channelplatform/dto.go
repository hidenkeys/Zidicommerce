package channelplatform

import (
	"time"

	"github.com/google/uuid"
)

type CreateConnectionInput struct {
	Provider       string   `json:"provider"`
	DisplayName    string   `json:"display_name"`
	Status         string   `json:"status"`
	OwnershipModel string   `json:"ownership_model"`
	Environment    string   `json:"environment"`
	Capabilities   []string `json:"capabilities"`
}

type UpdateConnectionInput struct {
	DisplayName    *string   `json:"display_name"`
	Status         *string   `json:"status"`
	OwnershipModel *string   `json:"ownership_model"`
	Environment    *string   `json:"environment"`
	Capabilities   *[]string `json:"capabilities"`
}

type CredentialReferenceInput struct {
	CredentialType  string     `json:"credential_type"`
	SecretRef       string     `json:"secret_ref"`
	Status          string     `json:"status"`
	ExpiresAt       *time.Time `json:"expires_at"`
	LastValidatedAt *time.Time `json:"last_validated_at"`
	Metadata        string     `json:"metadata"`
}

type CredentialReferenceView struct {
	ID                 uuid.UUID  `json:"id"`
	ConnectionID       uuid.UUID  `json:"channel_connection_id"`
	Provider           string     `json:"provider"`
	CredentialType     string     `json:"credential_type"`
	OwnershipModel     string     `json:"ownership_model"`
	Status             string     `json:"status"`
	HasSecretReference bool       `json:"has_secret_reference"`
	ReferenceType      string     `json:"reference_type,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	LastValidatedAt    *time.Time `json:"last_validated_at,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ConnectionView struct {
	ChannelConnection
	Capabilities     []string `json:"capabilities"`
	HealthStatus     string   `json:"health_status"`
	CredentialStatus string   `json:"credential_status"`
	IdentityCount    int      `json:"identity_count"`
}

type ConnectionDetail struct {
	Connection  ConnectionView            `json:"connection"`
	Accounts    []ProviderAccount         `json:"provider_accounts"`
	Identities  []ChannelIdentity         `json:"identities"`
	Credentials []CredentialReferenceView `json:"credentials"`
	Health      HealthSummary             `json:"health"`
	Metrics     MetricsSummary            `json:"metrics"`
}

type HealthSummary struct {
	ConnectionID  uuid.UUID    `json:"channel_connection_id"`
	Status        string       `json:"status"`
	LastCheck     *HealthCheck `json:"last_check,omitempty"`
	HealthyCount  int64        `json:"healthy_count"`
	DegradedCount int64        `json:"degraded_count"`
	FailedCount   int64        `json:"failed_count"`
}

type MetricsSummary struct {
	ConnectionID              uuid.UUID `json:"channel_connection_id"`
	From                      time.Time `json:"from"`
	To                        time.Time `json:"to"`
	InboundCount              int64     `json:"inbound_count"`
	OutboundCount             int64     `json:"outbound_count"`
	FailedOutboundCount       int64     `json:"failed_outbound_count"`
	DeliveredCount            int64     `json:"delivered_count"`
	ReadCount                 int64     `json:"read_count"`
	ConversationsStarted      int64     `json:"conversations_started"`
	ConversationsHumanHandled int64     `json:"conversations_human_handled"`
	ConversationsAIHandled    int64     `json:"conversations_ai_handled"`
	AverageResponseMS         int64     `json:"average_response_ms"`
	ProviderErrorCount        int64     `json:"provider_error_count"`
	Days                      int64     `json:"days"`
}

type HealthCheckInput struct {
	Status       string `json:"status"`
	LatencyMS    int64  `json:"latency_ms"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Metadata     string `json:"metadata"`
}

type ProviderEventInput struct {
	OrganizationID   uuid.UUID
	ConnectionID     uuid.UUID
	Provider         string
	EventType        string
	ProviderEventID  string
	ReceivedAt       time.Time
	NormalizedStatus string
	IdempotencyKey   string
	PayloadMetadata  map[string]any
	ProcessingError  string
}

type MetricInput struct {
	OrganizationID            uuid.UUID
	ConnectionID              uuid.UUID
	MetricDate                time.Time
	InboundCount              int64
	OutboundCount             int64
	FailedOutboundCount       int64
	DeliveredCount            int64
	ReadCount                 int64
	ConversationsStarted      int64
	ConversationsHumanHandled int64
	ConversationsAIHandled    int64
	AverageResponseMS         int64
	ProviderErrorCount        int64
	Metadata                  map[string]any
}
