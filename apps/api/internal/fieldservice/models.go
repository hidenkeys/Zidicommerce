package fieldservice

import (
	"time"

	"github.com/google/uuid"
)

const (
	RequestDraft            = "draft"
	RequestAwaitingPayment  = "awaiting_booking_fee"
	RequestMatching         = "matching"
	RequestDispatching      = "dispatching"
	RequestAssigned         = "assigned"
	RequestOnTheWay         = "on_the_way"
	RequestArrived          = "arrived"
	RequestInProgress       = "in_progress"
	RequestQuoteSent        = "quote_sent"
	RequestQuoteApproved    = "quote_approved"
	RequestPaymentConfirmed = "payment_confirmed"
	RequestCompleted        = "completed"
	RequestCancelled        = "cancelled"

	AvailabilityAvailable = "available"
	AvailabilityBusy      = "busy"
	AvailabilityOffline   = "offline"

	QuoteDraft    = "draft"
	QuoteSent     = "sent"
	QuoteApproved = "approved"
	QuoteDeclined = "declined"
	QuoteExpired  = "expired"
	QuotePaid     = "paid"

	DispatchNotified  = "notified"
	DispatchAccepted  = "accepted"
	DispatchDeclined  = "declined"
	DispatchTimeout   = "timeout"
	DispatchCancelled = "cancelled"
)

type Settings struct {
	OrganizationID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"organization_id"`
	BookingFeeMinor         int64     `json:"booking_fee_minor"`
	Currency                string    `json:"currency"`
	RequireBookingFee       bool      `json:"require_booking_fee"`
	BookingFeeRefundable    bool      `json:"booking_fee_refundable"`
	DispatchStrategy        string    `json:"dispatch_strategy"`
	AcceptanceWindowSeconds int       `json:"acceptance_window_seconds"`
	MaxDistanceKM           float64   `gorm:"column:max_distance_km" json:"max_distance_km"`
	WeightService           float64   `json:"weight_service"`
	WeightAvailability      float64   `json:"weight_availability"`
	WeightDistance          float64   `json:"weight_distance"`
	WeightRating            float64   `json:"weight_rating"`
	WeightExperience        float64   `json:"weight_experience"`
	WelcomeMessage          string    `json:"welcome_message"`
	CompanyDisplayName      string    `json:"company_display_name"`
	ProviderPortalBaseURL   string    `json:"provider_portal_base_url"`
	Metadata                string    `json:"metadata"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (Settings) TableName() string { return "service_org_settings" }

type Pool struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	Slug           string    `json:"slug"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	SortOrder      int       `json:"sort_order"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Pool) TableName() string { return "service_pools" }

type Provider struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	UserID           *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`
	PublicCode       string     `json:"public_code"`
	Name             string     `json:"name"`
	Phone            string     `json:"phone"`
	WhatsAppNumber   string     `gorm:"column:whatsapp_number" json:"whatsapp_number"`
	RatingAverage    float64    `json:"rating_average"`
	JobsCompleted    int        `json:"jobs_completed"`
	Area             string     `json:"area"`
	Address          string     `json:"address"`
	Latitude         *float64   `json:"latitude,omitempty"`
	Longitude        *float64   `json:"longitude,omitempty"`
	Availability     string     `json:"availability"`
	Status           string     `json:"status"`
	CurrentJobStatus string     `json:"current_job_status"`
	ProfileImageURL  string     `json:"profile_image_url"`
	Email            string     `gorm:"-" json:"email,omitempty"`
	Metadata         string     `json:"metadata"`
	Pools            []Pool     `gorm:"many2many:service_provider_pools;joinForeignKey:ProviderID;joinReferences:PoolID" json:"pools,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (Provider) TableName() string { return "service_providers" }

type ProviderPool struct {
	OrganizationID uuid.UUID `gorm:"type:uuid;primaryKey" json:"organization_id"`
	ProviderID     uuid.UUID `gorm:"type:uuid;primaryKey" json:"provider_id"`
	PoolID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"pool_id"`
}

func (ProviderPool) TableName() string { return "service_provider_pools" }

type Request struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID        uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	PublicCode            string     `json:"public_code"`
	CustomerID            uuid.UUID  `gorm:"type:uuid" json:"customer_id"`
	PoolID                uuid.UUID  `gorm:"type:uuid" json:"pool_id"`
	SessionID             *uuid.UUID `gorm:"type:uuid" json:"session_id,omitempty"`
	ChannelID             *uuid.UUID `gorm:"type:uuid" json:"channel_id,omitempty"`
	CustomerName          string     `json:"customer_name"`
	CustomerPhone         string     `json:"customer_phone"`
	Area                  string     `json:"area"`
	Address               string     `json:"address"`
	Latitude              *float64   `json:"latitude,omitempty"`
	Longitude             *float64   `json:"longitude,omitempty"`
	Description           string     `json:"description"`
	PreferredAt           string     `json:"preferred_at"`
	MediaURL              string     `json:"media_url"`
	Status                string     `json:"status"`
	BookingFeeMinor       int64      `json:"booking_fee_minor"`
	BookingOrderID        *uuid.UUID `gorm:"type:uuid" json:"booking_order_id,omitempty"`
	AssignedProviderID    *uuid.UUID `gorm:"type:uuid" json:"assigned_provider_id,omitempty"`
	ConversationSessionID *uuid.UUID `gorm:"type:uuid" json:"conversation_session_id,omitempty"`
	HandoffID             *uuid.UUID `gorm:"type:uuid" json:"handoff_id,omitempty"`
	Metadata              string     `json:"metadata"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	Pool                  Pool       `gorm:"foreignKey:PoolID" json:"pool,omitempty"`
	AssignedProvider      *Provider  `gorm:"foreignKey:AssignedProviderID" json:"assigned_provider,omitempty"`
}

func (Request) TableName() string { return "service_requests" }

type Match struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	RequestID      uuid.UUID `gorm:"type:uuid;index" json:"request_id"`
	ProviderID     uuid.UUID `gorm:"type:uuid" json:"provider_id"`
	Rank           int       `json:"rank"`
	Score          float64   `json:"score"`
	Breakdown      string    `json:"breakdown"`
	CreatedAt      time.Time `json:"created_at"`
	Provider       Provider  `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (Match) TableName() string { return "service_matches" }

type DispatchAttempt struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	RequestID      uuid.UUID  `gorm:"type:uuid;index" json:"request_id"`
	ProviderID     uuid.UUID  `gorm:"type:uuid" json:"provider_id"`
	Status         string     `json:"status"`
	NotifiedAt     time.Time  `json:"notified_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RespondedAt    *time.Time `json:"responded_at,omitempty"`
	Metadata       string     `json:"metadata"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Provider       Provider   `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
	Request        Request    `gorm:"foreignKey:RequestID" json:"request,omitempty"`
}

func (DispatchAttempt) TableName() string { return "service_dispatch_attempts" }

type Assignment struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	RequestID      uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"request_id"`
	ProviderID     uuid.UUID `gorm:"type:uuid" json:"provider_id"`
	Status         string    `json:"status"`
	Notes          string    `json:"notes"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Provider       Provider  `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
	Request        Request   `gorm:"foreignKey:RequestID" json:"request,omitempty"`
}

func (Assignment) TableName() string { return "service_assignments" }

type Quote struct {
	ID              uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID   `gorm:"type:uuid;index" json:"organization_id"`
	RequestID       uuid.UUID   `gorm:"type:uuid;index" json:"request_id"`
	AssignmentID    uuid.UUID   `gorm:"type:uuid" json:"assignment_id"`
	ProviderID      uuid.UUID   `gorm:"type:uuid" json:"provider_id"`
	PublicCode      string      `json:"public_code"`
	LabourMinor     int64       `json:"labour_minor"`
	MaterialsMinor  int64       `json:"materials_minor"`
	AdditionalMinor int64       `json:"additional_minor"`
	DiscountMinor   int64       `json:"discount_minor"`
	TotalMinor      int64       `json:"total_minor"`
	Currency        string      `json:"currency"`
	Notes           string      `json:"notes"`
	Status          string      `json:"status"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty"`
	PaymentOrderID  *uuid.UUID  `gorm:"type:uuid" json:"payment_order_id,omitempty"`
	Metadata        string      `json:"metadata"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	Items           []QuoteItem `gorm:"foreignKey:QuoteID" json:"items,omitempty"`
}

func (Quote) TableName() string { return "service_quotes" }

type QuoteItem struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid" json:"organization_id"`
	QuoteID        uuid.UUID `gorm:"type:uuid;index" json:"quote_id"`
	Kind           string    `json:"kind"`
	Description    string    `json:"description"`
	AmountMinor    int64     `json:"amount_minor"`
	SortOrder      int       `json:"sort_order"`
}

func (QuoteItem) TableName() string { return "service_quote_items" }

type Rating struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	RequestID      uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"request_id"`
	ProviderID     uuid.UUID `gorm:"type:uuid" json:"provider_id"`
	CustomerID     uuid.UUID `gorm:"type:uuid" json:"customer_id"`
	Score          int       `json:"score"`
	Feedback       string    `json:"feedback"`
	CreatedAt      time.Time `json:"created_at"`
}

func (Rating) TableName() string { return "service_ratings" }

type Message struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	RequestID      uuid.UUID  `gorm:"type:uuid;index" json:"request_id"`
	AuthorType     string     `json:"author_type"`
	AuthorUserID   *uuid.UUID `gorm:"type:uuid" json:"author_user_id,omitempty"`
	Body           string     `json:"body"`
	Metadata       string     `json:"metadata"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (Message) TableName() string { return "service_messages" }
