package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ListBots(ctx context.Context, actor auth.CurrentUser) ([]Bot, error) {
	if !canViewBots(actor.Role) {
		return nil, httperror.Forbidden("You cannot view bot configuration")
	}
	var bots []Bot
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("updated_at DESC").Find(&bots).Error
	return bots, err
}

func (s *Service) CreateBot(ctx context.Context, actor auth.CurrentUser, input BotInput) (Bot, error) {
	if !canManageBots(actor.Role) {
		return Bot{}, httperror.Forbidden("You cannot create bots")
	}
	input.normalize()
	if input.Name == "" {
		return Bot{}, httperror.BadRequest("Bot name is required")
	}
	bot := Bot{
		ID:              uuid.New(),
		OrganizationID:  actor.OrganizationID,
		Name:            input.Name,
		Description:     input.Description,
		Status:          defaultString(input.Status, BotStatusDraft),
		DefaultLanguage: defaultString(input.DefaultLanguage, "en"),
		Timezone:        defaultString(input.Timezone, "UTC"),
		FallbackConfig:  jsonObject(input.FallbackConfig),
		HandoffConfig:   jsonObject(input.HandoffConfig),
		Metadata:        jsonObject(input.Metadata),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&bot).Error; err != nil {
			return err
		}
		version := BotVersion{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: bot.ID, VersionNumber: 1, Status: VersionStatusDraft, StartStepKey: "start", ValidationErrors: "[]", CreatedByUserID: &actor.ID, Metadata: "{}"}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot", bot.ID, "bot_created", "{}")
	})
	return bot, err
}

func (s *Service) GetBot(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID) (Bot, error) {
	if !canViewBots(actor.Role) {
		return Bot{}, httperror.Forbidden("You cannot view bot configuration")
	}
	var bot Bot
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, botID).First(&bot).Error
	return bot, mapNotFound(err, "Bot not found")
}

func (s *Service) UpdateBot(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID, input BotInput) (Bot, error) {
	if !canManageBots(actor.Role) {
		return Bot{}, httperror.Forbidden("You cannot update bots")
	}
	bot, err := s.GetBot(ctx, actor, botID)
	if err != nil {
		return Bot{}, err
	}
	input.normalize()
	updates := map[string]any{"updated_at": s.now()}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	if input.DefaultLanguage != "" {
		updates["default_language"] = input.DefaultLanguage
	}
	if input.Timezone != "" {
		updates["timezone"] = input.Timezone
	}
	if input.FallbackConfig != "" {
		updates["fallback_config"] = jsonObject(input.FallbackConfig)
	}
	if input.HandoffConfig != "" {
		updates["handoff_config"] = jsonObject(input.HandoffConfig)
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	if err := s.db.WithContext(ctx).Model(&bot).Updates(updates).Error; err != nil {
		return Bot{}, err
	}
	_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot", bot.ID, "bot_updated", "{}")
	return s.GetBot(ctx, actor, botID)
}

func (s *Service) ListVersions(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID) ([]BotVersion, error) {
	if _, err := s.GetBot(ctx, actor, botID); err != nil {
		return nil, err
	}
	var versions []BotVersion
	err := s.db.WithContext(ctx).Where("organization_id = ? AND bot_id = ?", actor.OrganizationID, botID).Order("version_number DESC").Find(&versions).Error
	return versions, err
}

func (s *Service) CreateVersion(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID, input VersionInput) (BotVersion, error) {
	if !canManageBots(actor.Role) {
		return BotVersion{}, httperror.Forbidden("You cannot create bot versions")
	}
	if _, err := s.GetBot(ctx, actor, botID); err != nil {
		return BotVersion{}, err
	}
	var version BotVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&BotVersion{}).Where("organization_id = ? AND bot_id = ?", actor.OrganizationID, botID).Select("COALESCE(MAX(version_number), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		startStepKey := defaultString(input.StartStepKey, "start")
		if input.SourceVersionID != nil && input.StartStepKey == "" {
			var source BotVersion
			if err := tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, *input.SourceVersionID).First(&source).Error; err != nil {
				return mapNotFound(err, "Source version not found")
			}
			if source.BotID != botID {
				return httperror.BadRequest("Source version must belong to the selected bot")
			}
			startStepKey = source.StartStepKey
		}
		version = BotVersion{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: botID, VersionNumber: maxVersion + 1, Status: VersionStatusDraft, StartStepKey: startStepKey, ValidationErrors: "[]", Metadata: jsonObject(input.Metadata), CreatedByUserID: &actor.ID}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if input.SourceVersionID != nil {
			if err := s.copyVersionConfigTx(tx, actor.OrganizationID, *input.SourceVersionID, version.ID); err != nil {
				return err
			}
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot_version", version.ID, "bot_version_created", fmt.Sprintf(`{"version_number":%d}`, version.VersionNumber))
	})
	return version, err
}

func (s *Service) GetVersion(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (BotVersion, error) {
	if !canViewBots(actor.Role) {
		return BotVersion{}, httperror.Forbidden("You cannot view bot configuration")
	}
	var version BotVersion
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, versionID).First(&version).Error
	return version, mapNotFound(err, "Bot version not found")
}

func (s *Service) GetConfiguration(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (VersionConfiguration, error) {
	version, err := s.GetVersion(ctx, actor, versionID)
	if err != nil {
		return VersionConfiguration{}, err
	}
	return s.loadConfiguration(ctx, actor.OrganizationID, version)
}

func (s *Service) AddModule(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input ModuleInput) (VersionModule, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return VersionModule{}, err
	}
	input.ModuleKey = strings.ToUpper(strings.TrimSpace(input.ModuleKey))
	if input.ModuleKey == "" {
		return VersionModule{}, httperror.BadRequest("Module key is required")
	}
	spec := findModule(input.ModuleKey)
	module := VersionModule{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, ModuleKey: input.ModuleKey, Name: defaultString(input.Name, spec.Name), Category: defaultString(input.Category, spec.Category), Source: defaultString(input.Source, ModuleSourceSystem), Description: defaultString(input.Description, spec.Description), Parameters: jsonObject(input.Parameters), Metadata: jsonObject(input.Metadata), SortOrder: input.SortOrder}
	if module.Name == "" {
		module.Name = input.ModuleKey
	}
	err := s.db.WithContext(ctx).Create(&module).Error
	if err == nil {
		_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot_module", module.ID, "bot_module_added", fmt.Sprintf(`{"module_key":%q}`, module.ModuleKey))
	}
	return module, err
}

func (s *Service) ListModules(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]VersionModule, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var modules []VersionModule
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("sort_order ASC, created_at ASC").Find(&modules).Error
	return modules, err
}

func (s *Service) CreateVariable(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input VariableInput) (Variable, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return Variable{}, err
	}
	name := strings.TrimSpace(input.Name)
	typ := strings.ToLower(strings.TrimSpace(input.Type))
	scope := strings.ToLower(strings.TrimSpace(input.Scope))
	if name == "" || !validVariableType(typ) {
		return Variable{}, httperror.BadRequest("Variable name and valid type are required")
	}
	variable := Variable{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, Name: name, Type: typ, Scope: defaultString(scope, "user"), Description: input.Description, DefaultValue: input.DefaultValue, Metadata: jsonObject(input.Metadata)}
	if !validVariableScope(variable.Scope) {
		return Variable{}, httperror.BadRequest("Variable scope is not valid")
	}
	return variable, s.db.WithContext(ctx).Create(&variable).Error
}

func (s *Service) ListVariables(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]Variable, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var variables []Variable
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("scope ASC, name ASC").Find(&variables).Error
	return variables, err
}

func (s *Service) CreateQuestion(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input QuestionInput) (Question, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return Question{}, err
	}
	required := true
	if input.Required != nil {
		required = *input.Required
	}
	question := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, QuestionKey: slugKey(input.QuestionKey), Text: strings.TrimSpace(input.Text), Type: strings.ToLower(strings.TrimSpace(input.Type)), ResponseMode: strings.ToLower(strings.TrimSpace(input.ResponseMode)), Required: required, VariableName: strings.TrimSpace(input.VariableName), Description: input.Description, HelpText: input.HelpText, Options: jsonArray(input.Options), Validation: jsonObject(input.Validation), Metadata: jsonObject(input.Metadata)}
	if question.QuestionKey == "" || question.Text == "" || !validQuestionType(question.Type) || !validResponseMode(question.ResponseMode) {
		return Question{}, httperror.BadRequest("Question key, text, type, and response mode are required")
	}
	return question, s.db.WithContext(ctx).Create(&question).Error
}

func (s *Service) ListQuestions(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]Question, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var questions []Question
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("created_at ASC").Find(&questions).Error
	return questions, err
}

func (s *Service) CreateAction(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input ActionInput) (Action, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return Action{}, err
	}
	action := Action{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, ActionKey: slugKey(input.ActionKey), ActionType: strings.ToLower(strings.TrimSpace(input.ActionType)), Name: strings.TrimSpace(input.Name), Description: input.Description, InputMappings: jsonObject(input.InputMappings), OutputMappings: jsonObject(input.OutputMappings), Metadata: jsonObject(input.Metadata)}
	if action.ActionKey == "" || action.ActionType == "" || action.Name == "" {
		return Action{}, httperror.BadRequest("Action key, type, and name are required")
	}
	if !validActionType(action.ActionType) {
		return Action{}, httperror.BadRequest("Action type is not supported")
	}
	return action, s.db.WithContext(ctx).Create(&action).Error
}

func (s *Service) ListActions(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]Action, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var actions []Action
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("created_at ASC").Find(&actions).Error
	return actions, err
}

func (s *Service) CreateCondition(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input ConditionInput) (Condition, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return Condition{}, err
	}
	condition := Condition{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, ConditionKey: slugKey(input.ConditionKey), Name: strings.TrimSpace(input.Name), Combinator: strings.ToLower(strings.TrimSpace(input.Combinator)), Rules: jsonArray(input.Rules), Metadata: jsonObject(input.Metadata)}
	if condition.ConditionKey == "" || condition.Name == "" {
		return Condition{}, httperror.BadRequest("Condition key and name are required")
	}
	if condition.Combinator == "" {
		condition.Combinator = "and"
	}
	if condition.Combinator != "and" && condition.Combinator != "or" {
		return Condition{}, httperror.BadRequest("Condition combinator is not valid")
	}
	return condition, s.db.WithContext(ctx).Create(&condition).Error
}

func (s *Service) ListConditions(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]Condition, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var conditions []Condition
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("created_at ASC").Find(&conditions).Error
	return conditions, err
}

func (s *Service) CreateIntegration(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input IntegrationInput) (Integration, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return Integration{}, err
	}
	integration := Integration{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, Provider: strings.ToLower(strings.TrimSpace(input.Provider)), DisplayName: strings.TrimSpace(input.DisplayName), Required: input.Required, Config: jsonObject(input.Config), Metadata: jsonObject(input.Metadata)}
	if integration.Provider == "" || integration.DisplayName == "" {
		return Integration{}, httperror.BadRequest("Integration provider and display name are required")
	}
	if containsSecretLikeKey(integration.Config) {
		return Integration{}, httperror.BadRequest("Integration config must not contain plaintext secrets")
	}
	return integration, s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "version_id"}, {Name: "provider"}}, DoUpdates: clause.Assignments(map[string]any{"display_name": integration.DisplayName, "required": integration.Required, "config": integration.Config, "metadata": integration.Metadata, "updated_at": s.now()})}).Create(&integration).Error
}

func (s *Service) ListIntegrations(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]Integration, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var integrations []Integration
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("provider ASC").Find(&integrations).Error
	return integrations, err
}

func (s *Service) CreateStep(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input StepInput) (Step, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return Step{}, err
	}
	step := Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, StepKey: slugKey(input.StepKey), Type: strings.ToLower(strings.TrimSpace(input.Type)), Title: strings.TrimSpace(input.Title), Message: input.Message, QuestionID: input.QuestionID, ModuleID: input.ModuleID, ActionID: input.ActionID, ConditionID: input.ConditionID, ResponseMode: defaultString(strings.ToLower(strings.TrimSpace(input.ResponseMode)), "free_text"), Options: jsonArray(input.Options), NextStepKey: slugKey(input.NextStepKey), FallbackStepKey: slugKey(input.FallbackStepKey), SortOrder: input.SortOrder, Metadata: jsonObject(input.Metadata)}
	if step.StepKey == "" || step.Title == "" || !validStepType(step.Type) {
		return Step{}, httperror.BadRequest("Step key, title, and valid type are required")
	}
	return step, s.db.WithContext(ctx).Create(&step).Error
}

func (s *Service) UpdateStep(ctx context.Context, actor auth.CurrentUser, stepID uuid.UUID, input StepInput) (Step, error) {
	var step Step
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, stepID).First(&step).Error; err != nil {
		return Step{}, mapNotFound(err, "Step not found")
	}
	if err := s.ensureEditable(ctx, actor, step.VersionID); err != nil {
		return Step{}, err
	}
	updates := map[string]any{"updated_at": s.now()}
	if input.Title != "" {
		updates["title"] = input.Title
	}
	if input.Message != "" {
		updates["message"] = input.Message
	}
	if input.NextStepKey != "" {
		updates["next_step_key"] = slugKey(input.NextStepKey)
	}
	if input.FallbackStepKey != "" {
		updates["fallback_step_key"] = slugKey(input.FallbackStepKey)
	}
	if input.Options != "" {
		updates["options"] = jsonArray(input.Options)
	}
	if input.SortOrder != 0 {
		updates["sort_order"] = input.SortOrder
	}
	if err := s.db.WithContext(ctx).Model(&step).Updates(updates).Error; err != nil {
		return Step{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, stepID).First(&step).Error; err != nil {
		return Step{}, err
	}
	return step, nil
}

func (s *Service) ListSteps(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) ([]Step, error) {
	if _, err := s.GetVersion(ctx, actor, versionID); err != nil {
		return nil, err
	}
	var steps []Step
	err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Order("sort_order ASC, created_at ASC").Find(&steps).Error
	return steps, err
}

func (s *Service) ValidateVersion(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (ValidationResult, error) {
	version, err := s.GetVersion(ctx, actor, versionID)
	if err != nil {
		return ValidationResult{}, err
	}
	config, err := s.loadConfiguration(ctx, actor.OrganizationID, version)
	if err != nil {
		return ValidationResult{}, err
	}
	result := validateConfiguration(config)
	status := version.Status
	if result.Valid && status == VersionStatusDraft {
		status = VersionStatusValidated
	}
	raw, _ := json.Marshal(result.Issues)
	now := s.now()
	updates := map[string]any{"validation_errors": string(raw), "updated_at": now}
	if result.Valid {
		updates["status"] = status
		updates["validated_at"] = &now
	}
	_ = s.db.WithContext(ctx).Model(&version).Updates(updates).Error
	_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot_version", version.ID, "bot_validated", fmt.Sprintf(`{"valid":%t}`, result.Valid))
	return result, nil
}

func (s *Service) PublishVersion(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (PublishedSnapshot, error) {
	if !canManageBots(actor.Role) {
		return PublishedSnapshot{}, httperror.Forbidden("You cannot publish bots")
	}
	version, err := s.GetVersion(ctx, actor, versionID)
	if err != nil {
		return PublishedSnapshot{}, err
	}
	if version.Status == VersionStatusPublished {
		return PublishedSnapshot{}, httperror.BadRequest("Bot version is already published")
	}
	config, err := s.loadConfiguration(ctx, actor.OrganizationID, version)
	if err != nil {
		return PublishedSnapshot{}, err
	}
	result := validateConfiguration(config)
	if !result.Valid {
		return PublishedSnapshot{}, httperror.BadRequest("Bot version has validation errors")
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return PublishedSnapshot{}, err
	}
	snapshot := PublishedSnapshot{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: version.BotID, VersionID: version.ID, VersionNumber: version.VersionNumber, Snapshot: string(raw)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if err := tx.Model(&BotVersion{}).Where("organization_id = ? AND bot_id = ? AND status = ?", actor.OrganizationID, version.BotID, VersionStatusPublished).Updates(map[string]any{"status": VersionStatusArchived, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&version).Updates(map[string]any{"status": VersionStatusPublished, "published_at": &now, "validated_at": &now, "updated_at": now, "validation_errors": "[]"}).Error; err != nil {
			return err
		}
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}
		if err := tx.Model(&Bot{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, version.BotID).Updates(map[string]any{"published_version_id": version.ID, "status": BotStatusActive, "updated_at": now}).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot_version", version.ID, "bot_published", fmt.Sprintf(`{"version_number":%d}`, version.VersionNumber))
	})
	return snapshot, err
}

func (s *Service) PreviewVersion(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (Preview, error) {
	version, err := s.GetVersion(ctx, actor, versionID)
	if err != nil {
		return Preview{}, err
	}
	config, err := s.loadConfiguration(ctx, actor.OrganizationID, version)
	if err != nil {
		return Preview{}, err
	}
	result := validateConfiguration(config)
	steps := make([]PreviewStep, 0, len(config.Steps))
	for _, step := range config.Steps {
		steps = append(steps, PreviewStep{StepKey: step.StepKey, Type: step.Type, Title: step.Title, Message: step.Message, ResponseMode: step.ResponseMode, Options: parseStringOptions(step.Options), NextStepKey: step.NextStepKey})
	}
	return Preview{BotID: version.BotID, VersionID: version.ID, StartStepKey: version.StartStepKey, Steps: steps, Validation: result, RuntimeNotice: "Configuration preview only. No WhatsApp, payments, orders, or live runtime actions are executed."}, nil
}

func (s *Service) ensureEditable(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) error {
	if !canManageBots(actor.Role) {
		return httperror.Forbidden("You cannot edit bot configuration")
	}
	version, err := s.GetVersion(ctx, actor, versionID)
	if err != nil {
		return err
	}
	if version.Status == VersionStatusPublished || version.Status == VersionStatusArchived {
		return httperror.BadRequest("Published or archived bot versions are immutable")
	}
	return nil
}

func (s *Service) loadConfiguration(ctx context.Context, orgID uuid.UUID, version BotVersion) (VersionConfiguration, error) {
	config := VersionConfiguration{Version: version}
	db := s.db.WithContext(ctx)
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("sort_order ASC, created_at ASC").Find(&config.Modules).Error; err != nil {
		return VersionConfiguration{}, err
	}
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("scope ASC, name ASC").Find(&config.Variables).Error; err != nil {
		return VersionConfiguration{}, err
	}
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("created_at ASC").Find(&config.Questions).Error; err != nil {
		return VersionConfiguration{}, err
	}
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("created_at ASC").Find(&config.Actions).Error; err != nil {
		return VersionConfiguration{}, err
	}
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("created_at ASC").Find(&config.Conditions).Error; err != nil {
		return VersionConfiguration{}, err
	}
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("provider ASC").Find(&config.Integrations).Error; err != nil {
		return VersionConfiguration{}, err
	}
	if err := db.Where("organization_id = ? AND version_id = ?", orgID, version.ID).Order("sort_order ASC, created_at ASC").Find(&config.Steps).Error; err != nil {
		return VersionConfiguration{}, err
	}
	var bot Bot
	if err := db.Where("organization_id = ? AND id = ?", orgID, version.BotID).First(&bot).Error; err != nil {
		return VersionConfiguration{}, err
	}
	config.Fallback = bot.FallbackConfig
	config.Handoff = bot.HandoffConfig
	return config, nil
}

func (s *Service) copyVersionConfigTx(tx *gorm.DB, orgID, sourceVersionID, targetVersionID uuid.UUID) error {
	var source BotVersion
	if err := tx.Where("organization_id = ? AND id = ?", orgID, sourceVersionID).First(&source).Error; err != nil {
		return mapNotFound(err, "Source version not found")
	}
	var target BotVersion
	if err := tx.Where("organization_id = ? AND id = ?", orgID, targetVersionID).First(&target).Error; err != nil {
		return mapNotFound(err, "Target version not found")
	}
	if source.BotID != target.BotID {
		return httperror.BadRequest("Source version must belong to the selected bot")
	}
	moduleIDMap := map[uuid.UUID]uuid.UUID{}
	questionIDMap := map[uuid.UUID]uuid.UUID{}
	actionIDMap := map[uuid.UUID]uuid.UUID{}
	conditionIDMap := map[uuid.UUID]uuid.UUID{}

	var modules []VersionModule
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&modules).Error; err != nil {
		return err
	}
	for _, item := range modules {
		oldID := item.ID
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		moduleIDMap[oldID] = item.ID
	}
	var variables []Variable
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&variables).Error; err != nil {
		return err
	}
	for _, item := range variables {
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	var questions []Question
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&questions).Error; err != nil {
		return err
	}
	for _, item := range questions {
		oldID := item.ID
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		questionIDMap[oldID] = item.ID
	}
	var actions []Action
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&actions).Error; err != nil {
		return err
	}
	for _, item := range actions {
		oldID := item.ID
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		actionIDMap[oldID] = item.ID
	}
	var conditions []Condition
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&conditions).Error; err != nil {
		return err
	}
	for _, item := range conditions {
		oldID := item.ID
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		conditionIDMap[oldID] = item.ID
	}
	var integrations []Integration
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&integrations).Error; err != nil {
		return err
	}
	for _, item := range integrations {
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	var steps []Step
	if err := tx.Where("organization_id = ? AND version_id = ?", orgID, sourceVersionID).Find(&steps).Error; err != nil {
		return err
	}
	for _, item := range steps {
		item.ID = uuid.New()
		item.VersionID = targetVersionID
		item.QuestionID = remapOptional(item.QuestionID, questionIDMap)
		item.ModuleID = remapOptional(item.ModuleID, moduleIDMap)
		item.ActionID = remapOptional(item.ActionID, actionIDMap)
		item.ConditionID = remapOptional(item.ConditionID, conditionIDMap)
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateConfiguration(config VersionConfiguration) ValidationResult {
	var issues []ValidationIssue
	stepKeys := map[string]Step{}
	variableNames := map[string]struct{}{}
	questionIDs := map[uuid.UUID]struct{}{}
	moduleIDs := map[uuid.UUID]VersionModule{}
	actionIDs := map[uuid.UUID]Action{}
	conditionIDs := map[uuid.UUID]Condition{}
	integrations := map[string]struct{}{}

	for _, variable := range config.Variables {
		if !validVariableType(variable.Type) {
			issues = append(issues, ValidationIssue{Path: "variables." + variable.Name, Message: "Variable type is invalid"})
		}
		if !validVariableScope(variable.Scope) {
			issues = append(issues, ValidationIssue{Path: "variables." + variable.Name, Message: "Variable scope is invalid"})
		}
		variableNames[variable.Name] = struct{}{}
	}
	for _, question := range config.Questions {
		questionIDs[question.ID] = struct{}{}
		if !validQuestionType(question.Type) || !validResponseMode(question.ResponseMode) {
			issues = append(issues, ValidationIssue{Path: "questions." + question.QuestionKey, Message: "Question type or response mode is invalid"})
		}
		if question.VariableName != "" {
			if _, ok := variableNames[question.VariableName]; !ok {
				issues = append(issues, ValidationIssue{Path: "questions." + question.QuestionKey, Message: "Question references a variable that does not exist"})
			}
		}
	}
	for _, module := range config.Modules {
		moduleIDs[module.ID] = module
		if module.Source == ModuleSourceSystem && findModule(module.ModuleKey).Key == "" {
			issues = append(issues, ValidationIssue{Path: "modules." + module.ModuleKey, Message: "System module is not registered"})
		}
		if module.ModuleKey == "ORDER" && jsonBool(module.Parameters, "require_payment") {
			if _, ok := integrations["paystack"]; !ok {
				// Checked again after integrations load below.
			}
		}
	}
	for _, action := range config.Actions {
		actionIDs[action.ID] = action
		if !validActionType(action.ActionType) {
			issues = append(issues, ValidationIssue{Path: "actions." + action.ActionKey, Message: "Action type is not registered"})
		}
		if !jsonIsObject(action.InputMappings) || !jsonIsObject(action.OutputMappings) {
			issues = append(issues, ValidationIssue{Path: "actions." + action.ActionKey, Message: "Action input and output mappings must be objects"})
		}
	}
	for _, condition := range config.Conditions {
		conditionIDs[condition.ID] = condition
		if !validRules(condition.Rules) {
			issues = append(issues, ValidationIssue{Path: "conditions." + condition.ConditionKey, Message: "Condition rules are invalid"})
		}
	}
	for _, integration := range config.Integrations {
		integrations[integration.Provider] = struct{}{}
		if containsSecretLikeKey(integration.Config) {
			issues = append(issues, ValidationIssue{Path: "integrations." + integration.Provider, Message: "Integration config contains a plaintext secret-like key"})
		}
	}
	for _, module := range config.Modules {
		if module.ModuleKey == "ORDER" && jsonBool(module.Parameters, "require_payment") {
			if _, ok := integrations["paystack"]; !ok {
				issues = append(issues, ValidationIssue{Path: "modules.ORDER", Message: "Order module requires a payment integration when require_payment is enabled"})
			}
		}
	}
	for _, step := range config.Steps {
		stepKeys[step.StepKey] = step
		if !validStepType(step.Type) {
			issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step type is invalid"})
		}
		if step.QuestionID != nil {
			if _, ok := questionIDs[*step.QuestionID]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step references a missing question"})
			}
		}
		if step.ModuleID != nil {
			if _, ok := moduleIDs[*step.ModuleID]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step references a missing module"})
			}
		}
		if step.ActionID != nil {
			if _, ok := actionIDs[*step.ActionID]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step references a missing action"})
			}
		}
		if step.ConditionID != nil {
			if _, ok := conditionIDs[*step.ConditionID]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step references a missing condition"})
			}
		}
	}
	if config.Version.StartStepKey == "" {
		issues = append(issues, ValidationIssue{Path: "version.start_step_key", Message: "Bot version needs an explicit start step"})
	} else if _, ok := stepKeys[config.Version.StartStepKey]; !ok {
		issues = append(issues, ValidationIssue{Path: "version.start_step_key", Message: "Start step does not exist"})
	}
	for _, step := range config.Steps {
		if step.NextStepKey != "" {
			if _, ok := stepKeys[step.NextStepKey]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Next step does not exist"})
			}
		}
		if step.FallbackStepKey != "" {
			if _, ok := stepKeys[step.FallbackStepKey]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Fallback step does not exist"})
			}
		}
	}
	return ValidationResult{Valid: len(issues) == 0, Issues: issues}
}

func canManageBots(role authz.Role) bool {
	return role == authz.PlatformAdmin || role == authz.MerchantAdmin
}

func canViewBots(role authz.Role) bool {
	return role == authz.PlatformAdmin || role == authz.MerchantAdmin || role == authz.StoreManager || role == authz.SupportAgent || role == authz.Viewer
}

func findModule(key string) ModuleSpec {
	for _, module := range SystemModules() {
		if module.Key == strings.ToUpper(strings.TrimSpace(key)) {
			return module
		}
	}
	return ModuleSpec{}
}

func validActionType(key string) bool {
	for _, action := range SystemActions() {
		if action.Key == key {
			return true
		}
	}
	return key == "custom_webhook" || key == "handoff_to_staff" || key == "send_message"
}

func validQuestionType(key string) bool {
	for _, typ := range QuestionTypes() {
		if typ.Key == key {
			return true
		}
	}
	return false
}

func validResponseMode(mode string) bool {
	switch mode {
	case "free_text", "buttons", "list", "single_choice", "multiple_choice", "location", "image", "product_selection", "order_selection":
		return true
	default:
		return false
	}
}

func validVariableType(typ string) bool {
	switch typ {
	case "string", "number", "boolean", "date", "datetime", "location", "object", "array":
		return true
	default:
		return false
	}
}

func validVariableScope(scope string) bool {
	switch scope {
	case "system", "user", "conversation", "module":
		return true
	default:
		return false
	}
}

func validStepType(typ string) bool {
	switch typ {
	case StepMessage, StepQuestion, StepChoice, StepModule, StepCondition, StepAction, StepHandoff, StepEnd:
		return true
	default:
		return false
	}
}

func validRules(raw string) bool {
	var rules []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
	}
	if err := json.Unmarshal([]byte(jsonArray(raw)), &rules); err != nil {
		return false
	}
	if len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if strings.TrimSpace(rule.Field) == "" || !validOperator(rule.Operator) {
			return false
		}
	}
	return true
}

func validOperator(operator string) bool {
	switch operator {
	case "equals", "not_equals", "contains", "greater_than", "less_than", "exists", "not_exists", "in", "not_in":
		return true
	default:
		return false
	}
}

func containsSecretLikeKey(raw string) bool {
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonObject(raw)), &obj); err != nil {
		return true
	}
	for key := range obj {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") || strings.Contains(normalized, "password") || strings.Contains(normalized, "key") {
			return true
		}
	}
	return false
}

func parseStringOptions(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(jsonArray(raw)), &values)
	return values
}

func jsonBool(raw, key string) bool {
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonObject(raw)), &obj); err != nil {
		return false
	}
	value, ok := obj[key]
	if !ok {
		return false
	}
	asBool, ok := value.(bool)
	return ok && asBool
}

func jsonIsObject(raw string) bool {
	var obj map[string]any
	return json.Unmarshal([]byte(jsonObject(raw)), &obj) == nil
}

func jsonObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "{}"
	}
	return raw
}

func jsonArray(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]"
	}
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return "[]"
	}
	return raw
}

func slugKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func remapOptional(value *uuid.UUID, mapping map[uuid.UUID]uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	if next, ok := mapping[*value]; ok {
		return &next
	}
	return nil
}

func auditTx(tx *gorm.DB, organizationID, actorID uuid.UUID, targetType string, targetID uuid.UUID, action string, metadata string) error {
	log := organization.AuditLog{ID: uuid.New(), OrganizationID: &organizationID, ActorUserID: &actorID, TargetType: targetType, TargetID: &targetID, Action: action, Metadata: jsonObject(metadata)}
	return tx.Create(&log).Error
}

func mapNotFound(err error, message string) error {
	if err == nil {
		return nil
	}
	if err == gorm.ErrRecordNotFound {
		return httperror.NotFound(message)
	}
	return err
}
