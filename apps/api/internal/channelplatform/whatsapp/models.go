package whatsapp

import (
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

const (
	Provider = "whatsapp"

	CredentialAccessToken = "access_token"
	CredentialAppSecret   = "app_secret"
	CredentialVerifyToken = "verify_token"

	WebhookNotConfigured = "not_configured"
	WebhookPending       = "pending"
	WebhookVerified      = "verified"
	WebhookFailed        = "failed"

	SetupNotConnected        = "not_connected"
	SetupStarted             = "setup_started"
	SetupCredentialsAdded    = "credentials_added"
	SetupWebhookVerified     = "webhook_verified"
	SetupPhoneVerified       = "phone_identity_verified"
	SetupTestMessageReady    = "test_message_ready"
	SetupTestMessageSent     = "test_message_sent"
	SetupInboundTestReceived = "inbound_test_received"
	SetupConnected           = "connected"
	SetupHealthy             = "healthy"
	SetupDegraded            = "degraded"
	SetupRequiresAttention   = "requires_attention"
	SetupDisconnected        = "disconnected"
	SetupCredentialExpired   = "credential_expired"
	SetupWebhookFailed       = "webhook_failed"

	ConnectionMethodManual         = "manual"
	ConnectionMethodEmbeddedSignup = "embedded_signup"
	ConnectionMethodAssisted       = "assisted"

	AuthorizationNotStarted              = "not_started"
	AuthorizationPending                 = "pending"
	AuthorizationAuthorized              = "authorized"
	AuthorizationRequiresReauthorization = "requires_reauthorization"
	AuthorizationRevoked                 = "revoked"
	AuthorizationFailed                  = "failed"

	AssistedNotRequested           = "not_requested"
	AssistedRequested              = "requested"
	AssistedInProgress             = "in_progress"
	AssistedAwaitingMerchantAction = "awaiting_merchant_action"
	AssistedConnected              = "connected"
	AssistedBlocked                = "blocked"
	AssistedCompleted              = "completed"

	SignupAttemptInitiated  = "initiated"
	SignupAttemptProcessing = "processing"
	SignupAttemptCompleted  = "completed"
	SignupAttemptFailed     = "failed"
	SignupAttemptExpired    = "expired"

	ConsentOptedIn  = "opted_in"
	ConsentOptedOut = "opted_out"
	ConsentUnknown  = "unknown"

	ConsentSourceInbound = "inbound_message"
	ConsentSourceManual  = "manual"

	TemplateDraft    = "draft"
	TemplatePending  = "pending"
	TemplateApproved = "approved"
	TemplateRejected = "rejected"
	TemplatePaused   = "paused"
	TemplateDisabled = "disabled"
	TemplateArchived = "archived"

	PolicyAllowedFreeform            = "allowed_freeform"
	PolicyAllowedTemplate            = "allowed_template"
	PolicyBlockedNoContactState      = "blocked_no_contact_state"
	PolicyBlockedOptedOut            = "blocked_opted_out"
	PolicyBlockedWindowClosed        = "blocked_window_closed"
	PolicyBlockedMissingTemplate     = "blocked_missing_template"
	PolicyBlockedTemplateNotApproved = "blocked_template_not_approved"
	PolicyBlockedTemplateVariables   = "blocked_template_variables_invalid"
	PolicyBlockedRateLimit           = "blocked_rate_limit"
	PolicyBlockedUnknown             = "blocked_unknown_reason"
)

var requiredCredentialTypes = []string{CredentialAccessToken, CredentialAppSecret, CredentialVerifyToken}

type Configuration struct {
	ID                        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID            uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_whatsapp_config_connection" json:"organization_id"`
	ConnectionID              uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_whatsapp_config_connection" json:"channel_connection_id"`
	PhoneNumberID             string     `json:"phone_number_id"`
	WhatsAppBusinessAccountID string     `gorm:"column:whatsapp_business_account_id" json:"whatsapp_business_account_id"`
	MetaBusinessAccountID     string     `json:"meta_business_account_id"`
	DisplayPhoneNumber        string     `json:"display_phone_number"`
	GraphAPIVersion           string     `json:"graph_api_version"`
	ConnectionMethod          string     `json:"connection_method"`
	AuthorizationStatus       string     `json:"authorization_status"`
	AuthorizedAt              *time.Time `json:"authorized_at,omitempty"`
	AuthorizationExpiresAt    *time.Time `json:"authorization_expires_at,omitempty"`
	LastAuthorizationError    string     `json:"last_authorization_error,omitempty"`
	AssistedSetupStatus       string     `json:"assisted_setup_status"`
	AssistedSetupNote         string     `json:"assisted_setup_note,omitempty"`
	WebhookStatus             string     `json:"webhook_status"`
	SetupState                string     `json:"setup_state"`
	LastWebhookVerifiedAt     *time.Time `json:"last_webhook_verified_at,omitempty"`
	LastWebhookAttemptAt      *time.Time `gorm:"column:last_webhook_verification_attempt_at" json:"last_webhook_verification_attempt_at,omitempty"`
	LastWebhookFailedAt       *time.Time `gorm:"column:last_webhook_verification_failed_at" json:"last_webhook_verification_failed_at,omitempty"`
	LastWebhookError          string     `gorm:"column:last_webhook_verification_error" json:"last_webhook_verification_error,omitempty"`
	LastSignatureVerifiedAt   *time.Time `json:"last_signature_verified_at,omitempty"`
	LastSignatureRejectedAt   *time.Time `json:"last_signature_rejected_at,omitempty"`
	LastInboundAt             *time.Time `json:"last_inbound_at,omitempty"`
	LastOutboundAt            *time.Time `json:"last_outbound_at,omitempty"`
	LastProviderFailureAt     *time.Time `json:"last_provider_failure_at,omitempty"`
	RateLimitedUntil          *time.Time `json:"rate_limited_until,omitempty"`
	LastTestMessageAt         *time.Time `json:"last_test_message_at,omitempty"`
	LastTestMessageID         string     `json:"last_test_message_id,omitempty"`
	LastTestMessageError      string     `json:"last_test_message_error,omitempty"`
	TestRecipientHash         string     `json:"-"`
	TestRecipientDisplay      string     `json:"test_recipient_display,omitempty"`
	LastInboundTestAt         *time.Time `json:"last_inbound_test_at,omitempty"`
	CredentialRotatedAt       *time.Time `json:"credential_rotated_at,omitempty"`
	Metadata                  string     `json:"-"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

func (Configuration) TableName() string { return "channel_whatsapp_configs" }

type CredentialStatus struct {
	CredentialType    string     `json:"credential_type"`
	Status            string     `json:"status"`
	Configured        bool       `json:"configured"`
	Resolvable        bool       `json:"resolvable"`
	ReferenceType     string     `json:"reference_type,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	LastValidatedAt   *time.Time `json:"last_validated_at,omitempty"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
	ResolutionProblem string     `json:"resolution_problem,omitempty"`
}

type SetupChecklist struct {
	PhoneIdentityConfigured bool `json:"phone_identity_configured"`
	BusinessAccountKnown    bool `json:"business_account_known"`
	AccessTokenConfigured   bool `json:"access_token_configured"`
	AppSecretConfigured     bool `json:"app_secret_configured"`
	VerifyTokenConfigured   bool `json:"verify_token_configured"`
	WebhookVerified         bool `json:"webhook_verified"`
	SignatureVerified       bool `json:"signature_verified"`
	TestMessageReady        bool `json:"test_message_ready"`
	TestMessageSent         bool `json:"test_message_sent"`
	InboundTestReceived     bool `json:"inbound_test_received"`
	ReadyToComplete         bool `json:"ready_to_complete"`
	ReadyForInbound         bool `json:"ready_for_inbound"`
	ReadyForOutbound        bool `json:"ready_for_outbound"`
}

type ConfigurationView struct {
	Configuration
	Connection              channelplatform.ConnectionView `json:"connection"`
	Credentials             []CredentialStatus             `json:"credentials"`
	Checklist               SetupChecklist                 `json:"checklist"`
	WebhookCallbackURL      string                         `json:"webhook_callback_url"`
	SignatureStatus         string                         `json:"signature_status"`
	NextAction              string                         `json:"next_action"`
	OperationalEvents       []OperationalEvent             `json:"operational_events"`
	LegacyCredentialsFound  bool                           `json:"legacy_credentials_found"`
	EncryptedStorageEnabled bool                           `json:"encrypted_storage_enabled"`
	EmbeddedSignupAvailable bool                           `json:"embedded_signup_available"`
}

type MetaSignupAttempt struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"-"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;index" json:"-"`
	ConnectionID      uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index" json:"-"`
	RequestedByUserID *uuid.UUID `gorm:"type:uuid" json:"-"`
	TokenHash         string     `json:"-"`
	Status            string     `json:"-"`
	FailureCode       string     `json:"-"`
	ExpiresAt         time.Time  `json:"-"`
	CompletedAt       *time.Time `json:"-"`
	CreatedAt         time.Time  `json:"-"`
	UpdatedAt         time.Time  `json:"-"`
}

func (MetaSignupAttempt) TableName() string { return "channel_meta_signup_attempts" }

type ContactState struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID         uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_whatsapp_contact_identity" json:"-"`
	ConnectionID           uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_whatsapp_contact_identity" json:"channel_connection_id"`
	CustomerID             *uuid.UUID `gorm:"type:uuid" json:"customer_id,omitempty"`
	ExternalCustomerIDHash string     `gorm:"uniqueIndex:idx_whatsapp_contact_identity" json:"-"`
	MaskedPhone            string     `json:"masked_phone"`
	ConsentStatus          string     `json:"consent_status"`
	ConsentSource          string     `json:"consent_source"`
	OptedInAt              *time.Time `json:"opted_in_at,omitempty"`
	OptedOutAt             *time.Time `json:"opted_out_at,omitempty"`
	LastInboundAt          *time.Time `json:"last_inbound_at,omitempty"`
	LastOutboundAt         *time.Time `json:"last_outbound_at,omitempty"`
	ServiceWindowExpiresAt *time.Time `json:"service_window_expires_at,omitempty"`
	Metadata               string     `json:"-"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (ContactState) TableName() string { return "whatsapp_contact_states" }

type MessageTemplate struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_whatsapp_template_name" json:"-"`
	ConnectionID       uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_whatsapp_template_name" json:"channel_connection_id"`
	ProviderTemplateID string     `json:"provider_template_id,omitempty"`
	Name               string     `gorm:"uniqueIndex:idx_whatsapp_template_name" json:"name"`
	Language           string     `gorm:"uniqueIndex:idx_whatsapp_template_name" json:"language"`
	Category           string     `json:"category"`
	Status             string     `json:"status"`
	Body               string     `json:"body"`
	HeaderMetadata     string     `json:"header_metadata"`
	FooterMetadata     string     `json:"footer_metadata"`
	ButtonsMetadata    string     `json:"buttons_metadata"`
	VariableSchema     string     `json:"variable_schema"`
	SampleValues       string     `json:"sample_values"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
	RejectionReason    string     `json:"rejection_reason,omitempty"`
	CreatedByUserID    *uuid.UUID `gorm:"type:uuid" json:"created_by_user_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (MessageTemplate) TableName() string { return "whatsapp_message_templates" }

type PolicyDecisionRecord struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_whatsapp_policy_idempotency" json:"-"`
	ConnectionID   uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_whatsapp_policy_idempotency" json:"channel_connection_id"`
	ContactStateID *uuid.UUID `gorm:"type:uuid" json:"contact_state_id,omitempty"`
	TemplateID     *uuid.UUID `gorm:"type:uuid" json:"template_id,omitempty"`
	MessageType    string     `json:"message_type"`
	Decision       string     `json:"decision"`
	Allowed        bool       `json:"allowed"`
	Reason         string     `json:"reason"`
	IdempotencyKey string     `gorm:"uniqueIndex:idx_whatsapp_policy_idempotency" json:"-"`
	Metadata       string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (PolicyDecisionRecord) TableName() string { return "whatsapp_policy_decisions" }

type OperationalMetricDaily struct {
	ID                        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID            uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_whatsapp_metric_day" json:"-"`
	ConnectionID              uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_whatsapp_metric_day" json:"channel_connection_id"`
	MetricDate                time.Time `gorm:"column:metric_date;type:date;uniqueIndex:idx_whatsapp_metric_day" json:"date"`
	FreeformSendCount         int64     `json:"freeform_send_count"`
	TemplateSendCount         int64     `json:"template_send_count"`
	PolicyBlockedCount        int64     `json:"policy_blocked_count"`
	WindowClosedBlockedCount  int64     `json:"window_closed_blocked_count"`
	RateLimitCount            int64     `json:"rate_limit_count"`
	InvalidRecipientCount     int64     `json:"invalid_recipient_count"`
	InvalidCredentialCount    int64     `json:"invalid_credential_count"`
	ProviderDeliveryLatencyMS int64     `json:"provider_delivery_latency_ms"`
	ProviderDeliverySamples   int64     `json:"provider_delivery_samples"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

func (OperationalMetricDaily) TableName() string { return "whatsapp_operational_metrics_daily" }

type EmbeddedSignupInitiation struct {
	AppID           string    `json:"app_id"`
	ConfigurationID string    `json:"configuration_id"`
	GraphAPIVersion string    `json:"graph_api_version"`
	AttemptToken    string    `json:"attempt_token"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type EmbeddedSignupCompletionInput struct {
	AttemptToken              string `json:"attempt_token"`
	AuthorizationCode         string `json:"authorization_code"`
	WhatsAppBusinessAccountID string `json:"whatsapp_business_account_id"`
	PhoneNumberID             string `json:"phone_number_id"`
}

type EmbeddedSignupCompletion struct {
	ConnectionID        uuid.UUID  `json:"channel_connection_id"`
	ConnectionStatus    string     `json:"connection_status"`
	AuthorizationStatus string     `json:"authorization_status"`
	BusinessName        string     `json:"business_name,omitempty"`
	DisplayPhoneNumber  string     `json:"display_phone_number,omitempty"`
	OwnershipModel      string     `json:"ownership_model"`
	AuthorizedAt        *time.Time `json:"authorized_at,omitempty"`
	NextAction          string     `json:"next_action"`
}

type AssistedSetupInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

type ConfigurationInput struct {
	PhoneNumberID             *string `json:"phone_number_id"`
	WhatsAppBusinessAccountID *string `json:"whatsapp_business_account_id"`
	MetaBusinessAccountID     *string `json:"meta_business_account_id"`
	DisplayPhoneNumber        *string `json:"display_phone_number"`
	GraphAPIVersion           *string `json:"graph_api_version"`
	AccessTokenReference      *string `json:"access_token_reference"`
	AppSecretReference        *string `json:"app_secret_reference"`
	VerifyTokenReference      *string `json:"verify_token_reference"`
	AccessTokenValue          *string `json:"access_token_value"`
	AppSecretValue            *string `json:"app_secret_value"`
	VerifyTokenValue          *string `json:"verify_token_value"`
}

type LegacyMigrationResult struct {
	ConnectionID        uuid.UUID `json:"channel_connection_id"`
	ConfigurationFound  bool      `json:"configuration_found"`
	CredentialsFound    int       `json:"credentials_found"`
	CredentialsMigrated int       `json:"credentials_migrated"`
	Encrypted           bool      `json:"encrypted"`
	LegacyRetained      bool      `json:"legacy_retained"`
}

type HealthResult struct {
	Status     string   `json:"status"`
	SetupState string   `json:"setup_state"`
	Issues     []string `json:"issues"`
}

type CredentialRotationInput struct {
	CredentialType  string  `json:"credential_type"`
	SecretReference *string `json:"secret_reference"`
	SecretValue     *string `json:"secret_value"`
}

type CredentialRotationResult struct {
	CredentialType string    `json:"credential_type"`
	Status         string    `json:"status"`
	Configured     bool      `json:"configured"`
	Rotated        bool      `json:"rotated"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TestMessageInput struct {
	Recipient         string            `json:"recipient"`
	Message           string            `json:"message"`
	TemplateID        *uuid.UUID        `json:"template_id,omitempty"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
}

type TestMessageResult struct {
	Status            string    `json:"status"`
	RecipientDisplay  string    `json:"recipient_display"`
	ProviderMessageID string    `json:"provider_message_id,omitempty"`
	SentAt            time.Time `json:"sent_at"`
}

type ContactStateInput struct {
	ConsentStatus string `json:"consent_status"`
	ConsentSource string `json:"consent_source"`
}

type MessageTemplateInput struct {
	ProviderTemplateID string            `json:"provider_template_id"`
	Name               string            `json:"name"`
	Language           string            `json:"language"`
	Category           string            `json:"category"`
	Status             string            `json:"status"`
	Body               string            `json:"body"`
	HeaderMetadata     map[string]any    `json:"header_metadata"`
	FooterMetadata     map[string]any    `json:"footer_metadata"`
	ButtonsMetadata    []map[string]any  `json:"buttons_metadata"`
	VariableSchema     []string          `json:"variable_schema"`
	SampleValues       map[string]string `json:"sample_values"`
	RejectionReason    string            `json:"rejection_reason"`
}

type MessageTemplateUpdate struct {
	ProviderTemplateID *string            `json:"provider_template_id"`
	Name               *string            `json:"name"`
	Language           *string            `json:"language"`
	Category           *string            `json:"category"`
	Status             *string            `json:"status"`
	Body               *string            `json:"body"`
	HeaderMetadata     *map[string]any    `json:"header_metadata"`
	FooterMetadata     *map[string]any    `json:"footer_metadata"`
	ButtonsMetadata    *[]map[string]any  `json:"buttons_metadata"`
	VariableSchema     *[]string          `json:"variable_schema"`
	SampleValues       *map[string]string `json:"sample_values"`
	RejectionReason    *string            `json:"rejection_reason"`
}

type MessageTemplateView struct {
	ID                 uuid.UUID         `json:"id"`
	ConnectionID       uuid.UUID         `json:"channel_connection_id"`
	ProviderTemplateID string            `json:"provider_template_id,omitempty"`
	Name               string            `json:"name"`
	Language           string            `json:"language"`
	Category           string            `json:"category"`
	Status             string            `json:"status"`
	Body               string            `json:"body"`
	HeaderMetadata     map[string]any    `json:"header_metadata"`
	FooterMetadata     map[string]any    `json:"footer_metadata"`
	ButtonsMetadata    []map[string]any  `json:"buttons_metadata"`
	VariableSchema     []string          `json:"variable_schema"`
	SampleValues       map[string]string `json:"sample_values"`
	LastSyncedAt       *time.Time        `json:"last_synced_at,omitempty"`
	RejectionReason    string            `json:"rejection_reason,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type TemplatePreviewInput struct {
	Variables map[string]string `json:"variables"`
}

type TemplatePreview struct {
	Body             string   `json:"body"`
	MissingVariables []string `json:"missing_variables"`
	Valid            bool     `json:"valid"`
}

type PolicyDecision struct {
	Decision               string     `json:"decision"`
	Allowed                bool       `json:"allowed"`
	Reason                 string     `json:"reason"`
	ServiceWindowExpiresAt *time.Time `json:"service_window_expires_at,omitempty"`
	TemplateID             *uuid.UUID `json:"template_id,omitempty"`
}

type ConversationPolicyContext struct {
	ConsentStatus          string     `json:"consent_status"`
	MaskedPhone            string     `json:"masked_phone,omitempty"`
	ServiceWindowOpen      bool       `json:"service_window_open"`
	LastInboundAt          *time.Time `json:"last_inbound_at,omitempty"`
	ServiceWindowExpiresAt *time.Time `json:"service_window_expires_at,omitempty"`
	TemplateRequired       bool       `json:"template_required"`
	CanSendFreeform        bool       `json:"can_send_freeform"`
	Reason                 string     `json:"reason"`
}

type AdvancedMetrics struct {
	InboundMessages           int64   `json:"inbound_messages"`
	OutboundMessages          int64   `json:"outbound_messages"`
	FreeformSends             int64   `json:"freeform_sends"`
	TemplateSends             int64   `json:"template_sends"`
	PolicyBlockedSends        int64   `json:"policy_blocked_sends"`
	FailedSends               int64   `json:"failed_sends"`
	Delivered                 int64   `json:"delivered"`
	Read                      int64   `json:"read"`
	ServiceWindowOpenContacts int64   `json:"service_window_open_contacts"`
	ServiceWindowClosedBlocks int64   `json:"service_window_closed_blocks"`
	OptedInContacts           int64   `json:"opted_in_contacts"`
	OptedOutContacts          int64   `json:"opted_out_contacts"`
	AIHandledConversations    int64   `json:"ai_handled_conversations"`
	HumanHandledConversations int64   `json:"human_handled_conversations"`
	HandoffRate               float64 `json:"handoff_rate"`
	AverageFirstResponseMS    int64   `json:"average_first_response_ms"`
	AverageProviderDeliveryMS int64   `json:"average_provider_delivery_ms"`
	ProviderErrors            int64   `json:"provider_errors"`
	RateLimits                int64   `json:"rate_limits"`
	InvalidRecipientErrors    int64   `json:"invalid_recipient_errors"`
	InvalidCredentialErrors   int64   `json:"invalid_credential_errors"`
}

type OperationalEvent struct {
	Type       string    `json:"type"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Guidance   string    `json:"guidance,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}
