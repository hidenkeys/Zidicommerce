package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db                *gorm.DB
	jobs              *jobs.Service
	embeddingsEnabled bool
	now               func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ConfigureJobs(jobService *jobs.Service) {
	s.jobs = jobService
}

func (s *Service) ConfigureKnowledgeEmbeddings(enabled bool) {
	s.embeddingsEnabled = enabled
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

func (s *Service) CreateSelfServiceBot(ctx context.Context, actor auth.CurrentUser, input SelfServiceBotInput) (Bot, error) {
	if !canManageBots(actor.Role) {
		return Bot{}, httperror.Forbidden("You cannot create bots")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Customer assistant"
	}
	welcome := defaultString(input.WelcomeMessage, "Welcome. I can help you place an order, track an order, answer questions, or contact support.")
	support := defaultString(input.SupportMessage, "A team member will take over this conversation shortly.")
	requirePayment := boolDefault(input.RequirePayment, true)
	allowPickup := boolDefault(input.AllowPickup, true)
	allowCustomerRider := boolDefault(input.AllowCustomerRider, true)
	allowMerchantRider := boolDefault(input.AllowMerchantRider, false)

	botRecord := Bot{
		ID:              uuid.New(),
		OrganizationID:  actor.OrganizationID,
		Name:            name,
		Description:     input.Description,
		Status:          BotStatusDraft,
		DefaultLanguage: "en",
		Timezone:        "Africa/Lagos",
		FallbackConfig:  `{"message":"I didn't recognize that option. Please choose one of the options above, or type menu to return to the main menu.","missing_variable":"not available"}`,
		HandoffConfig:   `{"enabled":true,"message":"A team member will take over this conversation shortly."}`,
		Metadata:        `{"builder":"self_service","phase":"8"}`,
	}
	var version BotVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&botRecord).Error; err != nil {
			return err
		}
		if err := EnsureDefaultCommerceWorkflowConfigurationTx(tx, actor.OrganizationID, actor.ID, name, welcome, true, requirePayment, true); err != nil {
			return err
		}
		version = BotVersion{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: botRecord.ID, VersionNumber: 1, Status: VersionStatusDraft, StartStepKey: "start", ValidationErrors: "[]", Metadata: `{"template":"commerce_support"}`, CreatedByUserID: &actor.ID}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}

		modules := []VersionModule{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "WELCOME", Name: "Welcome", Category: "Entry", Source: ModuleSourceOrganization, Description: "Customer entry menu.", Parameters: "{}", Metadata: `{"enabled":true}`, SortOrder: 10},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "ORDER", Name: "Order", Category: "Commerce", Source: ModuleSourceSystem, Description: "Customer order flow.", Parameters: jsonValue(map[string]any{"entry_step": "order_get_stores", "menu_intent": "order", "menu_label": "Place order", "require_payment": requirePayment, "allow_pickup": allowPickup, "allow_customer_rider": allowCustomerRider, "allow_merchant_rider": allowMerchantRider}), Metadata: `{"enabled":true}`, SortOrder: 20},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "TRACK_ORDER", Name: "Track Order", Category: "Commerce", Source: ModuleSourceSystem, Description: "Track order status.", Parameters: jsonValue(map[string]any{"entry_step": "track_get_orders", "menu_intent": "track_order", "menu_label": "Track order"}), Metadata: `{"enabled":true}`, SortOrder: 30},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "FAQ", Name: "FAQ", Category: "Support", Source: ModuleSourceSystem, Description: "Answer configured FAQs.", Parameters: jsonValue(map[string]any{"menu_intent": "faq", "menu_label": "Ask a question"}), Metadata: `{"enabled":true}`, SortOrder: 40},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "COMPLAINT", Name: "Complaint", Category: "Support", Source: ModuleSourceSystem, Description: "Collect customer complaints.", Parameters: jsonValue(map[string]any{"menu_intent": "complaint", "menu_label": "Report a complaint"}), Metadata: `{"enabled":true}`, SortOrder: 50},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "CONTACT_SUPPORT", Name: "Contact Support", Category: "Support", Source: ModuleSourceSystem, Description: "Share support options.", Parameters: jsonValue(map[string]any{"menu_intent": "support", "menu_label": "Contact support"}), Metadata: `{"enabled":true}`, SortOrder: 60},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ModuleKey: "HUMAN_HANDOFF", Name: "Human Handoff", Category: "Support", Source: ModuleSourceSystem, Description: "Pause automation for staff takeover.", Parameters: jsonValue(map[string]any{"menu_intent": "support", "menu_label": "Contact support"}), Metadata: `{"enabled":true}`, SortOrder: 70},
		}
		if err := tx.Create(&modules).Error; err != nil {
			return err
		}

		variables := []Variable{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "intent", Type: "string", Scope: "conversation", Description: "Customer selected entry path.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "order_reference", Type: "string", Scope: "conversation", Description: "Order number or reference for tracking.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "faq_query", Type: "string", Scope: "conversation", Description: "Customer FAQ question.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "faq_answer", Type: "string", Scope: "conversation", Description: "Matched FAQ answer.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "complaint_message", Type: "string", Scope: "conversation", Description: "Customer complaint details.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "stores", Type: "array", Scope: "conversation", Description: "Available stores shown to the customer.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "store_choice", Type: "string", Scope: "conversation", Description: "Customer store selection.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "store_id", Type: "string", Scope: "conversation", Description: "Selected store id.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "store_name", Type: "string", Scope: "conversation", Description: "Selected store name.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "store_address", Type: "string", Scope: "conversation", Description: "Selected store address.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "categories", Type: "array", Scope: "conversation", Description: "Available categories shown to the customer.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "category_choice", Type: "string", Scope: "conversation", Description: "Customer category selection.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "category_id", Type: "string", Scope: "conversation", Description: "Selected category id.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "category_name", Type: "string", Scope: "conversation", Description: "Selected category name.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "product_options", Type: "array", Scope: "conversation", Description: "Available products shown to the customer.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "product_choice", Type: "string", Scope: "conversation", Description: "Customer product selection.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "product_id", Type: "string", Scope: "conversation", Description: "Selected product id.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "product_name", Type: "string", Scope: "conversation", Description: "Selected product name.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "variant_id", Type: "string", Scope: "conversation", Description: "Selected variant id.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "variant_name", Type: "string", Scope: "conversation", Description: "Selected variant name.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "price_minor", Type: "number", Scope: "conversation", Description: "Selected variant price in minor units.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "quantity", Type: "number", Scope: "conversation", Description: "Selected quantity.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "cart_id", Type: "string", Scope: "conversation", Description: "Active cart id.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "cart_choice", Type: "string", Scope: "conversation", Description: "Continue shopping or review cart.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "cart_total_minor", Type: "number", Scope: "conversation", Description: "Cart total in minor units.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "fulfilment_options", Type: "array", Scope: "conversation", Description: "Enabled fulfilment modes.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "fulfilment_choice", Type: "string", Scope: "conversation", Description: "Customer fulfilment selection.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "fulfilment_mode", Type: "string", Scope: "conversation", Description: "Selected fulfilment mode.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "delivery_fee_minor", Type: "number", Scope: "conversation", Description: "Delivery fee in minor units.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "delivery_address", Type: "string", Scope: "conversation", Description: "Delivery address when required.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "customer_email", Type: "string", Scope: "conversation", Description: "Customer email for payment receipt.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "order_id", Type: "string", Scope: "conversation", Description: "Created order id.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "order_number", Type: "string", Scope: "conversation", Description: "Created order number.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "order_total_minor", Type: "number", Scope: "conversation", Description: "Order total in minor units.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_url", Type: "string", Scope: "conversation", Description: "Payment authorization URL.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_reference", Type: "string", Scope: "conversation", Description: "Payment reference.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_status", Type: "string", Scope: "conversation", Description: "Payment status.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "payment_choice", Type: "string", Scope: "conversation", Description: "Customer payment follow-up choice.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "orders", Type: "array", Scope: "conversation", Description: "Customer orders shown during tracking.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "track_order_choice", Type: "string", Scope: "conversation", Description: "Customer track-order selection.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "order_status", Type: "string", Scope: "conversation", Description: "Tracked order status.", Metadata: `{"read_only":false}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "system.customer.phone", Type: "string", Scope: "system", Description: "Current WhatsApp sender phone.", Metadata: `{"read_only":true}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Name: "system.channel", Type: "string", Scope: "system", Description: "Inbound channel provider.", Metadata: `{"read_only":true}`},
		}
		if err := tx.Create(&variables).Error; err != nil {
			return err
		}

		menuQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "main_menu", Text: welcome, Type: "single_choice", ResponseMode: "list", Required: true, VariableName: "intent", Options: jsonValue([]map[string]string{
			{"id": "order", "label": "Place order"},
			{"id": "track_order", "label": "Track order"},
			{"id": "faq", "label": "Ask a question"},
			{"id": "complaint", "label": "Report a complaint"},
			{"id": "support", "label": "Contact support"},
		}), Validation: "{}", Metadata: "{}"}
		storeQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "store_choice", Text: "Choose a store using its number.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "store_choice", Options: "[]", Validation: "{}", Metadata: "{}"}
		categoryQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "category_choice", Text: "Choose a category using its number.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "category_choice", Options: "[]", Validation: "{}", Metadata: "{}"}
		productQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "product_choice", Text: "Choose a product using its number.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "product_choice", Options: "[]", Validation: "{}", Metadata: "{}"}
		quantityQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "quantity", Text: "How many would you like? Enter a quantity from 1 to 100.", Type: "number", ResponseMode: "free_text", Required: true, VariableName: "quantity", Options: "[]", Validation: `{"min":1,"max":100}`, Metadata: "{}"}
		cartQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "cart_choice", Text: "Would you like to add another item or review your cart?", Type: "single_choice", ResponseMode: "buttons", Required: true, VariableName: "cart_choice", Options: jsonValue([]map[string]string{
			{"id": "add_more", "label": "Add another item"},
			{"id": "review", "label": "Review cart"},
		}), Validation: "{}", Metadata: "{}"}
		fulfilmentQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "fulfilment_choice", Text: "Choose a fulfilment option using its number.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "fulfilment_choice", Options: "[]", Validation: "{}", Metadata: "{}"}
		deliveryAddressQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "delivery_address", Text: "Please enter the delivery address. The delivery fee is paid by the customer and will be included in the order total.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "delivery_address", Options: "[]", Validation: "{}", Metadata: "{}"}
		emailQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "customer_email", Text: "Enter an email address for your order confirmation and receipt.", Type: "email", ResponseMode: "free_text", Required: true, VariableName: "customer_email", Options: "[]", Validation: "{}", Metadata: "{}"}
		paymentQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "payment_choice", Text: "After opening the payment link, choose what you want to do next.", Type: "single_choice", ResponseMode: "buttons", Required: true, VariableName: "payment_choice", Options: jsonValue([]map[string]string{
			{"id": "paid", "label": "I have paid"},
			{"id": "retry", "label": "Send payment link again"},
			{"id": "cancel", "label": "Cancel order"},
			{"id": "support", "label": "Talk to support"},
		}), Validation: "{}", Metadata: "{}"}
		trackOrderQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "track_order_choice", Text: "Which order would you like to track? Reply with the number next to it, or type menu to go back.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "track_order_choice", Options: "[]", Validation: "{}", Metadata: "{}"}
		faqQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "faq_query", Text: "What would you like to know?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "faq_query", Options: "[]", Validation: "{}", Metadata: "{}"}
		complaintQuestion := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, QuestionKey: "complaint_message", Text: "Tell us what happened. Include your order number if you have one.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "complaint_message", Options: "[]", Validation: "{}", Metadata: "{}"}
		questions := []Question{menuQuestion, storeQuestion, categoryQuestion, productQuestion, quantityQuestion, cartQuestion, fulfilmentQuestion, deliveryAddressQuestion, emailQuestion, paymentQuestion, trackOrderQuestion, faqQuestion, complaintQuestion}
		if err := tx.Create(&questions).Error; err != nil {
			return err
		}

		conditions := []Condition{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "intent_order", Name: "Customer wants to order", Combinator: "and", Rules: conditionJSON("intent", "equals", "order"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "intent_track", Name: "Customer wants to track an order", Combinator: "and", Rules: conditionJSON("intent", "equals", "track_order"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "intent_faq", Name: "Customer wants FAQs", Combinator: "and", Rules: conditionJSON("intent", "equals", "faq"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "intent_complaint", Name: "Customer wants complaint support", Combinator: "and", Rules: conditionJSON("intent", "equals", "complaint"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "cart_add_more", Name: "Customer wants another item", Combinator: "and", Rules: conditionJSON("cart_choice", "equals", "add_more"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "merchant_delivery", Name: "Merchant delivery requires address", Combinator: "and", Rules: conditionJSON("fulfilment_mode", "equals", core.FulfilmentMerchantRider), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "payment_paid_choice", Name: "Customer says payment is complete", Combinator: "and", Rules: conditionJSON("payment_choice", "equals", "paid"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "payment_retry_choice", Name: "Customer wants payment link again", Combinator: "and", Rules: conditionJSON("payment_choice", "equals", "retry"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "payment_cancel_choice", Name: "Customer cancels order before payment", Combinator: "and", Rules: conditionJSON("payment_choice", "equals", "cancel"), Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ConditionKey: "payment_success", Name: "Payment is confirmed", Combinator: "and", Rules: conditionJSON("payment_status", "equals", core.PaymentPaid), Metadata: "{}"},
		}
		if err := tx.Create(&conditions).Error; err != nil {
			return err
		}

		actions := []Action{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "handoff_support", ActionType: "handoff_to_agent", Name: "Handoff to support", InputMappings: `{"reason":"support"}`, OutputMappings: "{}", Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "answer_faq", ActionType: "match_faq", Name: "Answer FAQ", InputMappings: `{"query":"faq_query"}`, OutputMappings: `{"answer":"variables.faq_answer"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "record_complaint", ActionType: "create_complaint", Name: "Record complaint", InputMappings: `{"message":"complaint_message"}`, OutputMappings: `{"complaint_id":"complaint_id"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_get_stores", ActionType: "get_stores", Name: "Get open stores", InputMappings: `{}`, OutputMappings: `{"stores":"variables.stores"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_select_store", ActionType: "select_store", Name: "Select store", InputMappings: `{"stores":"variables.stores","selection":"store_choice"}`, OutputMappings: `{"store_id":"variables.store_id","store_name":"variables.store_name","store_address":"variables.store_address"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_create_cart", ActionType: "get_or_create_cart", Name: "Create cart", InputMappings: `{"customer_id":"session.customer_id","store_id":"variables.store_id","currency":"NGN"}`, OutputMappings: `{"cart_id":"variables.cart_id"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_get_categories", ActionType: "get_categories", Name: "Get categories", InputMappings: `{}`, OutputMappings: `{"categories":"variables.categories"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_select_category", ActionType: "select_category", Name: "Select category", InputMappings: `{"categories":"variables.categories","selection":"category_choice"}`, OutputMappings: `{"category_id":"variables.category_id","category_name":"variables.category_name"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_get_products", ActionType: "get_products", Name: "Get products", InputMappings: `{"category_id":"variables.category_id","store_id":"variables.store_id"}`, OutputMappings: `{"product_options":"variables.product_options"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_select_product", ActionType: "select_product", Name: "Select product", InputMappings: `{"product_options":"variables.product_options","selection":"product_choice"}`, OutputMappings: `{"product_id":"variables.product_id","product_name":"variables.product_name","variant_id":"variables.variant_id","variant_name":"variables.variant_name","price_minor":"variables.price_minor"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_add_to_cart", ActionType: "add_to_cart", Name: "Add to cart", InputMappings: `{"cart_id":"variables.cart_id","variant_id":"variables.variant_id","quantity":"quantity"}`, OutputMappings: `{"total_minor":"variables.cart_total_minor"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_review_cart", ActionType: "calculate_cart", Name: "Review cart", InputMappings: `{"cart_id":"variables.cart_id"}`, OutputMappings: `{"total_minor":"variables.cart_total_minor"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_get_fulfilment", ActionType: "get_fulfilment_modes", Name: "Get fulfilment options", InputMappings: `{"store_id":"variables.store_id"}`, OutputMappings: `{"fulfilment_options":"variables.fulfilment_options"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_select_fulfilment", ActionType: "select_fulfilment_mode", Name: "Select fulfilment", InputMappings: `{"store_id":"variables.store_id","fulfilment_options":"variables.fulfilment_options","selection":"fulfilment_choice"}`, OutputMappings: `{"fulfilment_type":"variables.fulfilment_mode","delivery_fee_minor":"variables.delivery_fee_minor"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_create_order", ActionType: "create_order", Name: "Create order", InputMappings: `{"cart_id":"variables.cart_id","customer_id":"session.customer_id","store_id":"variables.store_id","fulfilment_type":"variables.fulfilment_mode","currency":"NGN"}`, OutputMappings: `{"order_id":"variables.order_id","order_number":"variables.order_number","total_minor":"variables.order_total_minor"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_create_delivery_order", ActionType: "create_order", Name: "Create delivery order", InputMappings: `{"cart_id":"variables.cart_id","customer_id":"session.customer_id","store_id":"variables.store_id","fulfilment_type":"variables.fulfilment_mode","delivery_address":"delivery_address","currency":"NGN"}`, OutputMappings: `{"order_id":"variables.order_id","order_number":"variables.order_number","total_minor":"variables.order_total_minor"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "track_get_orders", ActionType: "get_customer_orders", Name: "Get customer orders", InputMappings: `{"customer_id":"session.customer_id"}`, OutputMappings: `{"orders":"variables.orders"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "track_select_order", ActionType: "select_order", Name: "Select order", InputMappings: `{"orders":"variables.orders","selection":"track_order_choice"}`, OutputMappings: `{"order_id":"variables.order_id","order_number":"variables.order_number"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "track_get_status", ActionType: "get_order_status", Name: "Get order status", InputMappings: `{"order_id":"variables.order_id"}`, OutputMappings: `{"order_status":"variables.order_status"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_check_payment", ActionType: "check_payment", Name: "Check payment", InputMappings: `{"payment_reference":"variables.payment_reference"}`, OutputMappings: `{"payment_status":"variables.payment_status"}`, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_cancel", ActionType: "cancel_order", Name: "Cancel order", InputMappings: `{"order_id":"variables.order_id"}`, OutputMappings: `{"order_status":"variables.order_status"}`, Metadata: "{}"},
		}
		if requirePayment {
			actions = append(actions, Action{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, ActionKey: "order_initialize_payment", ActionType: "initialize_payment", Name: "Initialize payment", InputMappings: `{"order_id":"variables.order_id","email":"customer_email","provider":"paystack"}`, OutputMappings: `{"payment_url":"variables.payment_url","payment_reference":"variables.payment_reference","payment_status":"variables.payment_status"}`, Metadata: "{}"})
		}
		if err := tx.Create(&actions).Error; err != nil {
			return err
		}
		if requirePayment {
			integration := Integration{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, Provider: "paystack", DisplayName: "Paystack", Required: true, Config: `{"source":"payment_configuration"}`, Metadata: "{}"}
			if err := tx.Create(&integration).Error; err != nil {
				return err
			}
		}

		moduleByKey := map[string]uuid.UUID{}
		for _, module := range modules {
			moduleByKey[module.ModuleKey] = module.ID
		}
		conditionByKey := map[string]uuid.UUID{}
		for _, condition := range conditions {
			conditionByKey[condition.ConditionKey] = condition.ID
		}
		actionByKey := map[string]uuid.UUID{}
		for _, action := range actions {
			actionByKey[action.ActionKey] = action.ID
		}
		steps := []Step{
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "start", Type: StepChoice, Title: "Main menu", QuestionID: &menuQuestion.ID, NextStepKey: "route_order", SortOrder: 10, ResponseMode: "list", Options: menuQuestion.Options, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_order", Type: StepCondition, Title: "Route order", ConditionID: idPtr(conditionByKey["intent_order"]), NextStepKey: "order_module", FallbackStepKey: "route_track", SortOrder: 20, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_track", Type: StepCondition, Title: "Route tracking", ConditionID: idPtr(conditionByKey["intent_track"]), NextStepKey: "track_module", FallbackStepKey: "route_faq", SortOrder: 30, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_faq", Type: StepCondition, Title: "Route FAQ", ConditionID: idPtr(conditionByKey["intent_faq"]), NextStepKey: "ask_faq", FallbackStepKey: "route_complaint", SortOrder: 40, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_complaint", Type: StepCondition, Title: "Route complaint", ConditionID: idPtr(conditionByKey["intent_complaint"]), NextStepKey: "ask_complaint", FallbackStepKey: "support_handoff", SortOrder: 50, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_module", Type: StepModule, Title: "Order module", ModuleID: idPtr(moduleByKey["ORDER"]), SortOrder: 60, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "track_module", Type: StepModule, Title: "Track order module", ModuleID: idPtr(moduleByKey["TRACK_ORDER"]), SortOrder: 70, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_get_stores", Type: StepAction, Title: "Get stores", ActionID: idPtr(actionByKey["order_get_stores"]), NextStepKey: "ask_store", SortOrder: 80, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_store", Type: StepQuestion, Title: "Ask store", QuestionID: &storeQuestion.ID, NextStepKey: "order_select_store", SortOrder: 90, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_select_store", Type: StepAction, Title: "Select store", ActionID: idPtr(actionByKey["order_select_store"]), NextStepKey: "order_create_cart", SortOrder: 100, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_create_cart", Type: StepAction, Title: "Create cart", ActionID: idPtr(actionByKey["order_create_cart"]), NextStepKey: "order_get_categories", SortOrder: 110, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_get_categories", Type: StepAction, Title: "Get categories", ActionID: idPtr(actionByKey["order_get_categories"]), NextStepKey: "ask_category", SortOrder: 120, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_category", Type: StepQuestion, Title: "Ask category", QuestionID: &categoryQuestion.ID, NextStepKey: "order_select_category", SortOrder: 130, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_select_category", Type: StepAction, Title: "Select category", ActionID: idPtr(actionByKey["order_select_category"]), NextStepKey: "order_get_products", SortOrder: 140, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_get_products", Type: StepAction, Title: "Get products", ActionID: idPtr(actionByKey["order_get_products"]), NextStepKey: "ask_product", SortOrder: 150, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_product", Type: StepQuestion, Title: "Ask product", QuestionID: &productQuestion.ID, NextStepKey: "order_select_product", SortOrder: 160, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_select_product", Type: StepAction, Title: "Select product", ActionID: idPtr(actionByKey["order_select_product"]), NextStepKey: "ask_quantity", SortOrder: 170, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_quantity", Type: StepQuestion, Title: "Ask quantity", QuestionID: &quantityQuestion.ID, NextStepKey: "order_add_to_cart", SortOrder: 180, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_add_to_cart", Type: StepAction, Title: "Add to cart", ActionID: idPtr(actionByKey["order_add_to_cart"]), NextStepKey: "ask_cart_choice", SortOrder: 190, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_cart_choice", Type: StepChoice, Title: "Ask cart choice", QuestionID: &cartQuestion.ID, NextStepKey: "route_cart_choice", SortOrder: 200, ResponseMode: "buttons", Options: cartQuestion.Options, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_cart_choice", Type: StepCondition, Title: "Route cart choice", ConditionID: idPtr(conditionByKey["cart_add_more"]), NextStepKey: "order_get_categories", FallbackStepKey: "order_review_cart", SortOrder: 210, Metadata: `{"allow_cycle":true}`},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_review_cart", Type: StepAction, Title: "Review cart", ActionID: idPtr(actionByKey["order_review_cart"]), NextStepKey: "order_get_fulfilment", SortOrder: 220, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_get_fulfilment", Type: StepAction, Title: "Get fulfilment", ActionID: idPtr(actionByKey["order_get_fulfilment"]), NextStepKey: "ask_fulfilment", SortOrder: 230, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_fulfilment", Type: StepQuestion, Title: "Ask fulfilment", QuestionID: &fulfilmentQuestion.ID, NextStepKey: "order_select_fulfilment", SortOrder: 240, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_select_fulfilment", Type: StepAction, Title: "Select fulfilment", ActionID: idPtr(actionByKey["order_select_fulfilment"]), NextStepKey: "route_delivery_address", SortOrder: 250, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_delivery_address", Type: StepCondition, Title: "Route delivery address", ConditionID: idPtr(conditionByKey["merchant_delivery"]), NextStepKey: "ask_delivery_address", FallbackStepKey: "ask_customer_email", SortOrder: 260, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_delivery_address", Type: StepQuestion, Title: "Ask delivery address", QuestionID: &deliveryAddressQuestion.ID, NextStepKey: "ask_customer_email_delivery", SortOrder: 270, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_customer_email", Type: StepQuestion, Title: "Ask customer email", QuestionID: &emailQuestion.ID, NextStepKey: "order_create_order", SortOrder: 280, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_create_order", Type: StepAction, Title: "Create order", ActionID: idPtr(actionByKey["order_create_order"]), NextStepKey: "order_finish", SortOrder: 290, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_customer_email_delivery", Type: StepQuestion, Title: "Ask delivery customer email", QuestionID: &emailQuestion.ID, NextStepKey: "order_create_delivery_order", SortOrder: 292, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_create_delivery_order", Type: StepAction, Title: "Create delivery order", ActionID: idPtr(actionByKey["order_create_delivery_order"]), NextStepKey: "order_finish", SortOrder: 294, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "track_get_orders", Type: StepAction, Title: "Get customer orders", ActionID: idPtr(actionByKey["track_get_orders"]), NextStepKey: "ask_track_order", SortOrder: 320, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_track_order", Type: StepQuestion, Title: "Ask tracked order", QuestionID: &trackOrderQuestion.ID, NextStepKey: "track_select_order", SortOrder: 330, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "track_select_order", Type: StepAction, Title: "Select tracked order", ActionID: idPtr(actionByKey["track_select_order"]), NextStepKey: "track_get_status", SortOrder: 340, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "track_get_status", Type: StepAction, Title: "Get tracked order status", ActionID: idPtr(actionByKey["track_get_status"]), NextStepKey: "end", SortOrder: 350, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_faq", Type: StepQuestion, Title: "Ask FAQ", QuestionID: &faqQuestion.ID, NextStepKey: "answer_faq", SortOrder: 360, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "answer_faq", Type: StepAction, Title: "Answer FAQ", ActionID: idPtr(actionByKey["answer_faq"]), NextStepKey: "end", SortOrder: 370, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_complaint", Type: StepQuestion, Title: "Ask complaint", QuestionID: &complaintQuestion.ID, NextStepKey: "record_complaint", SortOrder: 380, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "record_complaint", Type: StepAction, Title: "Record complaint", ActionID: idPtr(actionByKey["record_complaint"]), NextStepKey: "complaint_done", SortOrder: 390, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "complaint_done", Type: StepMessage, Title: "Complaint received", Message: "Thank you. Your complaint has been recorded and support will follow up.", NextStepKey: "end", SortOrder: 400, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "support_handoff", Type: StepHandoff, Title: "Support handoff", ActionID: idPtr(actionByKey["handoff_support"]), Message: support, SortOrder: 410, Metadata: "{}"},
			{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "end", Type: StepEnd, Title: "End", Message: "Thanks for chatting with us.", SortOrder: 420, Metadata: "{}"},
		}
		if requirePayment {
			for index := range steps {
				if steps[index].StepKey == "order_create_order" || steps[index].StepKey == "order_create_delivery_order" {
					steps[index].NextStepKey = "order_initialize_payment"
				}
			}
			steps = append(steps,
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_initialize_payment", Type: StepAction, Title: "Initialize payment", ActionID: idPtr(actionByKey["order_initialize_payment"]), NextStepKey: "ask_payment_choice", SortOrder: 300, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "ask_payment_choice", Type: StepChoice, Title: "Ask payment choice", QuestionID: &paymentQuestion.ID, NextStepKey: "route_payment_paid_choice", SortOrder: 302, ResponseMode: "buttons", Options: paymentQuestion.Options, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_payment_paid_choice", Type: StepCondition, Title: "Route paid choice", ConditionID: idPtr(conditionByKey["payment_paid_choice"]), NextStepKey: "order_check_payment", FallbackStepKey: "route_payment_retry_choice", SortOrder: 304, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_payment_retry_choice", Type: StepCondition, Title: "Route payment retry", ConditionID: idPtr(conditionByKey["payment_retry_choice"]), NextStepKey: "order_initialize_payment", FallbackStepKey: "route_payment_cancel_choice", SortOrder: 306, Metadata: `{"allow_cycle":true}`},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_payment_cancel_choice", Type: StepCondition, Title: "Route payment cancel", ConditionID: idPtr(conditionByKey["payment_cancel_choice"]), NextStepKey: "order_cancel", FallbackStepKey: "support_handoff", SortOrder: 308, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_check_payment", Type: StepAction, Title: "Check payment", ActionID: idPtr(actionByKey["order_check_payment"]), NextStepKey: "route_payment_success", SortOrder: 310, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "route_payment_success", Type: StepCondition, Title: "Route payment status", ConditionID: idPtr(conditionByKey["payment_success"]), NextStepKey: "order_payment_confirmed", FallbackStepKey: "order_payment_pending", SortOrder: 312, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_payment_pending", Type: StepMessage, Title: "Payment pending", Message: "Your payment has not been completed yet. You can retry the payment link, cancel the order, or talk to support.", NextStepKey: "ask_payment_choice", SortOrder: 314, Metadata: `{"allow_cycle":true}`},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_payment_confirmed", Type: StepEnd, Title: "Payment confirmed", SortOrder: 316, Metadata: "{}"},
				Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_cancel", Type: StepAction, Title: "Cancel order", ActionID: idPtr(actionByKey["order_cancel"]), NextStepKey: "end", SortOrder: 318, Metadata: "{}"},
			)
		}
		if !requirePayment {
			steps = append(steps, Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: version.ID, StepKey: "order_finish", Type: StepEnd, Title: "Order finished", Message: "Your order has been received. We will send updates here as it progresses.", SortOrder: 310, Metadata: "{}"})
		}
		for index := range steps {
			steps[index].Options = jsonArray(steps[index].Options)
			steps[index].Metadata = jsonObject(steps[index].Metadata)
		}
		if err := tx.Create(&steps).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot", botRecord.ID, "self_service_bot_created", `{"template":"commerce_support"}`)
	})
	return botRecord, err
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

func (s *Service) UpdateModule(ctx context.Context, actor auth.CurrentUser, moduleID uuid.UUID, input ModuleInput) (VersionModule, error) {
	var module VersionModule
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, moduleID).First(&module).Error; err != nil {
		return VersionModule{}, mapNotFound(err, "Module not found")
	}
	if err := s.ensureEditable(ctx, actor, module.VersionID); err != nil {
		return VersionModule{}, err
	}
	updates := map[string]any{"updated_at": s.now()}
	if input.Name != "" {
		updates["name"] = strings.TrimSpace(input.Name)
	}
	if input.Category != "" {
		updates["category"] = strings.TrimSpace(input.Category)
	}
	if input.Source != "" {
		updates["source"] = strings.TrimSpace(input.Source)
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Parameters != "" {
		updates["parameters"] = jsonObject(input.Parameters)
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	if input.SortOrder != 0 {
		updates["sort_order"] = input.SortOrder
	}
	if err := s.db.WithContext(ctx).Model(&module).Updates(updates).Error; err != nil {
		return VersionModule{}, err
	}
	_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot_module", module.ID, "bot_module_updated", fmt.Sprintf(`{"module_key":%q}`, module.ModuleKey))
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, moduleID).First(&module).Error; err != nil {
		return VersionModule{}, err
	}
	return module, nil
}

func (s *Service) SetModuleEnabled(ctx context.Context, actor auth.CurrentUser, moduleID uuid.UUID, enabled bool) (VersionModule, error) {
	var module VersionModule
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, moduleID).First(&module).Error; err != nil {
		return VersionModule{}, mapNotFound(err, "Module not found")
	}
	if err := s.ensureEditable(ctx, actor, module.VersionID); err != nil {
		return VersionModule{}, err
	}
	metadata := jsonWithBool(module.Metadata, "enabled", enabled)
	if err := s.db.WithContext(ctx).Model(&module).Updates(map[string]any{"metadata": metadata, "updated_at": s.now()}).Error; err != nil {
		return VersionModule{}, err
	}
	_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot_module", module.ID, "bot_module_enabled_changed", fmt.Sprintf(`{"module_key":%q,"enabled":%t}`, module.ModuleKey, enabled))
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, moduleID).First(&module).Error; err != nil {
		return VersionModule{}, err
	}
	return module, nil
}

func (s *Service) ReorderModules(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, input ModuleReorderInput) ([]VersionModule, error) {
	if err := s.ensureEditable(ctx, actor, versionID); err != nil {
		return nil, err
	}
	if len(input.Modules) == 0 {
		return nil, httperror.BadRequest("At least one module order is required")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range input.Modules {
			if item.ModuleID == uuid.Nil {
				return httperror.BadRequest("Module ID is required")
			}
			result := tx.Model(&VersionModule{}).Where("organization_id = ? AND version_id = ? AND id = ?", actor.OrganizationID, versionID, item.ModuleID).Updates(map[string]any{"sort_order": item.SortOrder, "updated_at": s.now()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return httperror.BadRequest("Module order contains an unknown module")
			}
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot_version", versionID, "bot_modules_reordered", fmt.Sprintf(`{"count":%d}`, len(input.Modules)))
	})
	if err != nil {
		return nil, err
	}
	return s.ListModules(ctx, actor, versionID)
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
	if !actor.Role.HasPermission(authz.PermissionBotPublish) {
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

func (s *Service) ListFAQs(ctx context.Context, actor auth.CurrentUser) ([]FAQ, error) {
	if !canViewBots(actor.Role) {
		return nil, httperror.Forbidden("You cannot view bot knowledge")
	}
	var faqs []FAQ
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("updated_at DESC").Find(&faqs).Error
	return faqs, err
}

func (s *Service) ListKnowledgeEntries(ctx context.Context, actor auth.CurrentUser, filter KnowledgeEntryFilter) ([]KnowledgeEntry, error) {
	if !canViewKnowledge(actor.Role) {
		return nil, httperror.Forbidden("You cannot view merchant knowledge")
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID)
	if status := strings.ToLower(strings.TrimSpace(filter.Status)); status != "" {
		if !validKnowledgeStatus(status) {
			return nil, httperror.BadRequest("Knowledge status is not valid")
		}
		query = query.Where("status = ?", status)
	}
	if kind := strings.ToLower(strings.TrimSpace(filter.Kind)); kind != "" {
		if !validKnowledgeKind(kind) {
			return nil, httperror.BadRequest("Knowledge kind is not valid")
		}
		query = query.Where("kind = ?", kind)
	}
	if category := strings.ToLower(strings.TrimSpace(filter.Category)); category != "" {
		query = query.Where("category = ?", category)
	}
	if search := normalizeSearchText(filter.Search); search != "" {
		pattern := "%" + search + "%"
		query = query.Where(
			"LOWER(title) LIKE ? OR LOWER(question) LIKE ? OR LOWER(answer) LIKE ? OR LOWER(CAST(keywords AS TEXT)) LIKE ?",
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}
	var entries []KnowledgeEntry
	err := query.Order("updated_at DESC").Find(&entries).Error
	return entries, err
}

func (s *Service) GetKnowledgeEntry(ctx context.Context, actor auth.CurrentUser, entryID uuid.UUID) (KnowledgeEntry, error) {
	if !canViewKnowledge(actor.Role) {
		return KnowledgeEntry{}, httperror.Forbidden("You cannot view merchant knowledge")
	}
	var entry KnowledgeEntry
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, entryID).First(&entry).Error; err != nil {
		return KnowledgeEntry{}, mapNotFound(err, "Knowledge entry not found")
	}
	return entry, nil
}

func (s *Service) CreateKnowledgeEntry(ctx context.Context, actor auth.CurrentUser, input KnowledgeEntryInput) (KnowledgeEntry, error) {
	if !canManageKnowledge(actor.Role) {
		return KnowledgeEntry{}, httperror.Forbidden("You cannot manage merchant knowledge")
	}
	entry, err := knowledgeEntryFromInput(actor.OrganizationID, input, true)
	if err != nil {
		return KnowledgeEntry{}, err
	}
	s.applyKnowledgeEmbeddingLifecycle(&entry)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if err := s.enqueueKnowledgeEmbeddingTx(ctx, tx, actor.OrganizationID, entry); err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_knowledge_entry", entry.ID, "knowledge_entry_created", knowledgeAuditMetadata(entry))
	})
	return entry, err
}

func (s *Service) UpdateKnowledgeEntry(ctx context.Context, actor auth.CurrentUser, entryID uuid.UUID, input KnowledgeEntryInput) (KnowledgeEntry, error) {
	if !canManageKnowledge(actor.Role) {
		return KnowledgeEntry{}, httperror.Forbidden("You cannot manage merchant knowledge")
	}
	var entry KnowledgeEntry
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, entryID).First(&entry).Error; err != nil {
		return KnowledgeEntry{}, mapNotFound(err, "Knowledge entry not found")
	}
	updates, err := knowledgeEntryUpdates(input)
	if err != nil {
		return KnowledgeEntry{}, err
	}
	if len(updates) == 0 {
		return entry, nil
	}
	applyKnowledgeEmbeddingUpdates(s.embeddingsEnabled, entry, updates)
	updates["updated_at"] = s.now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entry).Updates(updates).Error; err != nil {
			return err
		}
		updated := entry
		applyKnowledgeEntryMap(&updated, updates)
		if err := s.enqueueKnowledgeEmbeddingTx(ctx, tx, actor.OrganizationID, updated); err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_knowledge_entry", entry.ID, "knowledge_entry_updated", jsonValue(map[string]any{"fields": sortedMapKeys(updates)}))
	})
	if err != nil {
		return KnowledgeEntry{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, entryID).First(&entry).Error; err != nil {
		return KnowledgeEntry{}, err
	}
	return entry, nil
}

func (s *Service) ArchiveKnowledgeEntry(ctx context.Context, actor auth.CurrentUser, entryID uuid.UUID) (KnowledgeEntry, error) {
	if !canManageKnowledge(actor.Role) {
		return KnowledgeEntry{}, httperror.Forbidden("You cannot manage merchant knowledge")
	}
	var entry KnowledgeEntry
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, entryID).First(&entry).Error; err != nil {
		return KnowledgeEntry{}, mapNotFound(err, "Knowledge entry not found")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entry).Updates(map[string]any{"status": "archived", "embedding_status": KnowledgeEmbeddingStatusDisabled, "embedding": nil, "embedding_error": "", "updated_at": s.now()}).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_knowledge_entry", entry.ID, "knowledge_entry_archived", knowledgeAuditMetadata(entry))
	})
	if err != nil {
		return KnowledgeEntry{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, entryID).First(&entry).Error; err != nil {
		return KnowledgeEntry{}, err
	}
	return entry, nil
}

func (s *Service) CreateFAQ(ctx context.Context, actor auth.CurrentUser, input FAQInput) (FAQ, error) {
	if !canManageBots(actor.Role) {
		return FAQ{}, httperror.Forbidden("You cannot manage bot knowledge")
	}
	question := strings.TrimSpace(input.Question)
	answer := strings.TrimSpace(input.Answer)
	if question == "" || answer == "" {
		return FAQ{}, httperror.BadRequest("FAQ question and answer are required")
	}
	faq := FAQ{ID: uuid.New(), OrganizationID: actor.OrganizationID, Question: question, Answer: answer, Keywords: jsonValue(cleanKeywords(input.Keywords)), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), core.StatusActive), Metadata: jsonObject(input.Metadata)}
	if faq.Status != core.StatusActive && faq.Status != "draft" && faq.Status != "archived" {
		return FAQ{}, httperror.BadRequest("FAQ status is not valid")
	}
	err := s.db.WithContext(ctx).Create(&faq).Error
	if err == nil {
		_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot_faq", faq.ID, "faq_created", "{}")
	}
	return faq, err
}

func (s *Service) UpdateFAQ(ctx context.Context, actor auth.CurrentUser, faqID uuid.UUID, input FAQInput) (FAQ, error) {
	if !canManageBots(actor.Role) {
		return FAQ{}, httperror.Forbidden("You cannot manage bot knowledge")
	}
	var faq FAQ
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, faqID).First(&faq).Error; err != nil {
		return FAQ{}, mapNotFound(err, "FAQ not found")
	}
	updates := map[string]any{"updated_at": s.now()}
	if strings.TrimSpace(input.Question) != "" {
		updates["question"] = strings.TrimSpace(input.Question)
	}
	if strings.TrimSpace(input.Answer) != "" {
		updates["answer"] = strings.TrimSpace(input.Answer)
	}
	if input.Keywords != nil {
		updates["keywords"] = jsonValue(cleanKeywords(input.Keywords))
	}
	if strings.TrimSpace(input.Status) != "" {
		status := strings.ToLower(strings.TrimSpace(input.Status))
		if status != core.StatusActive && status != "draft" && status != "archived" {
			return FAQ{}, httperror.BadRequest("FAQ status is not valid")
		}
		updates["status"] = status
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	if err := s.db.WithContext(ctx).Model(&faq).Updates(updates).Error; err != nil {
		return FAQ{}, err
	}
	_ = auditTx(s.db.WithContext(ctx), actor.OrganizationID, actor.ID, "bot_faq", faq.ID, "faq_updated", "{}")
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, faqID).First(&faq).Error; err != nil {
		return FAQ{}, err
	}
	return faq, nil
}

func (s *Service) MatchFAQ(ctx context.Context, actor auth.CurrentUser, query string) (FAQMatch, error) {
	if !canViewBots(actor.Role) {
		return FAQMatch{}, httperror.Forbidden("You cannot view bot knowledge")
	}
	query = normalizeSearchText(query)
	if query == "" {
		return FAQMatch{}, httperror.BadRequest("FAQ query is required")
	}
	var faqs []FAQ
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND status = ?", actor.OrganizationID, core.StatusActive).Find(&faqs).Error; err != nil {
		return FAQMatch{}, err
	}
	best := FAQMatch{}
	for _, faq := range faqs {
		score, matchedOn := scoreFAQ(query, faq)
		if score > best.Score {
			best = FAQMatch{FAQ: faq, Score: score, MatchedOn: matchedOn, Confidence: faqConfidence(score)}
		}
	}
	if best.FAQ.ID == uuid.Nil || best.Score < 0.24 {
		return FAQMatch{}, httperror.NotFound("No FAQ answer matched that question")
	}
	return best, nil
}

func (s *Service) GetShareLink(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID) (ShareLink, error) {
	botRecord, err := s.GetBot(ctx, actor, botID)
	if err != nil {
		return ShareLink{}, err
	}
	if botRecord.PublishedVersionID == nil {
		return ShareLink{Available: false, Reason: "Publish a bot version first."}, nil
	}
	var channel core.Channel
	err = s.db.WithContext(ctx).Where("organization_id = ? AND provider = ? AND status IN ?", actor.OrganizationID, "whatsapp", []string{core.StatusActive, "connected", "healthy", "degraded", "requires_attention"}).Order("updated_at DESC").First(&channel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return ShareLink{Available: false, Reason: "Connect an active WhatsApp channel first."}, nil
		}
		return ShareLink{}, err
	}
	displayNumber := strings.TrimSpace(channel.DisplayNumber)
	if displayNumber == "" {
		var configuration struct {
			DisplayPhoneNumber string
		}
		if configErr := s.db.WithContext(ctx).
			Table("channel_whatsapp_configs").
			Select("display_phone_number").
			Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, channel.ID).
			Take(&configuration).Error; configErr == nil {
			displayNumber = strings.TrimSpace(configuration.DisplayPhoneNumber)
		} else if configErr != gorm.ErrRecordNotFound {
			return ShareLink{}, configErr
		}
	}
	number := strings.TrimPrefix(strings.ReplaceAll(displayNumber, " ", ""), "+")
	if number == "" {
		return ShareLink{Available: false, Reason: "Add a WhatsApp display number to the active channel."}, nil
	}
	message := "Hi Zidi, I would like to place an order."
	if botRecord.Name != "" {
		message = "Hi Zidi, I would like to chat with " + botRecord.Name + "."
	}
	encoded := url.QueryEscape(message)
	return ShareLink{Available: true, URL: "https://wa.me/" + number + "?text=" + encoded, EncodedText: encoded, DisplayNumber: number, Message: message}, nil
}

func (s *Service) GetSetupStatus(ctx context.Context, actor auth.CurrentUser) (BotSetupStatus, error) {
	if !canViewBots(actor.Role) {
		return BotSetupStatus{}, httperror.Forbidden("You cannot view setup status")
	}
	count := func(model any, where string, args ...any) int64 {
		var total int64
		_ = s.db.WithContext(ctx).Model(model).Where(where, args...).Count(&total).Error
		return total
	}
	orgID := actor.OrganizationID
	var org organization.Organization
	organizationReady := s.db.WithContext(ctx).Where("id = ?", orgID).First(&org).Error == nil &&
		strings.TrimSpace(org.Name) != "" && strings.TrimSpace(org.Country) != "" &&
		strings.TrimSpace(org.Currency) != "" && strings.TrimSpace(org.Timezone) != ""
	knowledgeReady := count(&FAQ{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 0 ||
		count(&KnowledgeEntry{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 0
	items := []ChecklistItem{
		{Key: "organization", Label: "Business profile", Complete: organizationReady, Required: true, Group: "business", Description: "Add the business name, country, currency, and timezone used by orders and receipts."},
		{Key: "stores", Label: "Store", Complete: count(&core.Store{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 0, Required: true, Group: "business", Description: "Add at least one active location where orders can be prepared or collected."},
		{Key: "catalogue", Label: "Catalogue", Complete: count(&core.Product{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 0 && count(&core.Variant{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 0, Required: true, Group: "selling", Description: "Add an active product with a sellable option and current price."},
		{Key: "inventory", Label: "Inventory", Complete: count(&core.InventoryLevel{}, "organization_id = ? AND on_hand > reserved", orgID) > 0, Required: true, Group: "selling", Description: "Set available stock for at least one product at a store."},
		{Key: "payments", Label: "Payments", Complete: count(&core.PaymentConfiguration{}, "organization_id = ? AND enabled = ? AND status = ?", orgID, true, core.StatusActive) > 0, Required: true, Group: "selling", Description: "Enable and test a payment method before accepting paid orders."},
		{Key: "faqs", Label: "Business knowledge", Complete: knowledgeReady, Required: true, Group: "customer_service", Description: "Publish at least one active policy, FAQ, or business-information entry."},
		{Key: "bot", Label: "Assistant", Complete: count(&Bot{}, "organization_id = ? AND published_version_id IS NOT NULL AND status = ?", orgID, BotStatusActive) > 0, Required: true, Group: "customer_service", Description: "Publish the assistant after reviewing its capabilities and test conversation."},
		{Key: "team", Label: "Team", Complete: count(&organization.OrganizationMembership{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 1, Group: "operations", Description: "Optional: invite staff and assign stores so daily work reaches the right people."},
		{Key: "whatsapp", Label: "Customer channel", Complete: count(&core.Channel{}, "organization_id = ? AND status = ?", orgID, core.StatusActive) > 0, Group: "channels", Description: "Optional for this phase: external channel connections will plug into this readiness step later."},
		{Key: "database", Label: "Data service", Complete: true, Group: "system", Description: "The organization data service is available."},
		{Key: "worker", Label: "Notifications", Complete: count(&core.CommerceNotification{}, "organization_id = ? AND status IN ?", orgID, []string{"queued", "retry_pending", "failed"}) == 0, Group: "operations", Description: "No commerce notifications currently require an operator retry."},
		{Key: "outbound", Label: "Outbound messages", Complete: count(&runtimeOutboundModel{}, "organization_id = ? AND status IN ?", orgID, []string{"queued", "retry_pending", "failed"}) == 0, Group: "operations", Description: "No customer messages are currently waiting for an operator retry."},
		{Key: "support", Label: "Human support", Complete: count(&runtimeSupportHandoffModel{}, "organization_id = ? AND status IN ?", orgID, []string{"open", "assigned"}) == 0, Group: "operations", Description: "No unresolved human handoffs are currently pending."},
	}
	done, requiredDone, required := summarizeSetupStatus(items)
	return BotSetupStatus{OrganizationID: orgID, Items: items, CompleteCount: done, TotalCount: len(items), RequiredCompleteCount: requiredDone, RequiredCount: required, Ready: required > 0 && requiredDone == required}, nil
}

func summarizeSetupStatus(items []ChecklistItem) (complete, requiredComplete, required int) {
	for _, item := range items {
		if item.Complete {
			complete++
		}
		if !item.Required {
			continue
		}
		required++
		if item.Complete {
			requiredComplete++
		}
	}
	return complete, requiredComplete, required
}

type runtimeOutboundModel struct{}

func (runtimeOutboundModel) TableName() string { return "channel_outbound_messages" }

type runtimeSupportHandoffModel struct{}

func (runtimeSupportHandoffModel) TableName() string { return "support_handoffs" }

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
			if module, ok := moduleIDs[*step.ModuleID]; !ok {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step references a missing module"})
			} else if module.Source == ModuleSourceSystem && findModule(module.ModuleKey).Key == "" {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step references an unknown system module"})
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
	if _, ok := stepKeys[config.Version.StartStepKey]; ok {
		reachable := map[string]bool{}
		visiting := map[string]bool{}
		visited := map[string]bool{}
		var visit func(string)
		visit = func(stepKey string) {
			if stepKey == "" {
				return
			}
			if visiting[stepKey] {
				issues = append(issues, ValidationIssue{Path: "steps." + stepKey, Message: "Step flow contains a circular path"})
				return
			}
			if visited[stepKey] {
				return
			}
			step, ok := stepKeys[stepKey]
			if !ok {
				return
			}
			visiting[stepKey] = true
			reachable[stepKey] = true
			for _, next := range stepGraphEdges(step, moduleIDs) {
				if visiting[next] && jsonBool(step.Metadata, "allow_cycle") {
					continue
				}
				visit(next)
			}
			visiting[stepKey] = false
			visited[stepKey] = true
		}
		visit(config.Version.StartStepKey)
		for _, step := range config.Steps {
			if !reachable[step.StepKey] {
				issues = append(issues, ValidationIssue{Path: "steps." + step.StepKey, Message: "Step is not reachable from the start step"})
			}
		}
	}
	return ValidationResult{Valid: len(issues) == 0, Issues: issues}
}

func stepGraphEdges(step Step, modules map[uuid.UUID]VersionModule) []string {
	edges := []string{}
	if step.NextStepKey != "" {
		edges = append(edges, step.NextStepKey)
	}
	if step.FallbackStepKey != "" {
		edges = append(edges, step.FallbackStepKey)
	}
	if step.Type == StepModule && step.ModuleID != nil {
		module, ok := modules[*step.ModuleID]
		if ok {
			if entry := jsonString(module.Parameters, "entry_step"); entry != "" {
				edges = append(edges, entry)
			}
		}
	}
	return edges
}

func canManageBots(role authz.Role) bool {
	return role.HasPermission(authz.PermissionBotManage)
}

func canViewBots(role authz.Role) bool {
	return role.HasPermission(authz.PermissionBotView) || role.HasPermission(authz.PermissionKnowledgeView)
}

func canManageKnowledge(role authz.Role) bool {
	return role.HasPermission(authz.PermissionKnowledgeManage)
}

func canViewKnowledge(role authz.Role) bool {
	return role.HasPermission(authz.PermissionKnowledgeView)
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
	value, ok := jsonBoolValue(raw, key)
	return ok && value
}

func moduleEnabled(raw string) bool {
	value, ok := jsonBoolValue(raw, "enabled")
	if !ok {
		return true
	}
	return value
}

func jsonBoolValue(raw, key string) (bool, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonObject(raw)), &obj); err != nil {
		return false, false
	}
	value, ok := obj[key]
	if !ok {
		return false, false
	}
	asBool, ok := value.(bool)
	return asBool, ok
}

func jsonWithBool(raw, key string, value bool) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonObject(raw)), &obj); err != nil {
		obj = map[string]any{}
	}
	obj[key] = value
	return jsonValue(obj)
}

func jsonString(raw, key string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonObject(raw)), &obj); err != nil {
		return ""
	}
	value, ok := obj[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
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

func idPtr(value uuid.UUID) *uuid.UUID {
	return &value
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func conditionJSON(field, operator string, value any) string {
	return jsonValue([]map[string]any{{"field": field, "operator": operator, "value": value}})
}

func jsonValue(value any) string {
	if value == nil {
		return "{}"
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func cleanKeywords(values []string) []string {
	keywords := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		keyword := normalizeSearchText(value)
		if keyword == "" {
			continue
		}
		if _, ok := seen[keyword]; ok {
			continue
		}
		seen[keyword] = struct{}{}
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	return keywords
}

func knowledgeEntryFromInput(organizationID uuid.UUID, input KnowledgeEntryInput, requireAnswer bool) (KnowledgeEntry, error) {
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind == "" {
		kind = KnowledgeKindFAQ
	}
	if !validKnowledgeKind(kind) {
		return KnowledgeEntry{}, httperror.BadRequest("Knowledge kind is not valid")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = core.StatusActive
	}
	if !validKnowledgeStatus(status) {
		return KnowledgeEntry{}, httperror.BadRequest("Knowledge status is not valid")
	}
	category := normalizeKnowledgeCategory(input.Category)
	title := strings.TrimSpace(input.Title)
	question := strings.TrimSpace(input.Question)
	answer := strings.TrimSpace(input.Answer)
	if title == "" {
		title = question
	}
	if kind == KnowledgeKindFAQ && question == "" {
		question = title
	}
	if title == "" {
		return KnowledgeEntry{}, httperror.BadRequest("Knowledge title is required")
	}
	if requireAnswer && answer == "" {
		return KnowledgeEntry{}, httperror.BadRequest("Knowledge answer is required")
	}
	sourceType := strings.ToLower(strings.TrimSpace(input.SourceType))
	if sourceType == "" {
		sourceType = "manual"
	}
	return KnowledgeEntry{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Kind:            kind,
		Category:        category,
		Title:           title,
		Question:        question,
		Answer:          answer,
		Keywords:        jsonValue(cleanKeywords(input.Keywords)),
		SourceType:      sourceType,
		Status:          status,
		Metadata:        jsonObject(input.Metadata),
		EmbeddingStatus: KnowledgeEmbeddingStatusDisabled,
	}, nil
}

func knowledgeEntryUpdates(input KnowledgeEntryInput) (map[string]any, error) {
	updates := map[string]any{}
	if strings.TrimSpace(input.Kind) != "" {
		kind := strings.ToLower(strings.TrimSpace(input.Kind))
		if !validKnowledgeKind(kind) {
			return nil, httperror.BadRequest("Knowledge kind is not valid")
		}
		updates["kind"] = kind
	}
	if strings.TrimSpace(input.Category) != "" {
		updates["category"] = normalizeKnowledgeCategory(input.Category)
	}
	if strings.TrimSpace(input.Title) != "" {
		updates["title"] = strings.TrimSpace(input.Title)
	}
	if strings.TrimSpace(input.Question) != "" {
		updates["question"] = strings.TrimSpace(input.Question)
	}
	if strings.TrimSpace(input.Answer) != "" {
		updates["answer"] = strings.TrimSpace(input.Answer)
	}
	if input.Keywords != nil {
		updates["keywords"] = jsonValue(cleanKeywords(input.Keywords))
	}
	if strings.TrimSpace(input.SourceType) != "" {
		updates["source_type"] = strings.ToLower(strings.TrimSpace(input.SourceType))
	}
	if strings.TrimSpace(input.Status) != "" {
		status := strings.ToLower(strings.TrimSpace(input.Status))
		if !validKnowledgeStatus(status) {
			return nil, httperror.BadRequest("Knowledge status is not valid")
		}
		updates["status"] = status
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	return updates, nil
}

func normalizeKnowledgeCategory(value string) string {
	category := normalizeSearchText(value)
	if category == "" {
		return "general"
	}
	return strings.ReplaceAll(category, " ", "_")
}

func validKnowledgeKind(kind string) bool {
	switch kind {
	case KnowledgeKindFAQ, KnowledgeKindPolicy, KnowledgeKindBusinessInfo, KnowledgeKindDelivery, KnowledgeKindReturns, KnowledgeKindWarranty, KnowledgeKindLocation, KnowledgeKindPaymentInfo:
		return true
	default:
		return false
	}
}

func validKnowledgeStatus(status string) bool {
	return status == core.StatusActive || status == "draft" || status == "archived"
}

func knowledgeAuditMetadata(entry KnowledgeEntry) string {
	return jsonValue(map[string]any{"kind": entry.Kind, "category": entry.Category, "status": entry.Status})
}

func (s *Service) applyKnowledgeEmbeddingLifecycle(entry *KnowledgeEntry) {
	entry.EmbeddingContentHash = KnowledgeEmbeddingHash(*entry)
	entry.EmbeddingError = ""
	entry.EmbeddedAt = nil
	entry.Embedding = nil
	if s.embeddingsEnabled && entry.Status == core.StatusActive {
		entry.EmbeddingStatus = KnowledgeEmbeddingStatusPending
		return
	}
	entry.EmbeddingStatus = KnowledgeEmbeddingStatusDisabled
}

func applyKnowledgeEmbeddingUpdates(enabled bool, existing KnowledgeEntry, updates map[string]any) {
	status := existing.Status
	if raw, ok := updates["status"].(string); ok {
		status = raw
	}
	embeddingRelevant := false
	for _, key := range []string{"kind", "category", "title", "question", "answer", "keywords", "status"} {
		if _, ok := updates[key]; ok {
			embeddingRelevant = true
			break
		}
	}
	if !embeddingRelevant {
		return
	}
	if enabled && status == core.StatusActive {
		updated := existing
		applyKnowledgeEntryMap(&updated, updates)
		updates["embedding_status"] = KnowledgeEmbeddingStatusPending
		updates["embedding_content_hash"] = KnowledgeEmbeddingHash(updated)
		updates["embedding_error"] = ""
		updates["embedded_at"] = nil
		updates["embedding"] = nil
		return
	}
	updates["embedding_status"] = KnowledgeEmbeddingStatusDisabled
	updates["embedding_error"] = ""
	updates["embedded_at"] = nil
	updates["embedding"] = nil
}

func applyKnowledgeEntryMap(entry *KnowledgeEntry, updates map[string]any) {
	if value, ok := updates["kind"].(string); ok {
		entry.Kind = value
	}
	if value, ok := updates["category"].(string); ok {
		entry.Category = value
	}
	if value, ok := updates["title"].(string); ok {
		entry.Title = value
	}
	if value, ok := updates["question"].(string); ok {
		entry.Question = value
	}
	if value, ok := updates["answer"].(string); ok {
		entry.Answer = value
	}
	if value, ok := updates["keywords"].(string); ok {
		entry.Keywords = value
	}
	if value, ok := updates["source_type"].(string); ok {
		entry.SourceType = value
	}
	if value, ok := updates["status"].(string); ok {
		entry.Status = value
	}
	if value, ok := updates["metadata"].(string); ok {
		entry.Metadata = value
	}
	if value, ok := updates["embedding_status"].(string); ok {
		entry.EmbeddingStatus = value
	}
	if value, ok := updates["embedding_content_hash"].(string); ok {
		entry.EmbeddingContentHash = value
	}
}

func (s *Service) enqueueKnowledgeEmbeddingTx(ctx context.Context, tx *gorm.DB, organizationID uuid.UUID, entry KnowledgeEntry) error {
	if !s.embeddingsEnabled || s.jobs == nil || entry.Status != core.StatusActive || entry.EmbeddingStatus != KnowledgeEmbeddingStatusPending {
		return nil
	}
	hash := firstNonEmptyString(entry.EmbeddingContentHash, KnowledgeEmbeddingHash(entry))
	var existing jobs.Job
	err := tx.WithContext(ctx).Where("job_type = ? AND idempotency_key = ?", jobs.JobTypeKnowledgeEmbedding, "knowledge-embedding:"+entry.ID.String()+":"+hash).First(&existing).Error
	if err == nil {
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	now := s.now()
	job := jobs.Job{
		ID:             uuid.New(),
		OrganizationID: &organizationID,
		JobType:        jobs.JobTypeKnowledgeEmbedding,
		Status:         jobs.StatusQueued,
		Payload:        jsonValue(map[string]any{"knowledge_entry_id": entry.ID.String()}),
		IdempotencyKey: "knowledge-embedding:" + entry.ID.String() + ":" + hash,
		CorrelationID:  entry.ID.String(),
		MaxAttempts:    3,
		AvailableAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return tx.WithContext(ctx).Create(&job).Error
}

func KnowledgeEmbeddingSourceText(entry KnowledgeEntry) string {
	parts := []string{entry.Kind, entry.Category, entry.Title, entry.Question, entry.Answer}
	for _, keyword := range parseKeywords(entry.Keywords) {
		parts = append(parts, keyword)
	}
	return strings.Join(compactStrings(parts), "\n")
}

func KnowledgeEmbeddingHash(entry KnowledgeEntry) string {
	sum := sha256.Sum256([]byte(KnowledgeEmbeddingSourceText(entry)))
	return hex.EncodeToString(sum[:])
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "updated_at" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func scoreFAQ(query string, faq FAQ) (float64, string) {
	question := normalizeSearchText(faq.Question)
	answer := normalizeSearchText(faq.Answer)
	keywords := parseKeywords(faq.Keywords)
	if question == query {
		return 1, "question"
	}
	if strings.Contains(question, query) || strings.Contains(query, question) {
		return 0.86, "question"
	}
	queryTokens := tokenSet(query)
	questionScore := overlapScore(queryTokens, tokenSet(question))
	answerScore := overlapScore(queryTokens, tokenSet(answer)) * 0.35
	keywordScore := 0.0
	for _, keyword := range keywords {
		if keyword == query || strings.Contains(query, keyword) || strings.Contains(keyword, query) {
			keywordScore = 0.92
			break
		}
		keywordScore = maxFloat(keywordScore, overlapScore(queryTokens, tokenSet(keyword))*0.8)
	}
	score := maxFloat(questionScore, maxFloat(answerScore, keywordScore))
	matchedOn := "question"
	if score == keywordScore && keywordScore > 0 {
		matchedOn = "keyword"
	} else if score == answerScore && answerScore > 0 {
		matchedOn = "answer"
	}
	return score, matchedOn
}

func parseKeywords(raw string) []string {
	var keywords []string
	_ = json.Unmarshal([]byte(jsonArray(raw)), &keywords)
	return cleanKeywords(keywords)
}

func tokenSet(value string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, part := range strings.Fields(normalizeSearchText(value)) {
		if len(part) < 3 {
			continue
		}
		tokens[part] = struct{}{}
	}
	return tokens
}

func overlapScore(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	hits := 0
	for token := range a {
		if _, ok := b[token]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(a))
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(".", " ", ",", " ", "?", " ", "!", " ", "-", " ", "_", " ", ":", " ", ";", " ", "\n", " ")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func faqConfidence(score float64) string {
	switch {
	case score >= 0.8:
		return "high"
	case score >= 0.5:
		return "medium"
	default:
		return "low"
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
