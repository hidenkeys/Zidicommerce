package bot

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

func (s *Service) CreateServiceBookingBot(ctx context.Context, actor auth.CurrentUser, name, welcome string) (Bot, error) {
	if !canManageBots(actor.Role) {
		return Bot{}, httperror.Forbidden("You cannot create bots")
	}
	if name == "" {
		name = "Service assistant"
	}
	if welcome == "" {
		welcome = "Hi 👋 Welcome. We help you find trusted professionals for home services.\n\nWhat service do you need today?"
	}
	botRecord := Bot{
		ID:              uuid.New(),
		OrganizationID:  actor.OrganizationID,
		Name:            name,
		Description:     "Configurable field-service booking flow",
		Status:          BotStatusDraft,
		DefaultLanguage: "en",
		Timezone:        "Africa/Lagos",
		FallbackConfig:  `{"message":"I didn't catch that. Please choose one of the options, or type menu to start again.","missing_variable":"not available"}`,
		HandoffConfig:   `{"enabled":true,"message":"A professional will continue this conversation here."}`,
		Metadata:        `{"builder":"service_booking","template":"field_service"}`,
	}
	var version BotVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&botRecord).Error; err != nil {
			return err
		}
		version = BotVersion{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: botRecord.ID, VersionNumber: 1, Status: VersionStatusDraft, StartStepKey: "start", ValidationErrors: "[]", Metadata: `{"template":"field_service"}`, CreatedByUserID: &actor.ID}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		modules := []VersionModule{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "WELCOME", Name: "Welcome", Category: "Entry", Source: ModuleSourceOrganization, Parameters: jsonValue(map[string]any{"entry_step": "start"}), Metadata: `{"enabled":true}`, SortOrder: 10},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "SERVICE_SELECTION", Name: "Service selection", Category: "Field service", Source: ModuleSourceOrganization, Parameters: jsonValue(map[string]any{"entry_step": "svc_list_pools", "menu_intent": "book_service", "menu_label": "Book a service"}), Metadata: `{"enabled":true}`, SortOrder: 20},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "SERVICE_BOOKING", Name: "Service booking", Category: "Field service", Source: ModuleSourceSystem, Parameters: jsonValue(map[string]any{"entry_step": "start", "menu_intent": "book_service", "menu_label": "Book a service"}), Metadata: `{"enabled":true}`, SortOrder: 30},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "HUMAN_HANDOFF", Name: "Human handoff", Category: "Support", Source: ModuleSourceSystem, Parameters: jsonValue(map[string]any{"entry_step": "svc_handoff"}), Metadata: `{"enabled":true}`, SortOrder: 40},
		}
		if err := tx.Create(&modules).Error; err != nil {
			return err
		}
		variables := []Variable{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "service_options", Type: "array", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "service_menu", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "service_choice", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "pool_id", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "service_name", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "customer_name", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "area", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "location", Type: "location", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "address", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "issue", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "preferred_at", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "request_id", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_url", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_reference", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_status", Type: "string", Scope: "conversation", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_choice", Type: "string", Scope: "conversation", Metadata: "{}"},
		}
		if err := tx.Create(&variables).Error; err != nil {
			return err
		}
		serviceQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "service_choice", Text: "{{service_menu}}\n\nReply with the number, or type the service name.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "service_choice", Options: "[]", Validation: "{}", Metadata: "{}"}
		nameQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "customer_name", Text: "What's your name?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "customer_name", Options: "[]", Validation: "{}", Metadata: "{}"}
		locQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "area", Text: "Which Lagos area are you in? You can also share your WhatsApp location. (Lekki, Ikeja, Yaba...)", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "area", Options: "[]", Validation: "{}", Metadata: "{}"}
		addrQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "address", Text: "What's the address or landmark?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "address", Options: "[]", Validation: "{}", Metadata: "{}"}
		issueQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "issue", Text: "Briefly describe the problem.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "issue", Options: "[]", Validation: "{}", Metadata: "{}"}
		timeQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "preferred_at", Text: "When would you like someone to come? (today, tomorrow morning, etc.)", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "preferred_at", Options: "[]", Validation: "{}", Metadata: "{}"}
		payQ := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "payment_choice", Text: "After opening the payment link, choose what to do next.", Type: "single_choice", ResponseMode: "buttons", Required: true, VariableName: "payment_choice", Options: jsonValue([]map[string]string{{"id": "paid", "label": "I have paid"}, {"id": "retry", "label": "Send link again"}}), Validation: "{}", Metadata: "{}"}
		if err := tx.Create([]Question{serviceQ, nameQ, locQ, addrQ, issueQ, timeQ, payQ}).Error; err != nil {
			return err
		}
		conditions := []Condition{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "payment_paid_choice", Name: "Paid", Combinator: "and", Rules: conditionJSON("payment_choice", "equals", "paid"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "payment_success", Name: "Payment confirmed", Combinator: "and", Rules: conditionJSON("payment_status", "equals", "paid"), Metadata: "{}"},
		}
		if err := tx.Create(&conditions).Error; err != nil {
			return err
		}
		actions := []Action{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "svc_welcome", ActionType: "get_service_welcome", Name: "Welcome", InputMappings: "{}", OutputMappings: "{}", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "svc_list_pools", ActionType: "list_service_pools", Name: "List services", InputMappings: "{}", OutputMappings: `{"service_options":"variables.service_options","menu":"variables.service_menu"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "svc_select_pool", ActionType: "select_service_pool", Name: "Select service", InputMappings: `{"selection":"service_choice","service_options":"variables.service_options"}`, OutputMappings: `{"pool_id":"variables.pool_id","service_name":"variables.service_name"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "svc_create_request", ActionType: "create_service_request", Name: "Create request", InputMappings: `{"pool_id":"variables.pool_id","customer_name":"customer_name","address":"address","description":"issue","preferred_at":"preferred_at","area":"area"}`, OutputMappings: `{"request_id":"variables.request_id"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "svc_booking_fee", ActionType: "initialize_booking_fee", Name: "Booking fee", InputMappings: `{"request_id":"variables.request_id"}`, OutputMappings: `{"payment_url":"variables.payment_url","payment_reference":"variables.payment_reference"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "svc_check_payment", ActionType: "check_booking_payment", Name: "Check booking payment", InputMappings: `{"payment_reference":"variables.payment_reference"}`, OutputMappings: `{"payment_status":"variables.payment_status"}`, Metadata: "{}"},
		}
		if err := tx.Create(&actions).Error; err != nil {
			return err
		}
		if err := tx.Create(&Integration{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Provider: "paystack", DisplayName: "Paystack", Required: true, Config: `{"source":"payment_configuration"}`, Metadata: "{}"}).Error; err != nil {
			return err
		}
		actionByKey := map[string]uuid.UUID{}
		for _, action := range actions {
			actionByKey[action.ActionKey] = action.ID
		}
		conditionByKey := map[string]uuid.UUID{}
		for _, condition := range conditions {
			conditionByKey[condition.ConditionKey] = condition.ID
		}
		steps := []Step{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "start", Type: StepAction, Title: "Welcome", ActionID: idPtr(actionByKey["svc_welcome"]), NextStepKey: "svc_list_pools", SortOrder: 10, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "svc_list_pools", Type: StepAction, Title: "List services", ActionID: idPtr(actionByKey["svc_list_pools"]), NextStepKey: "ask_service", SortOrder: 20, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_service", Type: StepQuestion, Title: "Ask service", QuestionID: &serviceQ.ID, NextStepKey: "svc_select_pool", SortOrder: 30, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "svc_select_pool", Type: StepAction, Title: "Select service", ActionID: idPtr(actionByKey["svc_select_pool"]), NextStepKey: "ask_name", SortOrder: 40, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_name", Type: StepQuestion, Title: "Ask name", QuestionID: &nameQ.ID, NextStepKey: "ask_location", SortOrder: 50, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_location", Type: StepQuestion, Title: "Ask location", QuestionID: &locQ.ID, NextStepKey: "ask_address", SortOrder: 60, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_address", Type: StepQuestion, Title: "Ask address", QuestionID: &addrQ.ID, NextStepKey: "ask_issue", SortOrder: 70, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_issue", Type: StepQuestion, Title: "Ask issue", QuestionID: &issueQ.ID, NextStepKey: "ask_time", SortOrder: 80, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_time", Type: StepQuestion, Title: "Ask time", QuestionID: &timeQ.ID, NextStepKey: "svc_create_request", SortOrder: 90, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "svc_create_request", Type: StepAction, Title: "Create request", ActionID: idPtr(actionByKey["svc_create_request"]), NextStepKey: "svc_booking_fee", SortOrder: 100, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "svc_booking_fee", Type: StepAction, Title: "Booking fee", ActionID: idPtr(actionByKey["svc_booking_fee"]), NextStepKey: "ask_payment", SortOrder: 110, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_payment", Type: StepChoice, Title: "Payment follow-up", QuestionID: &payQ.ID, NextStepKey: "route_paid", ResponseMode: "buttons", Options: payQ.Options, SortOrder: 120, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_paid", Type: StepCondition, Title: "Route paid", ConditionID: idPtr(conditionByKey["payment_paid_choice"]), NextStepKey: "svc_check_payment", FallbackStepKey: "svc_booking_fee", SortOrder: 130, Metadata: `{"allow_cycle":true}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "svc_check_payment", Type: StepAction, Title: "Check payment", ActionID: idPtr(actionByKey["svc_check_payment"]), NextStepKey: "route_payment_ok", SortOrder: 140, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_payment_ok", Type: StepCondition, Title: "Payment ok", ConditionID: idPtr(conditionByKey["payment_success"]), NextStepKey: "svc_handoff", FallbackStepKey: "ask_payment", SortOrder: 150, Metadata: `{"allow_cycle":true}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "svc_handoff", Type: StepHandoff, Title: "Wait for professional", Message: "We're matching you with a nearby professional. You can keep this chat open — we'll update you here.", SortOrder: 160, Metadata: "{}"},
		}
		for index := range steps {
			steps[index].Options = jsonArray(steps[index].Options)
			steps[index].Metadata = jsonObject(steps[index].Metadata)
		}
		if err := tx.Create(&steps).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot", botRecord.ID, "service_booking_bot_created", `{"template":"field_service"}`)
	})
	if err != nil {
		return Bot{}, err
	}
	return botRecord, nil
}
