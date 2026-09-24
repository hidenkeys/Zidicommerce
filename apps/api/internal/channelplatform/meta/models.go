package meta

import (
	"time"

	"github.com/google/uuid"
)

const (
	WebhookPending  = "pending"
	WebhookVerified = "verified"
	WebhookActive   = "active"
)

type ConnectionState struct {
	ID                      uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID          uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_meta_connection_state" json:"organization_id"`
	ConnectionID            uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_meta_connection_state" json:"channel_connection_id"`
	Provider                string     `json:"provider"`
	WebhookStatus           string     `json:"webhook_status"`
	LastSignatureVerifiedAt *time.Time `json:"last_signature_verified_at,omitempty"`
	LastSignatureRejectedAt *time.Time `json:"last_signature_rejected_at,omitempty"`
	SignatureRejectionCount int64      `json:"signature_rejection_count"`
	LastInboundAt           *time.Time `json:"last_inbound_at,omitempty"`
	LastOutboundAt          *time.Time `json:"last_outbound_at,omitempty"`
	LastProviderError       string     `json:"last_provider_error,omitempty"`
	LastProviderErrorAt     *time.Time `json:"last_provider_error_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func (ConnectionState) TableName() string { return "channel_meta_connection_states" }

type ContactState struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_meta_contact_state" json:"organization_id"`
	ConnectionID           uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_meta_contact_state" json:"channel_connection_id"`
	Provider               string    `json:"provider"`
	ExternalCustomerID     string    `gorm:"uniqueIndex:idx_meta_contact_state" json:"external_customer_id"`
	LastInboundAt          time.Time `json:"last_inbound_at"`
	ConversationWindowEnds time.Time `gorm:"column:conversation_window_ends_at" json:"conversation_window_ends_at"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (ContactState) TableName() string { return "channel_meta_contact_states" }
