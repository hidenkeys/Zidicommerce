package bot

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var supportedWorkflowFulfilmentModes = map[string]struct{}{
	"pickup":         {},
	"customer_rider": {},
	"merchant_rider": {},
}

var supportedPostPaymentSteps = map[string]struct{}{
	"notify_customer":       {},
	"notify_store":          {},
	"merchant_prepares":     {},
	"assign_internal_rider": {},
	"third_party_delivery":  {},
	"customer_pickup":       {},
	"enable_tracking":       {},
	"request_human_handoff": {},
}

func DefaultCommerceWorkflowConfiguration(organizationID uuid.UUID) CommerceWorkflowConfiguration {
	return CommerceWorkflowConfiguration{
		ID:                       uuid.New(),
		OrganizationID:           organizationID,
		Status:                   WorkflowStatusActive,
		BotDisplayName:           "Store assistant",
		Greeting:                 "Welcome. How can I help you today?",
		Tone:                     "helpful",
		OrderingEnabled:          true,
		PaymentEnabled:           true,
		HumanHandoffEnabled:      true,
		StoreSelectionStrategy:   StoreSelectionCustomerChoice,
		EnabledActions:           jsonValue(allWorkflowActionKeys()),
		SupportedFulfilmentModes: jsonValue([]string{"pickup", "customer_rider", "merchant_rider"}),
		PostPaymentSteps:         jsonValue([]string{"notify_customer", "notify_store", "merchant_prepares", "enable_tracking"}),
		Metadata:                 "{}",
	}
}

func LoadEffectiveCommerceWorkflowConfiguration(ctx context.Context, db *gorm.DB, organizationID uuid.UUID) (CommerceWorkflowConfiguration, error) {
	var config CommerceWorkflowConfiguration
	err := db.WithContext(ctx).Where("organization_id = ?", organizationID).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return DefaultCommerceWorkflowConfiguration(organizationID), nil
	}
	return config, err
}

func (s *Service) GetCommerceWorkflowConfiguration(ctx context.Context, actor auth.CurrentUser) (CommerceWorkflowConfigurationView, error) {
	if !canViewBots(actor.Role) {
		return CommerceWorkflowConfigurationView{}, httperror.Forbidden("You cannot view bot workflow configuration")
	}
	config, err := LoadEffectiveCommerceWorkflowConfiguration(ctx, s.db, actor.OrganizationID)
	if err != nil {
		return CommerceWorkflowConfigurationView{}, err
	}
	return commerceWorkflowView(config), nil
}

func (s *Service) UpdateCommerceWorkflowConfiguration(ctx context.Context, actor auth.CurrentUser, input CommerceWorkflowConfigurationInput) (CommerceWorkflowConfigurationView, error) {
	if !canManageBots(actor.Role) {
		return CommerceWorkflowConfigurationView{}, httperror.Forbidden("You cannot manage bot workflow configuration")
	}
	config, err := commerceWorkflowFromInput(actor.OrganizationID, input)
	if err != nil {
		return CommerceWorkflowConfigurationView{}, err
	}
	config.CreatedByUserID = &actor.ID
	config.UpdatedByUserID = &actor.ID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing CommerceWorkflowConfiguration
		findErr := tx.Where("organization_id = ?", actor.OrganizationID).First(&existing).Error
		switch findErr {
		case nil:
			config.ID = existing.ID
			config.CreatedAt = existing.CreatedAt
			config.CreatedByUserID = existing.CreatedByUserID
			if err := tx.Model(&existing).Select("status", "bot_display_name", "greeting", "tone", "ordering_enabled", "payment_enabled", "human_handoff_enabled", "store_selection_strategy", "enabled_actions", "supported_fulfilment_modes", "post_payment_steps", "metadata", "updated_by_user_id", "updated_at").Updates(&config).Error; err != nil {
				return err
			}
		case gorm.ErrRecordNotFound:
			if err := tx.Create(&config).Error; err != nil {
				return err
			}
		default:
			return findErr
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "commerce_workflow_configuration", config.ID, "commerce_workflow_configuration_updated", jsonValue(map[string]any{
			"ordering_enabled":         config.OrderingEnabled,
			"payment_enabled":          config.PaymentEnabled,
			"handoff_enabled":          config.HumanHandoffEnabled,
			"store_selection_strategy": config.StoreSelectionStrategy,
		}))
	})
	if err != nil {
		return CommerceWorkflowConfigurationView{}, err
	}
	stored, err := LoadEffectiveCommerceWorkflowConfiguration(ctx, s.db, actor.OrganizationID)
	if err != nil {
		return CommerceWorkflowConfigurationView{}, err
	}
	return commerceWorkflowView(stored), nil
}

func EnsureDefaultCommerceWorkflowConfigurationTx(tx *gorm.DB, organizationID, actorID uuid.UUID, displayName, greeting string, orderingEnabled, paymentEnabled, handoffEnabled bool) error {
	config := DefaultCommerceWorkflowConfiguration(organizationID)
	config.BotDisplayName = defaultString(strings.TrimSpace(displayName), config.BotDisplayName)
	config.Greeting = defaultString(strings.TrimSpace(greeting), config.Greeting)
	config.OrderingEnabled = orderingEnabled
	config.PaymentEnabled = paymentEnabled
	config.HumanHandoffEnabled = handoffEnabled
	config.CreatedByUserID = &actorID
	config.UpdatedByUserID = &actorID
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}}, DoNothing: true}).Create(&config).Error
}

func (c CommerceWorkflowConfiguration) AllowsAction(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	if c.Status != "" && c.Status != WorkflowStatusActive {
		return false
	}
	if !c.OrderingEnabled && isOrderingMutation(action) {
		return false
	}
	if !c.PaymentEnabled && (action == "initialize_payment" || action == "check_payment") {
		return false
	}
	if !c.HumanHandoffEnabled && action == "handoff_to_agent" {
		return false
	}
	enabled := decodeStringList(c.EnabledActions)
	if len(enabled) == 0 {
		return false
	}
	for _, candidate := range enabled {
		if candidate == action {
			return true
		}
	}
	return false
}

func (c CommerceWorkflowConfiguration) AllowsFulfilmentMode(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	for _, candidate := range decodeStringList(c.SupportedFulfilmentModes) {
		if candidate == mode {
			return true
		}
	}
	return false
}

func (c CommerceWorkflowConfiguration) PostPaymentStepValues() []string {
	return decodeStringList(c.PostPaymentSteps)
}

func commerceWorkflowFromInput(organizationID uuid.UUID, input CommerceWorkflowConfigurationInput) (CommerceWorkflowConfiguration, error) {
	config := DefaultCommerceWorkflowConfiguration(organizationID)
	config.Status = defaultString(strings.ToLower(strings.TrimSpace(input.Status)), WorkflowStatusActive)
	if config.Status != WorkflowStatusActive && config.Status != WorkflowStatusInactive {
		return CommerceWorkflowConfiguration{}, httperror.BadRequest("Workflow status must be active or inactive")
	}
	config.BotDisplayName = defaultString(strings.TrimSpace(input.BotDisplayName), config.BotDisplayName)
	config.Greeting = defaultString(strings.TrimSpace(input.Greeting), config.Greeting)
	config.Tone = defaultString(strings.ToLower(strings.TrimSpace(input.Tone)), config.Tone)
	if input.OrderingEnabled != nil {
		config.OrderingEnabled = *input.OrderingEnabled
	}
	if input.PaymentEnabled != nil {
		config.PaymentEnabled = *input.PaymentEnabled
	}
	if input.HumanHandoffEnabled != nil {
		config.HumanHandoffEnabled = *input.HumanHandoffEnabled
	}
	config.StoreSelectionStrategy = defaultString(strings.ToLower(strings.TrimSpace(input.StoreSelectionStrategy)), StoreSelectionCustomerChoice)
	if !validStoreSelectionStrategy(config.StoreSelectionStrategy) {
		return CommerceWorkflowConfiguration{}, httperror.BadRequest("Store selection strategy is not supported")
	}
	actions, err := normalizeConfiguredValues(input.EnabledActions, workflowActionSet(), "runtime action")
	if err != nil {
		return CommerceWorkflowConfiguration{}, err
	}
	modes, err := normalizeConfiguredValues(input.SupportedFulfilmentModes, supportedWorkflowFulfilmentModes, "fulfilment mode")
	if err != nil {
		return CommerceWorkflowConfiguration{}, err
	}
	steps, err := normalizeConfiguredValues(input.PostPaymentSteps, supportedPostPaymentSteps, "post-payment step")
	if err != nil {
		return CommerceWorkflowConfiguration{}, err
	}
	config.EnabledActions = jsonValue(actions)
	config.SupportedFulfilmentModes = jsonValue(modes)
	config.PostPaymentSteps = jsonValue(steps)
	config.Metadata = jsonObject(input.Metadata)
	return config, nil
}

func commerceWorkflowView(config CommerceWorkflowConfiguration) CommerceWorkflowConfigurationView {
	return CommerceWorkflowConfigurationView{
		CommerceWorkflowConfiguration: config,
		EnabledActionList:             decodeStringList(config.EnabledActions),
		SupportedFulfilmentModeList:   decodeStringList(config.SupportedFulfilmentModes),
		PostPaymentStepList:           decodeStringList(config.PostPaymentSteps),
	}
}

func allWorkflowActionKeys() []string {
	keys := make([]string, 0, len(SystemActions()))
	for _, action := range SystemActions() {
		keys = append(keys, strings.ToLower(action.Key))
	}
	sort.Strings(keys)
	return keys
}

func workflowActionSet() map[string]struct{} {
	values := map[string]struct{}{}
	for _, action := range allWorkflowActionKeys() {
		values[action] = struct{}{}
	}
	return values
}

func normalizeConfiguredValues(values []string, allowed map[string]struct{}, label string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			return nil, httperror.BadRequest("Unsupported " + label + ": " + value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func decodeStringList(raw string) []string {
	values := []string{}
	if err := json.Unmarshal([]byte(defaultString(strings.TrimSpace(raw), "[]")), &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func validStoreSelectionStrategy(value string) bool {
	switch value {
	case StoreSelectionCustomerChoice, StoreSelectionSingleStore, StoreSelectionFirstAvailable, StoreSelectionNearest, StoreSelectionMerchantRule:
		return true
	default:
		return false
	}
}

func isOrderingMutation(action string) bool {
	switch action {
	case "create_cart", "get_or_create_cart", "add_to_cart", "update_cart_item", "remove_cart_item", "calculate_cart", "get_fulfilment_modes", "select_fulfilment_mode", "create_order", "cancel_order", "generate_invoice":
		return true
	default:
		return false
	}
}
