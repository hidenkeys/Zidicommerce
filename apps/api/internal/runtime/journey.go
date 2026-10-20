package runtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type JourneyPreviewSession struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	VersionID      uuid.UUID `gorm:"type:uuid" json:"version_id"`
	ActorID        uuid.UUID `gorm:"type:uuid" json:"-"`
	Snapshot       string    `json:"-"`
	State          string    `json:"-"`
	Scenario       string    `json:"scenario"`
	LastResult     string    `json:"-"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (JourneyPreviewSession) TableName() string { return "journey_preview_sessions" }

type JourneyExchangeReceipt struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journey_receipt"`
	SessionID      uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journey_receipt"`
	EventKey       string    `gorm:"uniqueIndex:idx_journey_receipt"`
	Result         string
	CreatedAt      time.Time
}

func (JourneyExchangeReceipt) TableName() string { return "journey_exchange_receipts" }

type JourneyFlowSession struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	ConnectionID   uuid.UUID `gorm:"type:uuid"`
	ConversationID uuid.UUID `gorm:"type:uuid"`
	VersionID      uuid.UUID `gorm:"type:uuid"`
	TokenHash      string    `gorm:"uniqueIndex"`
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

func (JourneyFlowSession) TableName() string { return "journey_flow_sessions" }

type JourneyPreviewResult struct {
	SessionID   uuid.UUID        `json:"session_id"`
	CurrentStep string           `json:"current_step"`
	Turn        string           `json:"turn"`
	Result      RuntimeResult    `json:"result"`
	Cart        []map[string]any `json:"cart"`
	FlowScreen  string           `json:"flow_screen"`
	Controlled  bool             `json:"controlled_test"`
}
type JourneyAnswer struct {
	EventID     string       `json:"event_id"`
	Turn        string       `json:"turn"`
	Text        string       `json:"text"`
	Command     string       `json:"command"`
	Location    *Location    `json:"location,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func opaqueJourneyID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func journeyTurn(session ConversationSession) string {
	return stringValue(parseJSONMap(session.SystemContext)["journey_turn"])
}

func (s *Service) StartJourneyPreview(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, scenario string) (JourneyPreviewResult, error) {
	if !actor.Role.HasPermission(authz.PermissionBotView) {
		return JourneyPreviewResult{}, httperror.Forbidden("You cannot preview journeys")
	}
	if !containsJourney([]string{"normal", "empty_catalogue", "unavailable_stock", "payment_failure", "handoff", "unsupported_channel"}, scenario) {
		return JourneyPreviewResult{}, httperror.BadRequest("Choose a valid preview scenario")
	}
	config, err := bot.NewService(s.db).GetConfiguration(ctx, actor, versionID)
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	if !bot.IsJourney(config.Version) {
		return JourneyPreviewResult{}, httperror.NotFound("Journey not found")
	}
	id := uuid.New()
	settings := bot.JourneySettings(config.Version)
	state := ConversationSession{ID: id, OrganizationID: actor.OrganizationID, BotID: config.Version.BotID, BotVersionID: versionID, CurrentStepKey: config.Version.StartStepKey, Status: SessionActive, Variables: "{}", SystemContext: jsonValue(map[string]any{"journey_preview": true, "scenario": scenario})}
	if settings.ConnectionID != nil {
		state.ChannelID = *settings.ConnectionID
	}
	row := JourneyPreviewSession{ID: id, OrganizationID: actor.OrganizationID, VersionID: versionID, ActorID: actor.ID, Snapshot: jsonValue(config), Scenario: scenario, ExpiresAt: s.now().Add(time.Duration(settings.TTLMinutes) * time.Minute)}
	result, err := s.executeJourney(ctx, config, &state, InboundMessage{SimulatorStart: true})
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	row.State, row.LastResult = jsonValue(state), jsonValue(result)
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return JourneyPreviewResult{}, err
	}
	return journeyPreviewResult(row, state, result), nil
}

func (s *Service) AnswerJourneyPreview(ctx context.Context, actor auth.CurrentUser, id uuid.UUID, input JourneyAnswer) (JourneyPreviewResult, error) {
	if !actor.Role.HasPermission(authz.PermissionBotView) {
		return JourneyPreviewResult{}, httperror.Forbidden("You cannot preview journeys")
	}
	if input.EventID == "" || len(input.EventID) > 128 || len(input.Text) > 4096 {
		return JourneyPreviewResult{}, httperror.BadRequest("A bounded event identifier and response are required")
	}
	var out JourneyPreviewResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row JourneyPreviewSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND actor_id = ? AND id = ?", actor.OrganizationID, actor.ID, id).First(&row).Error; err != nil {
			return httperror.NotFound("Preview session not found")
		}
		if !row.ExpiresAt.After(s.now()) {
			return httperror.BadRequest("Preview expired; restart it")
		}
		var receipt JourneyExchangeReceipt
		if err := tx.Where("organization_id = ? AND session_id = ? AND event_key = ?", actor.OrganizationID, id, "preview:"+input.EventID).First(&receipt).Error; err == nil {
			return json.Unmarshal([]byte(receipt.Result), &out)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		var config bot.VersionConfiguration
		var state ConversationSession
		if json.Unmarshal([]byte(row.Snapshot), &config) != nil || json.Unmarshal([]byte(row.State), &state) != nil {
			return httperror.BadRequest("Preview state is unavailable")
		}
		if input.Turn != journeyTurn(state) && input.Command == "" {
			return httperror.BadRequest("This selection is stale; use the current preview step")
		}
		engine := NewService(tx, core.NewService(tx, nil), s.log)
		text := input.Text
		if input.Command != "" {
			text = input.Command
		}
		result, err := engine.executeJourney(ctx, config, &state, InboundMessage{Text: text, Location: input.Location, Attachments: input.Attachments})
		if err != nil {
			return err
		}
		if state.BotVersionID != config.Version.ID {
			config, err = engine.loadSnapshot(ctx, actor.OrganizationID, state.BotID, state.BotVersionID)
			if err != nil {
				return err
			}
			row.VersionID, row.Snapshot = state.BotVersionID, jsonValue(config)
		}
		row.State, row.LastResult = jsonValue(state), jsonValue(result)
		if err := tx.Model(&row).Updates(map[string]any{"state": row.State, "last_result": row.LastResult, "version_id": row.VersionID, "snapshot": row.Snapshot}).Error; err != nil {
			return err
		}
		out = journeyPreviewResult(row, state, result)
		return tx.Create(&JourneyExchangeReceipt{ID: uuid.New(), OrganizationID: actor.OrganizationID, SessionID: id, EventKey: "preview:" + input.EventID, Result: jsonValue(out)}).Error
	})
	return out, err
}
func journeyPreviewResult(row JourneyPreviewSession, state ConversationSession, result RuntimeResult) JourneyPreviewResult {
	screen := bot.FlowScreenID(state.CurrentStepKey)
	if state.Status != SessionActive {
		screen = "FINISH"
	}
	return JourneyPreviewResult{SessionID: row.ID, CurrentStep: state.CurrentStepKey, Turn: journeyTurn(state), Result: result, Cart: mapSlice(parseJSONMap(state.Variables)["preview_cart"]), FlowScreen: screen}
}

func (s *Service) executeJourney(ctx context.Context, config bot.VersionConfiguration, session *ConversationSession, input InboundMessage) (RuntimeResult, error) {
	variables, system := parseJSONMap(session.Variables), parseJSONMap(session.SystemContext)
	defer func() { session.Variables = jsonValue(variables); session.SystemContext = jsonValue(system) }()
	preview := system["journey_preview"] == true
	if preview && system["scenario"] == "handoff" {
		session.Status = SessionHandoff
		session.ExpectedInput = ""
		return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Handoff: true, Messages: []OutboundMessage{{Type: MessageText, Text: "Preview handoff: a team member will help. No support ticket was created."}}}, nil
	}
	result := RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{}, Metadata: map[string]any{"preview": preview}}
	rctx := RuntimeContext{Session: *session, Variables: variables, System: system, Source: core.CommerceEventSourceRuntime}
	steps := indexSteps(config.Steps)
	appendText := func(text string) {
		if text != "" {
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: text})
		}
	}
	if command := classifyCommand(input.Text); command != commandNone {
		switch command {
		case commandCancel:
			session.Status = SessionCancelled
			session.ExpectedInput = ""
			appendText("Your journey has been cancelled. Existing orders and payments are unchanged.")
			result.SessionStatus = session.Status
			return result, nil
		case commandMenu, commandRestart:
			if stringValue(variables["order_id"]) != "" {
				appendText("Your order is already created. Use payment status or contact support.")
				return result, nil
			}
			variables = map[string]any{}
			rctx.Variables = variables
			for _, key := range []string{"journey_options", "journey_history", "journey_turn", "journey_page", "journey_retries", "journey_write_boundary", "stock_checked", "journey_flow_launched"} {
				delete(system, key)
			}
			rctx.System = system
			session.CurrentStepKey = config.Version.StartStepKey
			session.ExpectedInput = ""
			session.Status = SessionActive
		case commandBack:
			history := mapSlice(system["journey_history"])
			if len(history) == 0 || system["journey_write_boundary"] == true {
				appendText("Back is unavailable after a commerce action. Continue or contact support.")
				return result, nil
			}
			last := history[len(history)-1]
			session.CurrentStepKey = stringValue(last["step"])
			session.ExpectedInput = ""
			variables = parseJSONMap(stringValue(last["variables"]))
			rctx.Variables = variables
			system["journey_history"] = history[:len(history)-1]
		}
	}
	if session.Status != SessionActive {
		appendText("This journey has ended. Send menu to start again.")
		return result, nil
	}
	if session.ExpectedInput != "" {
		step, ok := steps[session.CurrentStepKey]
		if !ok {
			return result, httperror.BadRequest("Journey step unavailable")
		}
		j := bot.JourneyStepSettings(step)
		if input.Metadata["journey_turn"] != nil && input.Metadata["journey_turn"] != system["journey_turn"] {
			return result, httperror.BadRequest("This selection is stale")
		}
		for _, option := range mapSlice(system["journey_options"]) {
			if input.Text == stringValue(option["id"]) && strings.HasPrefix(stringValue(option["value"]), "__page:") {
				page, _ := strconv.Atoi(strings.TrimPrefix(stringValue(option["value"]), "__page:"))
				system["journey_page"] = page
				session.ExpectedInput = ""
			}
		}
		if session.ExpectedInput != "" {
			value, next, err := s.acceptJourneyAnswer(ctx, j, input, rctx)
			if err != nil {
				retries := int(int64Value(system["journey_retries"])) + 1
				system["journey_retries"] = retries
				appendText(err.Error())
				if j.RetryMessage != "" {
					appendText(j.RetryMessage)
				}
				if retries >= maxJourneyRetries(j) && j.Failure != "" {
					session.CurrentStepKey = j.Failure
					session.ExpectedInput = ""
				} else {
					result.Messages = append(result.Messages, journeyQuestionMessage(j, system))
					return result, nil
				}
			} else {
				if j.Kind == "entry_menu" {
					selected, err := s.selectJourneyEntry(ctx, session, stringValue(value), system)
					if err != nil {
						return result, err
					}
					if selected == nil {
						if stringValue(value) == "__human" {
							session.CurrentStepKey = j.Failure
							session.ExpectedInput = ""
						} else {
							system["journey_mode"] = "ai"
							session.ExpectedInput = ""
							appendText("What would you like to know about our products or policies?")
							return result, nil
						}
					} else {
						clear(variables)
						session.Variables, session.SystemContext = jsonValue(variables), jsonValue(system)
						result, err = s.executeConfiguredJourney(ctx, *selected, session, InboundMessage{SimulatorStart: true})
						variables, system = parseJSONMap(session.Variables), parseJSONMap(session.SystemContext)
						return result, err
					}
				} else {
					history := mapSlice(system["journey_history"])
					history = append(history, map[string]any{"step": j.Key, "variables": jsonValue(variables)})
					system["journey_history"] = history
					variables[defaultString(j.Destination, "answer_"+j.Key)] = value
					system["journey_page"] = 0
					if j.Source == "stores" {
						delete(variables, "category_id")
						delete(variables, "product_id")
						delete(variables, "variant_id")
					}
					if j.Source == "products" {
						delete(variables, "variant_id")
					}
					session.CurrentStepKey = defaultString(next, j.Next)
					session.ExpectedInput = ""
					system["journey_retries"] = 0
				}
			}
		}
	}
	for guard := 0; guard < 80 && session.Status == SessionActive && session.ExpectedInput == ""; guard++ {
		step, ok := steps[session.CurrentStepKey]
		if !ok {
			return result, httperror.BadRequest("Journey route unavailable")
		}
		j := bot.JourneyStepSettings(step)
		if j.VisibleWhen != "" && stringValue(variables[j.VisibleWhen]) == "" {
			session.CurrentStepKey = j.Next
			continue
		}
		switch step.Type {
		case bot.StepQuestion, bot.StepChoice:
			options, err := s.journeyOptions(ctx, j, rctx)
			if err != nil || ((j.Source != "" || len(j.Options) > 0) && len(options) == 0) {
				appendText("That selection is unavailable right now.")
				session.CurrentStepKey = j.Failure
				if j.Failure == "" {
					session.Status = SessionHandoff
				}
				continue
			}
			turn := opaqueJourneyID()
			if len(options) > 10 {
				page := int(int64Value(system["journey_page"]))
				maxPage := (len(options) - 1) / 8
				if page < 0 || page > maxPage {
					page = 0
				}
				start := page * 8
				end := start + 8
				if end > len(options) {
					end = len(options)
				}
				visible := append([]bot.JourneyOption{}, options[start:end]...)
				if page > 0 {
					visible = append(visible, bot.JourneyOption{Label: "Previous options", Value: fmt.Sprint("__page:", page-1)})
				}
				if page < maxPage {
					visible = append(visible, bot.JourneyOption{Label: "More options", Value: fmt.Sprint("__page:", page+1)})
				}
				options = visible
			}
			pending := []map[string]any{}
			for _, option := range options {
				pending = append(pending, map[string]any{"id": opaqueJourneyID(), "label": option.Label, "value": option.Value, "next": option.Next})
			}
			system["journey_options"] = pending
			system["journey_turn"] = turn
			system["journey_retries"] = 0
			session.ExpectedInput = step.StepKey
			result.Messages = append(result.Messages, journeyQuestionMessage(j, system))
		case bot.StepCondition:
			c, err := findCondition(config.Conditions, step.ConditionID)
			if err != nil {
				return result, err
			}
			matched, err := evaluateCondition(c, rctx)
			if err != nil {
				return result, err
			}
			session.CurrentStepKey = step.FallbackStepKey
			if matched {
				session.CurrentStepKey = step.NextStepKey
			}
		case bot.StepAction:
			a, err := findAction(config.Actions, step.ActionID)
			if err != nil {
				return result, err
			}
			outputs, err := s.journeyAction(ctx, a, j, rctx, preview)
			if err != nil {
				appendText("We could not complete this step. Your order and payment status have not been assumed.")
				if j.Failure == "" {
					session.Status = SessionHandoff
				} else {
					session.CurrentStepKey = j.Failure
				}
				continue
			}
			for key, value := range outputs {
				if key != "message" {
					variables[key] = value
				}
			}
			appendText(stringValue(outputs["message"]))
			if !preview {
				s.recordEvent(ctx, *session, EventActionCompleted, "info", j.Key, a.ActionKey, map[string]any{"action_type": a.ActionType})
			}
			session.CurrentStepKey = j.Next
		case bot.StepMessage:
			appendText(rctx.Render(step.Message, "not available"))
			session.CurrentStepKey = j.Next
		case bot.StepHandoff:
			appendText(step.Message)
			session.Status = SessionHandoff
			result.Handoff = true
			if !preview {
				session.ConversationStatus = ConversationHumanRequested
				session.HandoffState = "open"
				if err := s.openSupportHandoff(ctx, *session, step.Message, rctx); err != nil {
					return result, err
				}
			}
		case bot.StepEnd:
			appendText(step.Message)
			session.Status = SessionCompleted
		default:
			return result, httperror.BadRequest("Unsupported journey step")
		}
		if session.CurrentStepKey == "" && session.Status == SessionActive {
			session.Status = SessionCompleted
		}
		rctx.Session = *session
	}
	result.SessionStatus = session.Status
	result.Handoff = session.Status == SessionHandoff
	return result, nil
}

func journeyQuestionMessage(j bot.JourneyStep, system map[string]any) OutboundMessage {
	message := OutboundMessage{Type: MessageText, Text: j.Text, Metadata: map[string]any{"step": j.Key, "answer_type": j.AnswerType, "turn": system["journey_turn"], "help": j.Help}}
	for _, o := range mapSlice(system["journey_options"]) {
		message.Options = append(message.Options, MessageOption{ID: stringValue(o["id"]), Label: stringValue(o["label"])})
	}
	if len(message.Options) > 0 {
		message.Type = MessageButtons
		if len(message.Options) > 3 || j.Presentation == "list" {
			message.Type = MessageList
		}
	}
	if j.AnswerType == "location" {
		message.LocationRequest = true
		message.Type = "location_request"
	}
	textFallback := system["scenario"] == "unsupported_channel"
	for _, option := range message.Options {
		if len([]rune(option.Label)) > 24 || (message.Type == MessageButtons && len([]rune(option.Label)) > 20) {
			textFallback = true
		}
	}
	if textFallback && len(message.Options) > 0 {
		message.Type = MessageText
		for index, o := range message.Options {
			message.Text += fmt.Sprintf("\n%d. %s", index+1, o.Label)
		}
	}
	return message
}

func (s *Service) acceptJourneyAnswer(ctx context.Context, j bot.JourneyStep, input InboundMessage, rctx RuntimeContext) (any, string, error) {
	text := strings.TrimSpace(input.Text)
	if len(text) > 4096 || (j.MaxLength > 0 && len([]rune(text)) > j.MaxLength) {
		return nil, "", fmt.Errorf("That answer is too long")
	}
	if j.Source != "" || len(j.Options) > 0 {
		pending := mapSlice(rctx.System["journey_options"])
		var chosen map[string]any
		for index, option := range pending {
			if text == stringValue(option["id"]) || text == strconv.Itoa(index+1) {
				chosen = option
				break
			}
		}
		if chosen == nil {
			return nil, "", fmt.Errorf("Choose a current option using its button or number")
		}
		// Reload dynamic values at answer time; deleted/inactive records and stale
		// fulfilment options are rejected before they enter conversation state.
		if j.Source != "" {
			fresh, err := s.journeyOptions(ctx, j, rctx)
			if err != nil {
				return nil, "", fmt.Errorf("That selection could not be refreshed. Try again or contact support")
			}
			valid := false
			for _, o := range fresh {
				if o.Value == stringValue(chosen["value"]) {
					valid = true
				}
			}
			if !valid {
				return nil, "", fmt.Errorf("That option is no longer available; restart or contact support")
			}
		}
		return chosen["value"], stringValue(chosen["next"]), nil
	}
	if text == "" && !j.Required {
		return "", "", nil
	}
	typeName := defaultString(j.AnswerType, "text")
	if j.Kind == "quantity" {
		typeName = "number"
	}
	if typeName == "date" {
		if _, err := time.Parse("2006-01-02", text); err != nil {
			return nil, "", fmt.Errorf("Enter a date as YYYY-MM-DD")
		}
		return text, "", nil
	}
	if typeName == "media" {
		if len(input.Attachments) == 0 {
			return nil, "", fmt.Errorf("Please send an image or media attachment")
		}
		return input.Attachments, "", nil
	}
	if typeName == "location" && input.Location != nil {
		l := input.Location
		if math.IsNaN(l.Latitude) || math.IsNaN(l.Longitude) || math.IsInf(l.Latitude, 0) || math.IsInf(l.Longitude, 0) || l.Latitude < -90 || l.Latitude > 90 || l.Longitude < -180 || l.Longitude > 180 {
			return nil, "", fmt.Errorf("Share a valid location")
		}
	}
	if typeName == "email" {
		address, err := mail.ParseAddress(text)
		if err != nil || address.Address != text {
			return nil, "", fmt.Errorf("Enter a valid email address")
		}
	}
	q := bot.Question{Type: typeName, Required: j.Required, Validation: jsonValue(map[string]any{"min": j.Min, "max": j.Max})}
	value, err := validateAnswer(q, "[]", input)
	if typeName == "number" && err == nil {
		n, _ := numberValue(value)
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, "", fmt.Errorf("Enter a finite number")
		}
	}
	if j.Kind == "quantity" && err == nil {
		n, _ := numberValue(value)
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < 1 || n > 999 {
			return nil, "", fmt.Errorf("Quantity must be a whole number")
		}
	}
	return value, "", err
}

func (s *Service) journeyOptions(ctx context.Context, j bot.JourneyStep, rctx RuntimeContext) ([]bot.JourneyOption, error) {
	if j.Source == "" {
		return j.Options, nil
	}
	if rctx.System["scenario"] == "empty_catalogue" && containsJourney([]string{"categories", "products", "variants"}, j.Source) {
		return []bot.JourneyOption{}, nil
	}
	actor := auth.CurrentUser{OrganizationID: rctx.Session.OrganizationID, Role: authz.MerchantAdmin}
	options := []bot.JourneyOption{}
	add := func(id, label string) { options = append(options, bot.JourneyOption{ID: id, Value: id, Label: label}) }
	switch j.Source {
	case "journeys":
		var rows []bot.JourneyBinding
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND paused = ? AND trigger <> ?", actor.OrganizationID, rctx.Session.ChannelID, false, "default").Order("trigger ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.Trigger, strings.Title(strings.ReplaceAll(row.Trigger, "_", " ")))
		}
		add("__ai", "Product or policy question")
		add("__human", "Human support")
	case "stores":
		rows, err := s.commerce.ListStores(ctx, actor)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Status == core.StatusActive {
				add(row.ID.String(), row.Name)
			}
		}
	case "categories":
		rows, err := s.commerce.ListCategories(ctx, actor)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Status == core.StatusActive {
				add(row.ID.String(), row.Name)
			}
		}
	case "products":
		storeID, err := uuid.Parse(stringValue(rctx.Variables["store_id"]))
		if err != nil {
			return nil, fmt.Errorf("Choose a store first")
		}
		rows, err := s.commerce.ListProducts(ctx, actor)
		if err != nil {
			return nil, err
		}
		inventory, err := s.commerce.ListInventory(ctx, actor, &storeID)
		if err != nil {
			return nil, err
		}
		available := map[uuid.UUID]bool{}
		for _, level := range inventory {
			if level.Available() > 0 {
				available[level.VariantID] = true
			}
		}
		for _, row := range rows {
			if row.Status != core.StatusActive {
				continue
			}
			if category := stringValue(rctx.Variables["category_id"]); category != "" && (row.CategoryID == nil || row.CategoryID.String() != category) {
				continue
			}
			has := false
			for _, v := range row.Variants {
				if v.Status == core.StatusActive && available[v.ID] {
					has = true
				}
			}
			if has {
				add(row.ID.String(), row.Name)
			}
		}
	case "variants":
		productID, err := uuid.Parse(stringValue(rctx.Variables["product_id"]))
		if err != nil {
			return nil, err
		}
		product, err := s.commerce.GetProduct(ctx, actor, productID)
		if err != nil {
			return nil, err
		}
		storeID, err := uuid.Parse(stringValue(rctx.Variables["store_id"]))
		if err != nil {
			return nil, err
		}
		for _, v := range product.Variants {
			if v.Status != core.StatusActive {
				continue
			}
			if _, err := s.commerce.CheckInventory(ctx, actor, storeID, v.ID, 1); err == nil {
				add(v.ID.String(), v.Name+" - "+formatMinorCurrency(v.PriceMinor, v.Currency))
			}
		}
	case "fulfilment":
		id, err := uuid.Parse(stringValue(rctx.Variables["store_id"]))
		if err != nil {
			return nil, err
		}
		store, err := s.commerce.GetStore(ctx, actor, id)
		if err != nil {
			return nil, err
		}
		for _, mode := range store.FulfilmentModes {
			if mode.Enabled {
				add(mode.Mode, customerFulfilmentLabel(mode.Mode))
			}
		}
	case "orders":
		if rctx.Session.CustomerID == nil {
			return options, nil
		}
		rows, err := s.commerce.ListCustomerOrders(ctx, actor, *rctx.Session.CustomerID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.ID.String(), row.OrderNumber+" - "+customerOrderStatus(row.Status))
		}
	case "complaint_categories":
		for _, label := range []string{"Wrong item", "Damaged item", "Delivery problem", "Payment problem", "Other"} {
			add(strings.ToLower(strings.ReplaceAll(label, " ", "_")), label)
		}
	}
	return options, nil
}

func (s *Service) journeyAction(ctx context.Context, a bot.Action, j bot.JourneyStep, rctx RuntimeContext, preview bool) (map[string]any, error) {
	if !preview {
		if err := s.actions.authorizeRuntimeAction(ctx, a.ActionType, rctx); err != nil {
			return nil, err
		}
	}
	actor := auth.CurrentUser{OrganizationID: rctx.Session.OrganizationID, Role: authz.MerchantAdmin}
	storeID, _ := uuid.Parse(stringValue(rctx.Variables["store_id"]))
	variantID, _ := uuid.Parse(stringValue(rctx.Variables["variant_id"]))
	quantity := int(int64Value(rctx.Variables["quantity"]))
	if j.Kind == "check_inventory" {
		if rctx.System["scenario"] == "unavailable_stock" {
			return nil, fmt.Errorf("No stock")
		}
		if _, err := s.commerce.CheckInventory(ctx, actor, storeID, variantID, quantity); err != nil {
			return nil, err
		}
		rctx.System["stock_checked"] = true
		return map[string]any{}, nil
	}
	if j.Kind == "add_to_cart" {
		if rctx.System["stock_checked"] != true {
			return nil, fmt.Errorf("Inventory check required")
		}
		if preview {
			variant, err := s.commerce.GetVariant(ctx, actor, variantID)
			if err != nil {
				return nil, err
			}
			cart := mapSlice(rctx.Variables["preview_cart"])
			cart = append(cart, map[string]any{"variant_id": variant.ID.String(), "name": variant.Name, "quantity": quantity, "unit_price_minor": variant.PriceMinor, "currency": variant.Currency})
			rctx.Variables["preview_cart"] = cart
			rctx.System["journey_write_boundary"] = true
			return map[string]any{"message": "Item added to your preview cart."}, nil
		}
	}
	if preview {
		switch j.Kind {
		case "review_cart":
			cart := mapSlice(rctx.Variables["preview_cart"])
			total := int64(0)
			lines := "Preview order review:"
			for _, item := range cart {
				id, _ := uuid.Parse(stringValue(item["variant_id"]))
				v, err := s.commerce.GetVariant(ctx, actor, id)
				if err != nil {
					return nil, err
				}
				q := int(int64Value(item["quantity"]))
				if _, err := s.commerce.CheckInventory(ctx, actor, storeID, id, q); err != nil {
					return nil, err
				}
				total += v.PriceMinor * int64(q)
				lines += fmt.Sprintf("\n%d x %s - %s", q, v.Name, formatMinorCurrency(v.PriceMinor, v.Currency))
			}
			return map[string]any{"total_minor": total, "message": lines + "\nTotal: " + formatMinorCurrency(total, "NGN")}, nil
		case "create_order":
			return map[string]any{"order_id": "preview-order", "message": "Preview order created. No real order was saved."}, nil
		case "payment":
			return map[string]any{"payment_status": "pending", "message": "Preview payment link is ready. No payment provider was contacted."}, nil
		case "payment_status":
			status := "paid"
			if rctx.System["scenario"] == "payment_failure" {
				status = "failed"
			}
			return map[string]any{"payment_status": status, "message": "Preview payment status: " + status}, nil
		case "notification":
			return map[string]any{"message": "Preview notification only. Nothing was sent."}, nil
		case "complaint_action":
			return map[string]any{"message": "Preview complaint recorded. No real complaint was saved."}, nil
		case "track_action":
			return map[string]any{"message": "Preview order tracking. Customer orders are private to their own conversation."}, nil
		default:
			return nil, fmt.Errorf("Unsupported preview action")
		}
	}
	// Optional facts (cart before its creation, order before confirmation) are
	// absent by design. Commerce services validate required inputs per action.
	inputs := map[string]any{"store_id": rctx.Variables["store_id"], "variant_id": rctx.Variables["variant_id"], "quantity": rctx.Variables["quantity"], "cart_id": rctx.Variables["cart_id"], "order_id": rctx.Variables["order_id"], "payment_reference": rctx.Variables["payment_reference"], "fulfilment_type": rctx.Variables["fulfilment_mode"], "delivery_address": rctx.Variables["delivery_address"], "recipient_name": rctx.Variables["customer_name"], "recipient_phone": rctx.Variables["customer_phone"], "email": rctx.Variables["customer_email"], "provider": "paystack"}
	if rctx.System["journey_controlled_test"] == true {
		inputs["provider"] = "test"
	}
	if rctx.System["journey_mode"] == "whatsapp_flow" && containsJourney([]string{"create_order", "payment", "payment_status"}, j.Kind) {
		if err := s.commerce.RequireSandboxPaymentProvider(ctx, actor.OrganizationID, stringValue(inputs["provider"])); err != nil {
			return nil, err
		}
	}
	if rctx.Session.CustomerID != nil {
		inputs["customer_id"] = rctx.Session.CustomerID.String()
	}
	if j.Kind == "complaint_action" {
		inputs["message"] = rctx.Variables["complaint_message"]
	}
	if j.Kind == "notification" {
		if rctx.System["journey_controlled_test"] == true {
			return map[string]any{"message": "Test notification suppressed."}, nil
		}
		inputs["message"] = "Your order payment has been verified."
		inputs["recipient"] = stringValue(rctx.System["sender"])
		if j.Source == "store" {
			store, err := s.commerce.GetStore(ctx, actor, storeID)
			if err != nil {
				return nil, err
			}
			inputs["recipient"] = stringValue(parseJSONMap(store.Metadata)["notification_recipient"])
			inputs["message"] = "A customer order has verified payment: " + stringValue(rctx.Variables["order_number"])
		}
	}
	if j.Kind == "add_to_cart" {
		// Cart creation and add are one DB transaction, with a turn receipt. A
		// process restart after commit cannot repeat the item mutation.
		var outputs map[string]any
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			key := "action:" + j.Key + ":" + stringValue(rctx.System["journey_turn"])
			var receipt JourneyExchangeReceipt
			if err := tx.Where("organization_id = ? AND session_id = ? AND event_key = ?", actor.OrganizationID, rctx.Session.ID, key).First(&receipt).Error; err == nil {
				return json.Unmarshal([]byte(receipt.Result), &outputs)
			} else if err != gorm.ErrRecordNotFound {
				return err
			}
			commerce := core.NewService(tx, nil)
			customerID, err := requireSessionCustomer(rctx)
			if err != nil {
				return err
			}
			cartID, _ := uuid.Parse(stringValue(rctx.Variables["cart_id"]))
			if cartID == uuid.Nil {
				cart, err := commerce.CreateCart(ctx, actor, core.CartInput{StoreID: storeID, CustomerID: customerID})
				if err != nil {
					return err
				}
				cartID = cart.ID
			}
			summary, err := commerce.AddCartItem(ctx, actor, cartID, core.CartItemInput{VariantID: variantID, Quantity: quantity})
			if err != nil {
				return err
			}
			outputs = cartOutputs(summary)
			outputs["message"] = cartSummaryMessage(summary)
			return tx.Create(&JourneyExchangeReceipt{ID: uuid.New(), OrganizationID: actor.OrganizationID, SessionID: rctx.Session.ID, EventKey: key, Result: jsonValue(outputs)}).Error
		})
		if err == nil {
			rctx.System["journey_write_boundary"] = true
			rctx.System["stock_checked"] = false
		}
		return outputs, err
	}
	if j.Kind == "create_order" && stringValue(rctx.Variables["confirmation"]) != "confirm" {
		return nil, fmt.Errorf("Order confirmation required")
	}
	if j.Kind == "notification" && stringValue(rctx.Variables["payment_status"]) != core.PaymentPaid {
		return nil, fmt.Errorf("Verified payment required")
	}
	if j.Kind == "complaint_action" || j.Kind == "notification" {
		var outputs map[string]any
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			key := "action:" + j.Key + ":" + stringValue(rctx.System["journey_turn"])
			var receipt JourneyExchangeReceipt
			if err := tx.Where("organization_id = ? AND session_id = ? AND event_key = ?", actor.OrganizationID, rctx.Session.ID, key).First(&receipt).Error; err == nil {
				return json.Unmarshal([]byte(receipt.Result), &outputs)
			} else if err != gorm.ErrRecordNotFound {
				return err
			}
			var err error
			outputs, err = NewActionRegistry(tx, core.NewService(tx, nil)).ExecuteDirect(ctx, a.ActionType, rctx, inputs)
			if err != nil {
				return err
			}
			return tx.Create(&JourneyExchangeReceipt{ID: uuid.New(), OrganizationID: actor.OrganizationID, SessionID: rctx.Session.ID, EventKey: key, Result: jsonValue(outputs)}).Error
		})
		return outputs, err
	}
	return s.actions.ExecuteDirect(ctx, a.ActionType, rctx, inputs)
}
func maxJourneyRetries(j bot.JourneyStep) int {
	if j.MaxRetries > 0 {
		return j.MaxRetries
	}
	return 3
}
func containsJourney(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func flowTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
