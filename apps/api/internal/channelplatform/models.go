package channelplatform

import (
	"time"

	"github.com/google/uuid"
)

const (
	OwnershipMerchantManaged = "merchant_managed"
	OwnershipZidiManaged     = "zidi_managed"

	EnvironmentSandbox    = "sandbox"
	EnvironmentProduction = "production"

	StatusNotConnected      = "not_connected"
	StatusSetupRequired     = "setup_required"
	StatusConnecting        = "connecting"
	StatusConnected         = "connected"
	StatusHealthy           = "healthy"
	StatusDegraded          = "degraded"
	StatusDisconnected      = "disconnected"
	StatusRequiresAttention = "requires_attention"
	StatusArchived          = "archived"

	CredentialMissing                 = "missing"
	CredentialPresent                 = "present"
	CredentialExpiring                = "expiring"
	CredentialExpired                 = "expired"
	CredentialRevoked                 = "revoked"
	CredentialRequiresReauthorization = "requires_reauthorization"

	HealthHealthy  = "healthy"
	HealthDegraded = "degraded"
	HealthFailed   = "failed"
)

var KnownCapabilities = []string{
	"inbound_messages",
	"outbound_messages",
	"media",
	"templates",
	"delivery_receipts",
	"analytics",
}

// ChannelConnection intentionally maps to the existing channels table so
// conversation and outbound-message foreign keys remain stable.
type ChannelConnection struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	Provider          string     `json:"provider"`
	DisplayName       string     `json:"display_name"`
	Status            string     `json:"status"`
	OwnershipModel    string     `json:"ownership_model"`
	Environment       string     `json:"environment"`
	Capabilities      string     `json:"-"`
	CreatedByUserID   *uuid.UUID `gorm:"type:uuid" json:"created_by_user_id,omitempty"`
	LastConnectedAt   *time.Time `json:"last_connected_at,omitempty"`
	LastHealthCheckAt *time.Time `json:"last_health_check_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (ChannelConnection) TableName() string { return "channels" }

type ProviderAccount struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	ConnectionID      uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index" json:"channel_connection_id"`
	Provider          string    `json:"provider"`
	ProviderAccountID string    `json:"provider_account_id"`
	BusinessName      string    `json:"business_name"`
	DisplayName       string    `json:"display_name"`
	OwnershipModel    string    `json:"ownership_model"`
	Status            string    `json:"status"`
	Metadata          string    `json:"metadata"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (ProviderAccount) TableName() string { return "channel_provider_accounts" }

type ChannelIdentity struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	ConnectionID       uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index" json:"channel_connection_id"`
	Provider           string    `json:"provider"`
	IdentityType       string    `json:"identity_type"`
	DisplayName        string    `json:"display_name"`
	ProviderIdentityID string    `json:"provider_identity_id"`
	ExternalHandle     string    `json:"external_handle"`
	Status             string    `json:"status"`
	Metadata           string    `json:"metadata"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (ChannelIdentity) TableName() string { return "channel_identities" }

type CredentialReference struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_channel_credential_type" json:"organization_id"`
	ConnectionID    uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_channel_credential_type" json:"channel_connection_id"`
	Provider        string     `json:"provider"`
	CredentialType  string     `gorm:"uniqueIndex:idx_channel_credential_type" json:"credential_type"`
	OwnershipModel  string     `json:"ownership_model"`
	SecretRef       string     `json:"-"`
	Status          string     `json:"status"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastValidatedAt *time.Time `json:"last_validated_at,omitempty"`
	Metadata        string     `json:"metadata"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (CredentialReference) TableName() string { return "channel_credential_references" }

type HealthCheck struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	ConnectionID   uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index" json:"channel_connection_id"`
	Status         string    `json:"status"`
	CheckedAt      time.Time `json:"checked_at"`
	LatencyMS      int64     `json:"latency_ms"`
	ErrorCode      string    `json:"error_code"`
	ErrorMessage   string    `json:"error_message"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
}

func (HealthCheck) TableName() string { return "channel_health_checks" }

type ProviderEvent struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_channel_event_external;uniqueIndex:idx_channel_event_idempotency" json:"organization_id"`
	ConnectionID     uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index" json:"channel_connection_id"`
	Provider         string    `gorm:"uniqueIndex:idx_channel_event_external" json:"provider"`
	EventType        string    `json:"event_type"`
	ProviderEventID  string    `gorm:"uniqueIndex:idx_channel_event_external" json:"provider_event_id"`
	ReceivedAt       time.Time `json:"received_at"`
	NormalizedStatus string    `json:"normalized_status"`
	IdempotencyKey   string    `gorm:"uniqueIndex:idx_channel_event_idempotency" json:"idempotency_key"`
	PayloadMetadata  string    `json:"payload_metadata"`
	ProcessingError  string    `json:"processing_error"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (ProviderEvent) TableName() string { return "channel_provider_events" }

type MetricDaily struct {
	ID                        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID            uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_channel_metric_day" json:"organization_id"`
	ConnectionID              uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_channel_metric_day" json:"channel_connection_id"`
	MetricDate                time.Time `gorm:"column:metric_date;type:date;uniqueIndex:idx_channel_metric_day" json:"date"`
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
	Metadata                  string    `json:"metadata"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

func (MetricDaily) TableName() string { return "channel_metrics_daily" }
