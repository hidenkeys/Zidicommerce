package runtime

import (
	"time"

	"github.com/google/uuid"
)

const (
	SessionActive    = "active"
	SessionCompleted = "completed"
	SessionHandoff   = "handoff"
	SessionExpired   = "expired"
	SessionCancelled = "cancelled"

	ConversationOpen           = "open"
	ConversationAIHandling     = "ai_handling"
	ConversationHumanRequested = "human_requested"
	ConversationHumanAssigned  = "human_assigned"
	ConversationPending        = "pending"
	ConversationWaiting        = "waiting"
	ConversationResolved       = "resolved"
	ConversationReopened       = "reopened"

	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
	DirectionInternal = "internal"

	MessageText            = "text"
	MessageButtons         = "buttons"
	MessageList            = "list"
	MessageImage           = "image"
	MessageLocationRequest = "location_request"

	EventConversationStarted   = "conversation_started"
	EventMessageReceived       = "message_received"
	EventStepExecuted          = "step_executed"
	EventQuestionPresented     = "question_presented"
	EventAnswerReceived        = "answer_received"
	EventActionStarted         = "action_started"
	EventActionRequested       = "action_requested"
	EventActionAuthorized      = "action_authorized"
	EventActionDenied          = "action_denied"
	EventActionCompleted       = "action_completed"
	EventActionFailed          = "action_failed"
	EventModuleStarted         = "module_started"
	EventModuleCompleted       = "module_completed"
	EventConversationCompleted = "conversation_completed"
	EventHandoffStarted        = "handoff_started"
	EventHandoffClaimed        = "handoff_claimed"
	EventHandoffAssigned       = "handoff_assigned"
	EventHandoffReleased       = "handoff_released"
	EventHandoffResolved       = "handoff_resolved"
	EventHandoffReopened       = "handoff_reopened"
	EventConversationRead      = "conversation_read"
	EventConversationUnread    = "conversation_unread"
	EventCommerceOrderLinked   = "commerce_order_linked"
	EventCommercePaymentPaid   = "commerce_payment_confirmed"
	EventRuntimeError          = "runtime_error"

	OutboundQueued            = "queued"
	OutboundSending           = "sending"
	OutboundSent              = "sent"
	OutboundDelivered         = "delivered"
	OutboundRead              = "read"
	OutboundFailed            = "failed"
	OutboundRetryPending      = "retry_pending"
	OutboundFailedPermanently = "failed_permanently"
	OutboundSkipped           = "skipped"
)

type ConversationSession struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_runtime_session_external" json:"organization_id"`
	BotID                  uuid.UUID  `gorm:"type:uuid;index" json:"bot_id"`
	BotVersionID           uuid.UUID  `gorm:"type:uuid;index" json:"bot_version_id"`
	ChannelID              uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_runtime_session_external" json:"channel_id"`
	CustomerID             *uuid.UUID `gorm:"type:uuid" json:"customer_id,omitempty"`
	StoreID                *uuid.UUID `gorm:"type:uuid;index" json:"store_id,omitempty"`
	ExternalConversationID string     `gorm:"uniqueIndex:idx_runtime_session_external" json:"external_conversation_id"`
	CurrentStepKey         string     `json:"current_step_key"`
	ExpectedInput          string     `json:"expected_input"`
	Status                 string     `json:"status"`
	ConversationStatus     string     `json:"conversation_status"`
	AssignedUserID         *uuid.UUID `gorm:"type:uuid;index" json:"assigned_user_id,omitempty"`
	Priority               string     `json:"priority"`
	HandoffState           string     `json:"handoff_state"`
	UnreadCount            int        `json:"unread_count"`
	LastMessageBody        string     `json:"last_message_body"`
	LastMessageDirection   string     `json:"last_message_direction"`
	Variables              string     `json:"variables"`
	SystemContext          string     `json:"system_context"`
	LockVersion            int        `json:"lock_version"`
	LastMessageAt          *time.Time `json:"last_message_at,omitempty"`
	LastReadAt             *time.Time `json:"last_read_at,omitempty"`
	HumanOwnedAt           *time.Time `json:"human_owned_at,omitempty"`
	HumanReleasedAt        *time.Time `json:"human_released_at,omitempty"`
	ResolvedAt             *time.Time `json:"resolved_at,omitempty"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (ConversationSession) TableName() string { return "conversation_sessions" }

func (s ConversationSession) AIShouldPause() bool {
	return s.Status == SessionHandoff && (s.ConversationStatus == ConversationHumanRequested || s.ConversationStatus == ConversationHumanAssigned || s.HandoffState == "open" || s.HandoffState == "assigned" || s.HandoffState == "reopened" || s.AssignedUserID != nil || s.HumanOwnedAt != nil)
}

func ActiveHandoffStatuses() []string {
	statuses := make([]string, len(activeHandoffStatuses))
	copy(statuses, activeHandoffStatuses)
	return statuses
}

type ConversationMessage struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	SessionID         uuid.UUID `gorm:"type:uuid;index" json:"session_id"`
	ChannelID         uuid.UUID `gorm:"type:uuid;index" json:"channel_id"`
	ExternalMessageID string    `json:"external_message_id"`
	Direction         string    `json:"direction"`
	MessageType       string    `json:"message_type"`
	Sender            string    `json:"sender"`
	Body              string    `json:"body"`
	Metadata          string    `json:"metadata"`
	CreatedAt         time.Time `json:"created_at"`
}

func (ConversationMessage) TableName() string { return "conversation_messages" }

type ProcessedMessage struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_runtime_processed_external" json:"organization_id"`
	ChannelID              uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_runtime_processed_external" json:"channel_id"`
	SessionID              *uuid.UUID `gorm:"type:uuid" json:"session_id,omitempty"`
	ExternalMessageID      string     `gorm:"uniqueIndex:idx_runtime_processed_external" json:"external_message_id"`
	ExternalConversationID string     `json:"external_conversation_id"`
	Status                 string     `json:"status"`
	Result                 string     `json:"result"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (ProcessedMessage) TableName() string { return "processed_messages" }

type RuntimeEvent struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	SessionID      *uuid.UUID `gorm:"type:uuid;index" json:"session_id,omitempty"`
	BotID          *uuid.UUID `gorm:"type:uuid" json:"bot_id,omitempty"`
	BotVersionID   *uuid.UUID `gorm:"type:uuid" json:"bot_version_id,omitempty"`
	ChannelID      *uuid.UUID `gorm:"type:uuid" json:"channel_id,omitempty"`
	EventType      string     `json:"event_type"`
	Severity       string     `json:"severity"`
	StepKey        string     `json:"step_key"`
	ActionKey      string     `json:"action_key"`
	Metadata       string     `json:"metadata"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (RuntimeEvent) TableName() string { return "runtime_events" }

type ChannelOutboundMessage struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	ChannelID              uuid.UUID  `gorm:"type:uuid;index" json:"channel_id"`
	SessionID              *uuid.UUID `gorm:"type:uuid;index" json:"session_id,omitempty"`
	ExternalConversationID string     `json:"external_conversation_id"`
	Recipient              string     `json:"recipient"`
	Provider               string     `json:"provider"`
	MessageType            string     `json:"message_type"`
	Status                 string     `json:"status"`
	Payload                string     `json:"payload"`
	IdempotencyKey         string     `json:"idempotency_key"`
	ProviderMessageID      string     `json:"provider_message_id"`
	ProviderResponse       string     `json:"provider_response"`
	ErrorMessage           string     `json:"error_message"`
	Attempts               int        `json:"attempts"`
	NextAttemptAt          *time.Time `json:"next_attempt_at,omitempty"`
	SentAt                 *time.Time `json:"sent_at,omitempty"`
	DeliveredAt            *time.Time `json:"delivered_at,omitempty"`
	ReadAt                 *time.Time `json:"read_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (ChannelOutboundMessage) TableName() string { return "channel_outbound_messages" }

type SupportHandoff struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	SessionID      uuid.UUID  `gorm:"type:uuid;index" json:"session_id"`
	CustomerID     *uuid.UUID `gorm:"type:uuid;index" json:"customer_id,omitempty"`
	OrderID        *uuid.UUID `gorm:"type:uuid;index" json:"order_id,omitempty"`
	AssignedUserID *uuid.UUID `gorm:"type:uuid" json:"assigned_user_id,omitempty"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason"`
	Priority       string     `json:"priority"`
	Metadata       string     `json:"metadata"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (SupportHandoff) TableName() string { return "support_handoffs" }

type SupportTicket struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	SessionID      uuid.UUID  `gorm:"type:uuid;index" json:"session_id"`
	CustomerID     *uuid.UUID `gorm:"type:uuid;index" json:"customer_id,omitempty"`
	OrderID        *uuid.UUID `gorm:"type:uuid;index" json:"order_id,omitempty"`
	AssignedUserID *uuid.UUID `gorm:"type:uuid" json:"assigned_user_id,omitempty"`
	TicketType     string     `json:"ticket_type"`
	Status         string     `json:"status"`
	Subject        string     `json:"subject"`
	Description    string     `json:"description"`
	MediaURL       string     `json:"media_url"`
	Metadata       string     `json:"metadata"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (SupportTicket) TableName() string { return "support_tickets" }

type SupportHandoffNote struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	HandoffID      uuid.UUID `gorm:"type:uuid;index" json:"handoff_id"`
	ActorUserID    uuid.UUID `gorm:"type:uuid;index" json:"actor_user_id"`
	Note           string    `json:"note"`
	Internal       bool      `json:"internal"`
	CreatedAt      time.Time `json:"created_at"`
}

func (SupportHandoffNote) TableName() string { return "support_handoff_notes" }
