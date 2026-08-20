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
