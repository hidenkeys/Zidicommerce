package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// JourneyDefinition is the merchant DTO. Its only persisted definition is the
// existing version/question/step/action/condition graph, including snapshots.
type JourneyDefinition struct {
	Name         string        `json:"name"`
	Purpose      string        `json:"purpose"`
	Category     string        `json:"category"`
	Mode         string        `json:"mode"`
	Channel      string        `json:"channel"`
	ConnectionID *uuid.UUID    `json:"connection_id,omitempty"`
	Trigger      string        `json:"trigger"`
	Start        string        `json:"start"`
	TTLMinutes   int           `json:"ttl_minutes"`
	Steps        []JourneyStep `json:"steps"`
}

type JourneyOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
	Next  string `json:"next,omitempty"`
}

type JourneyStep struct {
	Key               string          `json:"key"`
	Kind              string          `json:"kind"`
	Title             string          `json:"title"`
	Text              string          `json:"text"`
	Help              string          `json:"help"`
	Required          bool            `json:"required"`
	Locked            bool            `json:"locked"`
	AnswerType        string          `json:"answer_type"`
	Presentation      string          `json:"presentation"`
	Destination       string          `json:"destination,omitempty"`
	Source            string          `json:"source"`
	Options           []JourneyOption `json:"options"`
	Min               *float64        `json:"min,omitempty"`
	Max               *float64        `json:"max,omitempty"`
	MaxLength         int             `json:"max_length"`
	RetryMessage      string          `json:"retry_message"`
	MaxRetries        int             `json:"max_retries"`
	Next              string          `json:"next"`
	Failure           string          `json:"failure"`
	ConditionField    string          `json:"condition_field,omitempty"`
	ConditionOperator string          `json:"condition_operator,omitempty"`
	ConditionValue    string          `json:"condition_value,omitempty"`
	VisibleWhen       string          `json:"visible_when,omitempty"`
	AnalyticsLabel    string          `json:"analytics_label"`
}

type JourneyDetail struct {
	Bot        Bot                    `json:"journey"`
	Version    BotVersion             `json:"version"`
	Definition JourneyDefinition      `json:"definition"`
	Versions   []BotVersion           `json:"versions"`
	Binding    *JourneyBinding        `json:"binding,omitempty"`
	Deployment *JourneyFlowDeployment `json:"deployment,omitempty"`
}

type JourneyBinding struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;uniqueIndex:idx_journey_route" json:"organization_id"`
	BotID          uuid.UUID  `gorm:"type:uuid;uniqueIndex" json:"bot_id"`
	VersionID      uuid.UUID  `gorm:"type:uuid" json:"version_id"`
	ConnectionID   *uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journey_route" json:"connection_id,omitempty"`
	Mode           string     `json:"mode"`
	Trigger        string     `gorm:"uniqueIndex:idx_journey_route" json:"trigger"`
	Paused         bool       `json:"paused"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (JourneyBinding) TableName() string { return "journey_bindings" }

type JourneyFlowDeployment struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journey_flow_revision" json:"organization_id"`
	BotID          uuid.UUID `gorm:"type:uuid" json:"bot_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journey_flow_revision" json:"version_id"`
	ConnectionID   uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journey_flow_revision" json:"connection_id"`
	ProviderFlowID string    `json:"provider_flow_id"`
	Status         string    `json:"status"`
	Revision       int       `json:"revision"`
	Errors         string    `json:"errors"`
	Warnings       string    `json:"warnings"`
	PreviewURL     string    `json:"preview_url"`
	CompiledHash   string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (JourneyFlowDeployment) TableName() string { return "journey_flow_deployments" }

var journeyKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)
var journeyMarkup = regexp.MustCompile(`(?i)<[^>]+>|javascript:|data:text/html`)

func JourneySettings(version BotVersion) JourneyDefinition {
	var metadata struct {
		Journey JourneyDefinition `json:"journey"`
	}
	_ = json.Unmarshal([]byte(version.Metadata), &metadata)
	return metadata.Journey
}
func IsJourney(version BotVersion) bool { return JourneySettings(version).Category != "" }
func JourneyStepSettings(step Step) JourneyStep {
	var metadata struct {
		Journey JourneyStep `json:"journey"`
	}
	_ = json.Unmarshal([]byte(step.Metadata), &metadata)
	return metadata.Journey
}

func (s *Service) ListJourneys(ctx context.Context, actor auth.CurrentUser) ([]Bot, error) {
	rows, err := s.ListBots(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := []Bot{}
	for _, row := range rows {
		var m map[string]any
		_ = json.Unmarshal([]byte(row.Metadata), &m)
		if m["journey"] == true {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *Service) CreateJourney(ctx context.Context, actor auth.CurrentUser, template, name string) (JourneyDetail, error) {
	if actor.OrganizationID == uuid.Nil || !canManageBots(actor.Role) {
		return JourneyDetail{}, httperror.Forbidden("You cannot create journeys")
	}
	definition, ok := JourneyTemplate(template)
	if !ok {
		return JourneyDetail{}, httperror.BadRequest("Choose a valid journey template")
	}
	if strings.TrimSpace(name) != "" {
		definition.Name = strings.TrimSpace(name)
	}
	var record Bot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		service := NewService(tx)
		var err error
		record, err = service.CreateBot(ctx, actor, BotInput{Name: definition.Name, Description: definition.Purpose, Metadata: `{"journey":true}`, FallbackConfig: `{"message":"Please try again or contact support.","missing_variable":"not available"}`})
		if err != nil {
			return err
		}
		var version BotVersion
		if err := tx.Where("organization_id = ? AND bot_id = ?", actor.OrganizationID, record.ID).First(&version).Error; err != nil {
			return err
		}
		_, err = service.SaveJourney(ctx, actor, version.ID, definition)
		return err
	})
	if err != nil {
		return JourneyDetail{}, err
	}
	return s.GetJourney(ctx, actor, record.ID, nil)
}

func (s *Service) GetJourney(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID, versionID *uuid.UUID) (JourneyDetail, error) {
	record, err := s.GetBot(ctx, actor, botID)
	if err != nil {
		return JourneyDetail{}, err
	}
	versions, err := s.ListVersions(ctx, actor, botID)
	if err != nil || len(versions) == 0 {
		return JourneyDetail{}, httperror.NotFound("Journey version not found")
	}
	version := versions[0]
	if versionID != nil {
		version, err = s.GetVersion(ctx, actor, *versionID)
		if err != nil {
			return JourneyDetail{}, err
		}
		if version.BotID != botID {
			return JourneyDetail{}, httperror.NotFound("Journey version not found")
		}
	}
	if !IsJourney(version) {
		return JourneyDetail{}, httperror.NotFound("Journey not found")
	}
	config, err := s.loadConfiguration(ctx, actor.OrganizationID, version)
	if err != nil {
		return JourneyDetail{}, err
	}
	detail := JourneyDetail{Bot: record, Version: version, Definition: JourneyFromConfiguration(config), Versions: versions}
	var binding JourneyBinding
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND bot_id = ?", actor.OrganizationID, botID).First(&binding).Error; err == nil {
		detail.Binding = &binding
	} else if err != gorm.ErrRecordNotFound {
		return detail, err
	}
	var deployment JourneyFlowDeployment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ?", actor.OrganizationID, version.ID).Order("updated_at DESC").First(&deployment).Error; err == nil {
		detail.Deployment = &deployment
	} else if err != gorm.ErrRecordNotFound {
		return detail, err
	}
	return detail, nil
}

func JourneyFromConfiguration(config VersionConfiguration) JourneyDefinition {
	d := JourneySettings(config.Version)
	d.Start = config.Version.StartStepKey
	d.Steps = []JourneyStep{}
	for _, step := range config.Steps {
		j := JourneyStepSettings(step)
		j.Key, j.Title, j.Next, j.Failure = step.StepKey, step.Title, step.NextStepKey, step.FallbackStepKey
		j.Text = step.Message
		for _, q := range config.Questions {
			if step.QuestionID != nil && q.ID == *step.QuestionID {
				j.Text, j.Help, j.Required = q.Text, q.HelpText, q.Required
				j.AnswerType, j.Presentation, j.Destination = q.Type, q.ResponseMode, q.VariableName
				_ = json.Unmarshal([]byte(q.Options), &j.Options)
			}
		}
		d.Steps = append(d.Steps, j)
	}
	return d
}

func (s *Service) SaveJourney(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, d JourneyDefinition) (JourneyDetail, error) {
	if !canManageBots(actor.Role) {
		return JourneyDetail{}, httperror.Forbidden("You cannot edit journeys")
	}
	if err := validateJourneyInput(d); err != nil {
		return JourneyDetail{}, err
	}
	var version BotVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, versionID).First(&version).Error; err != nil {
			return err
		}
		if version.Status != VersionStatusDraft && version.Status != VersionStatusValidated {
			return httperror.BadRequest("Published journeys are immutable; create a new draft")
		}
		var immutable int64
		if err := tx.Model(&JourneyFlowDeployment{}).Where("organization_id = ? AND version_id = ? AND status IN ?", actor.OrganizationID, versionID, []string{"published", "deprecated"}).Count(&immutable).Error; err != nil {
			return err
		}
		if immutable > 0 {
			return httperror.BadRequest("This version has an immutable provider Flow; clone a new draft")
		}
		if !IsJourney(version) && version.Metadata != "{}" {
			return httperror.BadRequest("This version is not a Journey Studio draft")
		}
		if d.ConnectionID != nil {
			var c core.Channel
			if err := tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, *d.ConnectionID).First(&c).Error; err != nil {
				return httperror.BadRequest("Channel connection does not belong to this organization")
			}
			if d.Channel != "" && c.Provider != d.Channel {
				return httperror.BadRequest("Channel and connection do not match")
			}
		}
		// Replacing a draft is atomic. A failed edit never leaves a partial graph.
		for _, model := range []any{&Step{}, &Question{}, &Action{}, &Condition{}, &Variable{}, &VersionModule{}, &Integration{}} {
			if err := tx.Where("organization_id = ? AND version_id = ?", actor.OrganizationID, versionID).Delete(model).Error; err != nil {
				return err
			}
		}
		settings := d
		settings.Steps = nil
		if err := tx.Model(&version).Updates(map[string]any{"start_step_key": d.Start, "status": VersionStatusDraft, "validated_at": nil, "validation_errors": "[]", "metadata": jsonValue(map[string]any{"journey": settings})}).Error; err != nil {
			return err
		}
		for index, j := range d.Steps {
			step := Step{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, StepKey: j.Key, Title: j.Title, Message: j.Text, NextStepKey: j.Next, FallbackStepKey: j.Failure, SortOrder: index * 10, ResponseMode: j.Presentation, Options: "[]", Metadata: jsonValue(map[string]any{"journey": j})}
			switch j.Kind {
			case "message":
				step.Type = StepMessage
			case "end":
				step.Type = StepEnd
			case "handoff":
				step.Type = StepHandoff
			case "condition":
				step.Type = StepCondition
				c := Condition{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, ConditionKey: j.Key, Name: j.Title, Combinator: "and", Rules: conditionJSON("variables."+j.ConditionField, defaultString(j.ConditionOperator, "equals"), j.ConditionValue), Metadata: "{}"}
				if err := tx.Create(&c).Error; err != nil {
					return err
				}
				step.ConditionID = &c.ID
			case "question", "buttons", "list", "entry_menu", "store_selector", "category_selector", "product_selector", "variant_selector", "fulfilment_selector", "quantity", "address", "review_cart":
				step.Type = StepQuestion
				if j.Kind == "review_cart" {
					step.Type = StepAction
					a := journeyAction(actor.OrganizationID, versionID, j)
					if err := tx.Create(&a).Error; err != nil {
						return err
					}
					step.ActionID = &a.ID
					break
				}
				answerType := defaultString(j.AnswerType, "text")
				if j.Source != "" || j.Kind == "buttons" || j.Kind == "list" {
					answerType = "single_choice"
				}
				if j.Kind == "quantity" {
					answerType = "number"
				}
				q := Question{ID: uuid.New(), OrganizationID: actor.OrganizationID, VersionID: versionID, QuestionKey: j.Key, Text: j.Text, HelpText: j.Help, Type: answerType, ResponseMode: defaultString(j.Presentation, "free_text"), Required: j.Required, VariableName: defaultString(j.Destination, "answer_"+j.Key), Options: jsonValue(j.Options), Validation: jsonValue(map[string]any{"min": j.Min, "max": j.Max, "max_length": j.MaxLength}), Metadata: step.Metadata}
				if err := tx.Create(&q).Error; err != nil {
					return err
				}
				step.QuestionID = &q.ID
			case "check_inventory", "add_to_cart", "create_order", "payment", "payment_status", "notification", "complaint_action", "track_action":
				step.Type = StepAction
				a := journeyAction(actor.OrganizationID, versionID, j)
				if err := tx.Create(&a).Error; err != nil {
					return err
				}
				step.ActionID = &a.ID
			default:
				return httperror.BadRequest("Unsupported journey step type")
			}
			if err := tx.Create(&step).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Bot{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, version.BotID).Updates(map[string]any{"name": d.Name, "description": d.Purpose}).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot_version", versionID, "journey_draft_saved", "{}")
	})
	if err != nil {
		return JourneyDetail{}, err
	}
	return s.GetJourney(ctx, actor, version.BotID, &versionID)
}

func validateJourneyInput(d JourneyDefinition) error {
	if strings.TrimSpace(d.Name) == "" || len(d.Name) > 120 || len(d.Purpose) > 1000 || len(d.Steps) == 0 || len(d.Steps) > 80 {
		return httperror.BadRequest("Journey name and 1 to 80 steps are required")
	}
	if !containsString([]string{"order", "track_order", "complaint", "catalogue", "support", "custom"}, d.Category) || !containsString([]string{"guided_chat", "whatsapp_flow", "ai"}, d.Mode) {
		return httperror.BadRequest("Journey category or presentation is invalid")
	}
	if d.TTLMinutes < 5 || d.TTLMinutes > 10080 {
		return httperror.BadRequest("Session expiry must be between 5 minutes and 7 days")
	}
	if !containsString([]string{"order", "track_order", "complaint", "catalogue", "support", "custom", "default"}, d.Trigger) {
		return httperror.BadRequest("Choose a supported entry intent")
	}
	keys := map[string]bool{}
	for _, j := range d.Steps {
		if !journeyKey.MatchString(j.Key) || keys[j.Key] {
			return httperror.BadRequest("Steps need unique stable identifiers")
		}
		keys[j.Key] = true
		if len(j.Title) > 80 || len(j.Text) > 4096 || len(j.Help) > 1000 || journeyMarkup.MatchString(j.Text+j.Title+j.Help+j.RetryMessage+d.Name+d.Purpose) {
			return httperror.BadRequest("Use plain text for journey messages (maximum 4096 characters)")
		}
		if len(j.Options) > 10 || j.MaxRetries < 0 || j.MaxRetries > 10 || j.MaxLength < 0 || j.MaxLength > 4096 {
			return httperror.BadRequest("Question limits are invalid")
		}
		if j.Destination != "" && (!journeyKey.MatchString(j.Destination) || (containsString([]string{"payment_status", "order_id", "cart_id", "payment_reference", "payment_url", "order_total_minor", "cart_total_minor", "customer_id", "total_minor", "payment_id", "order_number", "amount_minor", "currency", "support_ticket_id", "complaint_id"}, j.Destination) && !(j.Destination == "order_id" && j.Source == "orders" && d.Category == "track_order"))) {
			return httperror.BadRequest("This answer destination is reserved for server facts")
		}
		if j.Min != nil && j.Max != nil && *j.Min > *j.Max {
			return httperror.BadRequest("Question minimum exceeds its maximum")
		}
		protected := map[string][2]string{"store_selector": {"stores", "store_id"}, "category_selector": {"categories", "category_id"}, "product_selector": {"products", "product_id"}, "variant_selector": {"variants", "variant_id"}, "fulfilment_selector": {"fulfilment", "fulfilment_mode"}, "quantity": {"", "quantity"}, "address": {"", "delivery_address"}}
		if fields, ok := protected[j.Kind]; ok && (j.Source != fields[0] || j.Destination != fields[1]) {
			return httperror.BadRequest("Commerce selectors use protected server data bindings")
		}
		if j.Kind == "entry_menu" && (j.Source != "journeys" || d.Trigger != "default") {
			return httperror.BadRequest("Entry menus use current published journeys and the default entry intent")
		}
		if j.Source != "" && !containsString([]string{"stores", "categories", "products", "variants", "fulfilment", "orders", "complaint_categories", "journeys"}, j.Source) && !(j.Kind == "notification" && j.Source == "store") {
			return httperror.BadRequest("Question data source is invalid")
		}
		if j.AnswerType != "" && !containsString([]string{"text", "single_choice", "yes_no", "number", "email", "phone", "date", "location", "media"}, j.AnswerType) {
			return httperror.BadRequest("Question answer type is invalid")
		}
		if j.ConditionField != "" && !journeyKey.MatchString(j.ConditionField) {
			return httperror.BadRequest("Condition field is invalid")
		}
		if j.ConditionOperator != "" && !containsString([]string{"equals", "not_equals", "contains"}, j.ConditionOperator) {
			return httperror.BadRequest("Condition operator is invalid")
		}
		if j.Kind == "condition" && containsString([]string{"confirmation", "payment_status"}, j.ConditionField) && j.ConditionOperator != "" && j.ConditionOperator != "equals" {
			return httperror.BadRequest("Confirmation and payment proof require an exact comparison")
		}
		ids := map[string]bool{}
		for _, o := range j.Options {
			if !journeyKey.MatchString(o.ID) || ids[o.ID] || strings.TrimSpace(o.Label) == "" || len(o.Label) > 72 || len(o.Value) > 120 || journeyMarkup.MatchString(o.Label+o.Value) {
				return httperror.BadRequest("Options need unique identifiers and plain text labels")
			}
			ids[o.ID] = true
		}
	}
	return nil
}

func journeyAction(org, version uuid.UUID, j JourneyStep) Action {
	types := map[string]string{"check_inventory": "check_inventory", "add_to_cart": "add_to_cart", "create_order": "create_order", "payment": "initialize_payment", "payment_status": "check_payment", "review_cart": "calculate_cart", "complaint_action": "create_complaint", "track_action": "get_order_status", "notification": "notify_customer"}
	actionType := types[j.Kind]
	if j.Kind == "notification" && j.Source == "store" {
		actionType = "notify_store"
	}
	inputs := map[string]any{"store_id": "variables.store_id", "variant_id": "variables.variant_id", "quantity": "variables.quantity", "cart_id": "variables.cart_id", "customer_id": "session.customer_id", "order_id": "variables.order_id", "payment_reference": "variables.payment_reference", "fulfilment_type": "variables.fulfilment_mode", "delivery_address": "variables.delivery_address", "recipient_name": "variables.customer_name", "recipient_phone": "variables.customer_phone", "email": "variables.customer_email", "description": "variables.complaint_message", "message": "variables.final_message", "provider": "paystack"}
	outputs := map[string]any{}
	for _, key := range []string{"cart_id", "order_id", "order_number", "payment_reference", "payment_url", "payment_status", "store_id", "total_minor"} {
		outputs[key] = "variables." + key
	}
	return Action{ID: uuid.New(), OrganizationID: org, VersionID: version, ActionKey: j.Key, ActionType: actionType, Name: j.Title, InputMappings: jsonValue(inputs), OutputMappings: jsonValue(outputs), Metadata: "{}"}
}

func sameJourneyJSON(left, right string) bool {
	var a, b any
	if json.Unmarshal([]byte(left), &a) != nil || json.Unmarshal([]byte(right), &b) != nil {
		return false
	}
	return jsonValue(a) == jsonValue(b)
}

func ValidateJourney(d JourneyDefinition) ValidationResult {
	issues := []ValidationIssue{}
	add := func(key, message string) {
		issues = append(issues, ValidationIssue{Path: "steps." + key, Message: message})
	}
	if err := validateJourneyInput(d); err != nil {
		add("", err.Error())
	}
	steps := map[string]JourneyStep{}
	for _, j := range d.Steps {
		steps[j.Key] = j
	}
	if _, ok := steps[d.Start]; !ok {
		add(d.Start, "Choose an existing start step")
	}
	for _, j := range d.Steps {
		for _, edge := range journeyEdges(j) {
			if _, ok := steps[edge]; !ok {
				add(j.Key, "Route points to a missing step: "+edge)
			}
		}
		if j.Kind == "condition" && (j.Next == "" || j.Failure == "" || j.ConditionField == "") {
			add(j.Key, "Conditions need a field and both routes")
		}
		if (j.Kind == "buttons" || j.Kind == "list" || j.AnswerType == "single_choice") && j.Source == "" && len(j.Options) == 0 {
			add(j.Key, "Add selectable options or a data source")
		}
		if j.Kind != "end" && j.Kind != "handoff" && j.Next == "" && len(j.Options) == 0 {
			add(j.Key, "Choose a next step")
		}
		if (j.Source != "" || j.Kind == "payment" || j.Kind == "create_order" || j.Kind == "check_inventory" || j.Kind == "add_to_cart" || j.Kind == "payment_status") && j.Failure == "" {
			add(j.Key, "Add a failure or human handoff route")
		}
		if j.VisibleWhen != "" {
			if j.Locked || containsString([]string{"check_inventory", "add_to_cart", "review_cart", "create_order", "payment", "payment_status"}, j.Kind) {
				add(j.Key, "Protected commerce operations cannot be conditionally hidden")
			}
			if !journeyKey.MatchString(j.VisibleWhen) {
				add(j.Key, "Visibility must refer to a collected answer")
			}
		}
	}
	reachable := map[string]bool{}
	var walk func(string)
	walk = func(key string) {
		if reachable[key] {
			return
		}
		j, ok := steps[key]
		if !ok {
			return
		}
		reachable[key] = true
		for _, e := range journeyEdges(j) {
			walk(e)
		}
	}
	walk(d.Start)
	for _, j := range d.Steps {
		if !reachable[j.Key] {
			add(j.Key, "Step is unreachable")
		}
	}
	// Evaluate each path's safety state, including loops. A merchant cannot route
	// around stock checks, review/confirmation, order creation or provider proof.
	type state struct {
		key                                                    string
		checked, added, reviewed, confirmed, ordered, verified bool
	}
	queue := []state{{key: d.Start}}
	seen := map[state]bool{}
	for len(queue) > 0 && len(seen) < 10000 {
		x := queue[0]
		queue = queue[1:]
		if seen[x] {
			continue
		}
		seen[x] = true
		j, ok := steps[x.key]
		if !ok {
			continue
		}
		switch j.Kind {
		case "quantity":
			x.checked = false
			x.reviewed = false
			x.confirmed = false
		case "check_inventory":
			x.checked = true
		case "add_to_cart":
			if !x.checked {
				add(j.Key, "Inventory verification must run before adding an item")
			}
			x.added = true
			x.reviewed = false
			x.confirmed = false
		case "review_cart":
			if !x.added {
				add(j.Key, "Review requires a cart item")
			}
			x.reviewed = true
		case "create_order":
			if !x.checked || !x.added || !x.reviewed || !x.confirmed {
				add(j.Key, "Order creation requires stock verification, cart review and explicit confirmation")
			}
			x.ordered = true
		case "payment":
			if !x.ordered {
				add(j.Key, "Payment must follow server-side order creation")
			}
		case "notification":
			if d.Category == "order" && (!x.ordered || !x.verified) {
				add(j.Key, "Order confirmation requires verified payment")
			}
		}
		for _, edge := range journeyEdges(j) {
			n := x
			n.key = edge
			if j.Kind == "condition" && j.ConditionField == "confirmation" {
				n.confirmed = edge == j.Next && j.ConditionValue == "confirm"
			}
			if j.Kind == "condition" && j.ConditionField == "payment_status" {
				n.verified = edge == j.Next && j.ConditionValue == core.PaymentPaid
			}
			queue = append(queue, n)
		}
	}
	// Automatic cycles have no customer escape and would spin the interpreter.
	visiting, done := map[string]bool{}, map[string]bool{}
	var cycle func(string)
	cycle = func(key string) {
		j, ok := steps[key]
		if !ok || done[key] {
			return
		}
		if visiting[key] {
			add(key, "Automatic loop has no customer escape")
			return
		}
		if isJourneyQuestion(j) {
			return
		}
		visiting[key] = true
		for _, e := range journeyEdges(j) {
			cycle(e)
		}
		visiting[key] = false
		done[key] = true
	}
	for key := range steps {
		cycle(key)
	}
	if d.Category == "order" {
		for _, kind := range []string{"store_selector", "product_selector", "variant_selector", "quantity", "check_inventory", "add_to_cart", "fulfilment_selector", "review_cart", "create_order", "payment", "payment_status", "notification"} {
			found := false
			for _, j := range d.Steps {
				if j.Kind == kind && reachable[j.Key] {
					found = true
				}
			}
			if !found {
				add("", "Ordering requires the protected "+strings.ReplaceAll(kind, "_", " ")+" step")
			}
		}
	}
	unique := []ValidationIssue{}
	seenIssue := map[string]bool{}
	for _, i := range issues {
		key := i.Path + i.Message
		if !seenIssue[key] {
			unique = append(unique, i)
			seenIssue[key] = true
		}
	}
	return ValidationResult{Valid: len(unique) == 0, Issues: unique}
}
func journeyEdges(j JourneyStep) []string {
	edges := []string{}
	for _, e := range []string{j.Next, j.Failure} {
		if e != "" {
			edges = append(edges, e)
		}
	}
	for _, o := range j.Options {
		if o.Next != "" {
			edges = append(edges, o.Next)
		}
	}
	return edges
}
func isJourneyQuestion(j JourneyStep) bool {
	return containsString([]string{"question", "buttons", "list", "entry_menu", "store_selector", "category_selector", "product_selector", "variant_selector", "fulfilment_selector", "quantity", "address"}, j.Kind)
}
func containsString(values []string, value string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}

func (s *Service) ValidateJourneyVersion(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (ValidationResult, error) {
	config, err := s.GetConfiguration(ctx, actor, versionID)
	if err != nil {
		return ValidationResult{}, err
	}
	if !IsJourney(config.Version) {
		return ValidationResult{}, httperror.NotFound("Journey version not found")
	}
	result := validateConfiguration(config)
	if JourneySettings(config.Version).Mode == "whatsapp_flow" {
		compiled := CompileJourneyFlow(JourneyFromConfiguration(config))
		result.Issues = append(result.Issues, compiled.Errors...)
		result.Valid = len(result.Issues) == 0
	}
	if canManageBots(actor.Role) && config.Version.Status == VersionStatusDraft {
		updates := map[string]any{"validation_errors": jsonValue(result.Issues)}
		if result.Valid {
			updates["status"] = VersionStatusValidated
			updates["validated_at"] = s.now()
		}
		if err := s.db.WithContext(ctx).Model(&BotVersion{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, versionID).Updates(updates).Error; err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Service) PublishJourney(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID) (JourneyDetail, error) {
	if !actor.Role.HasPermission(authz.PermissionBotPublish) {
		return JourneyDetail{}, httperror.Forbidden("You cannot publish journeys")
	}
	var botID uuid.UUID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		service := NewService(tx)
		var v BotVersion
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, versionID).First(&v).Error; err != nil {
			return mapNotFound(err, "Journey version not found")
		}
		botID = v.BotID
		if v.Status == VersionStatusPublished || v.Status == VersionStatusArchived {
			return httperror.BadRequest("Use rollback to reactivate a published version")
		}
		result, err := service.ValidateJourneyVersion(ctx, actor, versionID)
		if err != nil {
			return err
		}
		if !result.Valid {
			return httperror.BadRequest("Resolve journey validation errors before publishing")
		}
		d := JourneySettings(v)
		if err := service.checkJourneyConnection(ctx, actor, d, versionID); err != nil {
			return err
		}
		if _, err := service.PublishVersion(ctx, actor, versionID); err != nil {
			return err
		}
		b := JourneyBinding{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: botID, VersionID: versionID, ConnectionID: d.ConnectionID, Mode: d.Mode, Trigger: defaultString(d.Trigger, d.Category)}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "bot_id"}}, DoUpdates: clause.AssignmentColumns([]string{"version_id", "connection_id", "mode", "trigger", "paused", "updated_at"})}).Create(&b).Error
	})
	if err != nil {
		return JourneyDetail{}, err
	}
	return s.GetJourney(ctx, actor, botID, &versionID)
}

func (s *Service) checkJourneyConnection(ctx context.Context, actor auth.CurrentUser, d JourneyDefinition, versionID uuid.UUID) error {
	if d.ConnectionID == nil {
		if d.Mode == "whatsapp_flow" {
			return httperror.BadRequest("Choose a connected WhatsApp channel")
		}
		return nil
	}
	var c core.Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, *d.ConnectionID).First(&c).Error; err != nil {
		return httperror.BadRequest("Channel connection not found")
	}
	if !containsString([]string{"active", "connected", "healthy", "degraded"}, c.Status) {
		return httperror.BadRequest("Channel connection must be healthy before publishing")
	}
	if d.Mode == "whatsapp_flow" {
		var p JourneyFlowDeployment
		if c.Provider != "whatsapp" || s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ? AND connection_id = ? AND status = ?", actor.OrganizationID, versionID, c.ID, "published").First(&p).Error != nil {
			return httperror.BadRequest("Publish and validate this version's provider Flow first")
		}
	}
	return nil
}

func (s *Service) ActivateJourney(ctx context.Context, actor auth.CurrentUser, botID, versionID uuid.UUID, mode string, paused bool) (JourneyDetail, error) {
	if !actor.Role.HasPermission(authz.PermissionBotPublish) {
		return JourneyDetail{}, httperror.Forbidden("You cannot switch or roll back journeys")
	}
	detail, err := s.GetJourney(ctx, actor, botID, &versionID)
	if err != nil {
		return detail, err
	}
	if detail.Version.PublishedAt == nil {
		return detail, httperror.BadRequest("Only a published snapshot can be activated")
	}
	d := detail.Definition
	d.Mode = mode
	if !containsString([]string{"guided_chat", "whatsapp_flow", "ai"}, mode) {
		return detail, httperror.BadRequest("Choose a valid mode")
	}
	if mode == "whatsapp_flow" {
		c := CompileJourneyFlow(d)
		if len(c.Errors) > 0 {
			return detail, httperror.BadRequest("This version cannot be compiled as a WhatsApp Flow")
		}
	}
	if err := s.checkJourneyConnection(ctx, actor, d, versionID); err != nil {
		return detail, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked Bot
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, botID).First(&locked).Error; err != nil {
			return err
		}
		var snapshot PublishedSnapshot
		if err := tx.Where("organization_id = ? AND bot_id = ? AND version_id = ?", actor.OrganizationID, botID, versionID).First(&snapshot).Error; err != nil {
			return httperror.BadRequest("Published snapshot is missing")
		}
		b := JourneyBinding{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: botID, VersionID: versionID, ConnectionID: d.ConnectionID, Mode: mode, Trigger: defaultString(d.Trigger, d.Category), Paused: paused}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "bot_id"}}, DoUpdates: clause.AssignmentColumns([]string{"version_id", "connection_id", "trigger", "mode", "paused", "updated_at"})}).Create(&b).Error; err != nil {
			return err
		}
		if err := tx.Model(&Bot{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, botID).Updates(map[string]any{"published_version_id": versionID, "status": BotStatusActive}).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot", botID, "journey_activated", jsonValue(map[string]any{"version_id": versionID, "mode": mode, "paused": paused}))
	})
	if err != nil {
		return detail, err
	}
	return s.GetJourney(ctx, actor, botID, &versionID)
}

func (s *Service) CloneJourneyDraft(ctx context.Context, actor auth.CurrentUser, botID, sourceID uuid.UUID) (JourneyDetail, error) {
	detail, err := s.GetJourney(ctx, actor, botID, &sourceID)
	if err != nil {
		return detail, err
	}
	settings := detail.Definition
	settings.Steps = nil
	version, err := s.CreateVersion(ctx, actor, botID, VersionInput{SourceVersionID: &sourceID, Metadata: jsonValue(map[string]any{"journey": settings})})
	if err != nil {
		return detail, err
	}
	return s.GetJourney(ctx, actor, botID, &version.ID)
}

func (s *Service) ArchiveJourney(ctx context.Context, actor auth.CurrentUser, botID uuid.UUID) error {
	if !canManageBots(actor.Role) {
		return httperror.Forbidden("You cannot archive journeys")
	}
	if _, err := s.GetJourney(ctx, actor, botID, nil); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Bot{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, botID).Update("status", BotStatusArchived).Error; err != nil {
			return err
		}
		if err := tx.Model(&JourneyBinding{}).Where("organization_id = ? AND bot_id = ?", actor.OrganizationID, botID).Update("paused", true).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "bot", botID, "journey_archived", "{}")
	})
}

func JourneyStepLibrary() []map[string]string {
	items := []map[string]string{}
	for _, kind := range []string{"message", "question", "buttons", "list", "entry_menu", "store_selector", "category_selector", "product_selector", "variant_selector", "quantity", "fulfilment_selector", "address", "review_cart", "condition", "check_inventory", "add_to_cart", "create_order", "payment", "payment_status", "notification", "handoff", "end"} {
		items = append(items, map[string]string{"kind": kind, "name": strings.Title(strings.ReplaceAll(kind, "_", " "))})
	}
	for _, answer := range []string{"yes_no", "number", "email", "phone", "date", "location", "media"} {
		items = append(items, map[string]string{"kind": "question", "name": strings.Title(strings.ReplaceAll(answer, "_", " ")), "answer_type": answer})
	}
	return items
}

// Stable ordering makes compilation and validation artifacts reproducible.
func sortedJourneyKeys(values map[string]bool) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func journeyError(key, message string) ValidationIssue {
	return ValidationIssue{Path: fmt.Sprintf("steps.%s", key), Message: message}
}
