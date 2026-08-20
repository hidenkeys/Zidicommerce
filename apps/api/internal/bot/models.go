package bot

import (
	"time"

	"github.com/google/uuid"
)

const (
	BotStatusDraft    = "draft"
	BotStatusActive   = "active"
	BotStatusInactive = "inactive"
	BotStatusArchived = "archived"

	VersionStatusDraft     = "draft"
	VersionStatusValidated = "validated"
	VersionStatusPublished = "published"
	VersionStatusArchived  = "archived"

	StepMessage   = "message"
	StepQuestion  = "question"
	StepChoice    = "choice"
	StepModule    = "module"
	StepCondition = "condition"
	StepAction    = "action"
	StepHandoff   = "handoff"
	StepEnd       = "end"

	ModuleSourceSystem       = "system"
	ModuleSourceOrganization = "organization"
)

type Bot struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Status             string     `json:"status"`
	DefaultLanguage    string     `json:"default_language"`
	Timezone           string     `json:"timezone"`
	FallbackConfig     string     `json:"fallback_config"`
	HandoffConfig      string     `json:"handoff_config"`
	Metadata           string     `json:"metadata"`
	PublishedVersionID *uuid.UUID `gorm:"type:uuid" json:"published_version_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (Bot) TableName() string { return "bots" }

type BotVersion struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	BotID            uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_bot_version_number" json:"bot_id"`
	VersionNumber    int        `gorm:"uniqueIndex:idx_bot_version_number" json:"version_number"`
	Status           string     `json:"status"`
	StartStepKey     string     `json:"start_step_key"`
	ValidationErrors string     `json:"validation_errors"`
	Metadata         string     `json:"metadata"`
	CreatedByUserID  *uuid.UUID `gorm:"type:uuid" json:"created_by_user_id,omitempty"`
	ValidatedAt      *time.Time `json:"validated_at,omitempty"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (BotVersion) TableName() string { return "bot_versions" }

type VersionModule struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_bot_module_key" json:"version_id"`
	ModuleKey      string    `gorm:"uniqueIndex:idx_bot_module_key" json:"module_key"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	Source         string    `json:"source"`
	Description    string    `json:"description"`
	Parameters     string    `json:"parameters"`
	Metadata       string    `json:"metadata"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (VersionModule) TableName() string { return "bot_version_modules" }

type Variable struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_bot_variable_name" json:"version_id"`
	Name           string    `gorm:"uniqueIndex:idx_bot_variable_name" json:"name"`
	Type           string    `json:"type"`
	Scope          string    `json:"scope"`
	Description    string    `json:"description"`
	DefaultValue   string    `json:"default_value"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Variable) TableName() string { return "bot_variables" }

type Question struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_bot_question_key" json:"version_id"`
	QuestionKey    string    `gorm:"uniqueIndex:idx_bot_question_key" json:"question_key"`
	Text           string    `json:"text"`
	Type           string    `json:"type"`
	ResponseMode   string    `json:"response_mode"`
	Required       bool      `json:"required"`
	VariableName   string    `json:"variable_name"`
	Description    string    `json:"description"`
	HelpText       string    `json:"help_text"`
	Options        string    `json:"options"`
	Validation     string    `json:"validation"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Question) TableName() string { return "bot_questions" }

type Action struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_bot_action_key" json:"version_id"`
	ActionKey      string    `gorm:"uniqueIndex:idx_bot_action_key" json:"action_key"`
	ActionType     string    `json:"action_type"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	InputMappings  string    `json:"input_mappings"`
	OutputMappings string    `json:"output_mappings"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Action) TableName() string { return "bot_actions" }

type Condition struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_bot_condition_key" json:"version_id"`
	ConditionKey   string    `gorm:"uniqueIndex:idx_bot_condition_key" json:"condition_key"`
	Name           string    `json:"name"`
	Combinator     string    `json:"combinator"`
	Rules          string    `json:"rules"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Condition) TableName() string { return "bot_conditions" }

type Integration struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_bot_integration_provider" json:"version_id"`
	Provider       string    `gorm:"uniqueIndex:idx_bot_integration_provider" json:"provider"`
	DisplayName    string    `json:"display_name"`
	Required       bool      `json:"required"`
	Config         string    `json:"config"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Integration) TableName() string { return "bot_integrations" }

type Step struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	VersionID       uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_bot_step_key" json:"version_id"`
	StepKey         string     `gorm:"uniqueIndex:idx_bot_step_key" json:"step_key"`
	Type            string     `json:"type"`
	Title           string     `json:"title"`
	Message         string     `json:"message"`
	QuestionID      *uuid.UUID `gorm:"type:uuid" json:"question_id,omitempty"`
	ModuleID        *uuid.UUID `gorm:"type:uuid" json:"module_id,omitempty"`
	ActionID        *uuid.UUID `gorm:"type:uuid" json:"action_id,omitempty"`
	ConditionID     *uuid.UUID `gorm:"type:uuid" json:"condition_id,omitempty"`
	ResponseMode    string     `json:"response_mode"`
	Options         string     `json:"options"`
	NextStepKey     string     `json:"next_step_key"`
	FallbackStepKey string     `json:"fallback_step_key"`
	SortOrder       int        `json:"sort_order"`
	Metadata        string     `json:"metadata"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (Step) TableName() string { return "bot_steps" }

type PublishedSnapshot struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	BotID          uuid.UUID `gorm:"type:uuid;index" json:"bot_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;index;uniqueIndex" json:"version_id"`
	VersionNumber  int       `json:"version_number"`
	Snapshot       string    `json:"snapshot"`
	CreatedAt      time.Time `json:"created_at"`
}

func (PublishedSnapshot) TableName() string { return "bot_published_snapshots" }

type FAQ struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	Question       string    `json:"question"`
	Answer         string    `json:"answer"`
	Keywords       string    `json:"keywords"`
	Status         string    `json:"status"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (FAQ) TableName() string { return "bot_faqs" }
