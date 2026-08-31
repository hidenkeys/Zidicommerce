package bot

import (
	"strings"

	"github.com/google/uuid"
)

type BotInput struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	DefaultLanguage string `json:"default_language"`
	Timezone        string `json:"timezone"`
	FallbackConfig  string `json:"fallback_config"`
	HandoffConfig   string `json:"handoff_config"`
	Metadata        string `json:"metadata"`
}

func (i *BotInput) normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
	i.DefaultLanguage = strings.ToLower(strings.TrimSpace(i.DefaultLanguage))
	i.Timezone = strings.TrimSpace(i.Timezone)
}

type VersionInput struct {
	SourceVersionID *uuid.UUID `json:"source_version_id"`
	StartStepKey    string     `json:"start_step_key"`
	Metadata        string     `json:"metadata"`
}

type ModuleInput struct {
	ModuleKey   string `json:"module_key"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Parameters  string `json:"parameters"`
	Metadata    string `json:"metadata"`
	SortOrder   int    `json:"sort_order"`
}

type ModuleOrderInput struct {
	ModuleID  uuid.UUID `json:"module_id"`
	SortOrder int       `json:"sort_order"`
}

type ModuleReorderInput struct {
	Modules []ModuleOrderInput `json:"modules"`
}

type VariableInput struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Scope        string `json:"scope"`
	Description  string `json:"description"`
	DefaultValue string `json:"default_value"`
	Metadata     string `json:"metadata"`
}

type QuestionInput struct {
	QuestionKey  string `json:"question_key"`
	Text         string `json:"text"`
	Type         string `json:"type"`
	ResponseMode string `json:"response_mode"`
	Required     *bool  `json:"required"`
	VariableName string `json:"variable_name"`
	Description  string `json:"description"`
	HelpText     string `json:"help_text"`
	Options      string `json:"options"`
	Validation   string `json:"validation"`
	Metadata     string `json:"metadata"`
}

type ActionInput struct {
	ActionKey      string `json:"action_key"`
	ActionType     string `json:"action_type"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	InputMappings  string `json:"input_mappings"`
	OutputMappings string `json:"output_mappings"`
	Metadata       string `json:"metadata"`
}

type ConditionInput struct {
	ConditionKey string `json:"condition_key"`
	Name         string `json:"name"`
	Combinator   string `json:"combinator"`
	Rules        string `json:"rules"`
	Metadata     string `json:"metadata"`
}

type IntegrationInput struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Required    bool   `json:"required"`
	Config      string `json:"config"`
	Metadata    string `json:"metadata"`
}

type StepInput struct {
	StepKey         string     `json:"step_key"`
	Type            string     `json:"type"`
	Title           string     `json:"title"`
	Message         string     `json:"message"`
	QuestionID      *uuid.UUID `json:"question_id"`
	ModuleID        *uuid.UUID `json:"module_id"`
	ActionID        *uuid.UUID `json:"action_id"`
	ConditionID     *uuid.UUID `json:"condition_id"`
	ResponseMode    string     `json:"response_mode"`
	Options         string     `json:"options"`
	NextStepKey     string     `json:"next_step_key"`
	FallbackStepKey string     `json:"fallback_step_key"`
	SortOrder       int        `json:"sort_order"`
	Metadata        string     `json:"metadata"`
}

type SelfServiceBotInput struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	WelcomeMessage     string `json:"welcome_message"`
	SupportMessage     string `json:"support_message"`
	RequirePayment     *bool  `json:"require_payment"`
	AllowPickup        *bool  `json:"allow_pickup"`
	AllowCustomerRider *bool  `json:"allow_customer_rider"`
	AllowMerchantRider *bool  `json:"allow_merchant_rider"`
}

type CommerceWorkflowConfigurationInput struct {
	Status                   string   `json:"status"`
	BotDisplayName           string   `json:"bot_display_name"`
	Greeting                 string   `json:"greeting"`
	Tone                     string   `json:"tone"`
	OrderingEnabled          *bool    `json:"ordering_enabled"`
	PaymentEnabled           *bool    `json:"payment_enabled"`
	HumanHandoffEnabled      *bool    `json:"human_handoff_enabled"`
	StoreSelectionStrategy   string   `json:"store_selection_strategy"`
	EnabledActions           []string `json:"enabled_actions"`
	SupportedFulfilmentModes []string `json:"supported_fulfilment_modes"`
	PostPaymentSteps         []string `json:"post_payment_steps"`
	Metadata                 string   `json:"metadata"`
}

type CommerceWorkflowConfigurationView struct {
	CommerceWorkflowConfiguration
	EnabledActionList           []string `json:"enabled_action_list"`
	SupportedFulfilmentModeList []string `json:"supported_fulfilment_mode_list"`
	PostPaymentStepList         []string `json:"post_payment_step_list"`
}

type ServiceBookingBotInput struct {
	Name           string `json:"name"`
	WelcomeMessage string `json:"welcome_message"`
}

type FAQInput struct {
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	Keywords []string `json:"keywords"`
	Status   string   `json:"status"`
	Metadata string   `json:"metadata"`
}

type FAQMatch struct {
	FAQ        FAQ     `json:"faq"`
	Score      float64 `json:"score"`
	MatchedOn  string  `json:"matched_on"`
	Confidence string  `json:"confidence"`
}

type KnowledgeEntryInput struct {
	Kind       string   `json:"kind"`
	Category   string   `json:"category"`
	Title      string   `json:"title"`
	Question   string   `json:"question"`
	Answer     string   `json:"answer"`
	Keywords   []string `json:"keywords"`
	SourceType string   `json:"source_type"`
	Status     string   `json:"status"`
	Metadata   string   `json:"metadata"`
}

type KnowledgeEntryFilter struct {
	Kind     string
	Category string
	Status   string
	Search   string
}

type DocumentSourceInput struct {
	Title            string `json:"title"`
	SourceType       string `json:"source_type"`
	Status           string `json:"status"`
	OriginalFilename string `json:"original_filename"`
	SourceURL        string `json:"source_url"`
	SourceLabel      string `json:"source_label"`
	MimeType         string `json:"mime_type"`
	StorageKey       string `json:"storage_key"`
	RawText          string `json:"raw_text"`
	ErrorMessage     string `json:"error_message"`
	Metadata         string `json:"metadata"`
}

type DocumentSourceFilter struct {
	SourceType string
	Status     string
	Search     string
}

type DocumentChunkInput struct {
	Title    string `json:"title"`
	Heading  string `json:"heading"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Metadata string `json:"metadata"`
}

type DocumentChunkApprovalInput struct {
	Kind     string   `json:"kind"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Question string   `json:"question"`
	Keywords []string `json:"keywords"`
	Status   string   `json:"status"`
	Metadata string   `json:"metadata"`
}

type DocumentSourceArchiveOptions struct {
	ArchiveLinkedKnowledge bool
}

type ShareLink struct {
	Available     bool   `json:"available"`
	URL           string `json:"url"`
	EncodedText   string `json:"encoded_text"`
	DisplayNumber string `json:"display_number"`
	Message       string `json:"message"`
	Reason        string `json:"reason,omitempty"`
}

type ChecklistItem struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Complete    bool   `json:"complete"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Group       string `json:"group"`
}

type BotSetupStatus struct {
	OrganizationID        uuid.UUID       `json:"organization_id"`
	Items                 []ChecklistItem `json:"items"`
	CompleteCount         int             `json:"complete_count"`
	TotalCount            int             `json:"total_count"`
	RequiredCompleteCount int             `json:"required_complete_count"`
	RequiredCount         int             `json:"required_count"`
	Ready                 bool            `json:"ready"`
}

type ValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Issues []ValidationIssue `json:"issues"`
}

type VersionConfiguration struct {
	Version      BotVersion      `json:"version"`
	Modules      []VersionModule `json:"modules"`
	Variables    []Variable      `json:"variables"`
	Questions    []Question      `json:"questions"`
	Actions      []Action        `json:"actions"`
	Conditions   []Condition     `json:"conditions"`
	Integrations []Integration   `json:"integrations"`
	Steps        []Step          `json:"steps"`
	Fallback     string          `json:"fallback"`
	Handoff      string          `json:"handoff"`
}

type Preview struct {
	BotID         uuid.UUID        `json:"bot_id"`
	VersionID     uuid.UUID        `json:"version_id"`
	StartStepKey  string           `json:"start_step_key"`
	Steps         []PreviewStep    `json:"steps"`
	Validation    ValidationResult `json:"validation"`
	RuntimeNotice string           `json:"runtime_notice"`
}

type PreviewStep struct {
	StepKey      string   `json:"step_key"`
	Type         string   `json:"type"`
	Title        string   `json:"title"`
	Message      string   `json:"message"`
	ResponseMode string   `json:"response_mode"`
	Options      []string `json:"options"`
	NextStepKey  string   `json:"next_step_key"`
}
