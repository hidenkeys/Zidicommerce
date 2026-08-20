package runtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type runtimeFixture struct {
	db       *gorm.DB
	commerce *core.Service
	service  *Service
	actor    auth.CurrentUser
	channel  core.Channel
	bot      bot.Bot
	version  bot.BotVersion
}

func newRuntimeFixture(t *testing.T, config bot.VersionConfiguration) runtimeFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{},
		&organization.User{},
		&organization.OrganizationMembership{},
		&organization.OrganizationInvitation{},
		&organization.AuditLog{},
		&core.Store{},
		&core.StoreHour{},
		&core.StoreFulfilmentMode{},
		&core.StoreUserAssignment{},
		&core.Category{},
		&core.Product{},
		&core.Variant{},
		&core.ProductImage{},
		&core.InventoryLevel{},
		&core.Customer{},
		&core.Cart{},
		&core.CartItem{},
		&core.Order{},
		&core.OrderItem{},
		&core.OrderEvent{},
		&core.Payment{},
		&core.Fulfilment{},
		&core.Channel{},
		&bot.Bot{},
		&bot.BotVersion{},
		&bot.VersionModule{},
		&bot.Variable{},
		&bot.Question{},
		&bot.Action{},
		&bot.Condition{},
		&bot.Integration{},
		&bot.Step{},
		&bot.PublishedSnapshot{},
		&ConversationSession{},
		&ConversationMessage{},
		&ProcessedMessage{},
		&RuntimeEvent{},
		&ChannelOutboundMessage{},
	); err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: orgID, Role: authz.MerchantAdmin}
	if err := db.Create(&organization.Organization{ID: orgID, Name: "Example Merchant", Slug: "example-merchant", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	botID := uuid.New()
	versionID := uuid.New()
	botRecord := bot.Bot{ID: botID, OrganizationID: orgID, Name: "Example Store Assistant", Status: bot.BotStatusActive, DefaultLanguage: "en", Timezone: "Africa/Lagos", FallbackConfig: `{"message":"Please try again.","missing_variable":"unknown"}`, HandoffConfig: "{}", PublishedVersionID: &versionID, Metadata: "{}"}
	version := bot.BotVersion{ID: versionID, OrganizationID: orgID, BotID: botID, VersionNumber: 1, Status: bot.VersionStatusPublished, StartStepKey: config.Version.StartStepKey, ValidationErrors: "[]", Metadata: "{}"}
	config.Version = version
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	channel := core.Channel{ID: uuid.New(), OrganizationID: orgID, Provider: "test", DisplayName: "Test Channel", PhoneNumberID: "test-phone", DisplayNumber: "test", Status: core.StatusActive, Config: `{"bot_id":"` + botID.String() + `"}`, SecretConfig: "{}"}
	if err := db.Create(&botRecord).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&bot.PublishedSnapshot{ID: uuid.New(), OrganizationID: orgID, BotID: botID, VersionID: versionID, VersionNumber: 1, Snapshot: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	commerce := core.NewService(db, core.SafeTestProvider{})
	return runtimeFixture{db: db, commerce: commerce, service: NewService(db, commerce, nil), actor: actor, channel: channel, bot: botRecord, version: version}
}

func TestRuntimeExecutesMessageQuestionAndEnd(t *testing.T) {
	questionID := uuid.New()
	config := bot.VersionConfiguration{
		Version:   bot.BotVersion{StartStepKey: "start"},
		Questions: []bot.Question{{ID: questionID, QuestionKey: "ask_name", Text: "What is your name?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "customer_name"}},
		Steps: []bot.Step{
			{ID: uuid.New(), StepKey: "start", Type: bot.StepMessage, Title: "Start", Message: "Welcome.", NextStepKey: "ask_name"},
			{ID: uuid.New(), StepKey: "ask_name", Type: bot.StepQuestion, Title: "Ask name", QuestionID: &questionID, NextStepKey: "done"},
			{ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Thanks {{variables.customer_name}}."},
		},
	}
	fx := newRuntimeFixture(t, config)
	first, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-1", "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 2 || first.Messages[0].Text != "Welcome." || first.Messages[1].Text != "What is your name?" {
		t.Fatalf("unexpected first messages: %+v", first.Messages)
	}
	second, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-1", "Ada"))
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionStatus != SessionCompleted || second.Messages[0].Text != "Thanks Ada." {
		t.Fatalf("unexpected completion: %+v", second)
	}
	session, err := fx.service.GetConversation(context.Background(), fx.actor, second.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(session.Variables, "Ada") {
		t.Fatalf("expected variable to be persisted, got %s", session.Variables)
	}
}

func TestRuntimeRejectsInvalidQuestionInputWithoutAdvancing(t *testing.T) {
	questionID := uuid.New()
	config := bot.VersionConfiguration{
		Version:   bot.BotVersion{StartStepKey: "ask_email"},
		Questions: []bot.Question{{ID: questionID, QuestionKey: "ask_email", Text: "Email?", Type: "email", ResponseMode: "free_text", Required: true, VariableName: "email"}},
		Steps:     []bot.Step{{ID: uuid.New(), StepKey: "ask_email", Type: bot.StepQuestion, Title: "Ask email", QuestionID: &questionID, NextStepKey: "done"}, {ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Saved."}},
	}
	fx := newRuntimeFixture(t, config)
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-2", "hi")); err != nil {
		t.Fatal(err)
	}
	bad, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-2", "not-email"))
	if err != nil {
		t.Fatal(err)
	}
	if bad.SessionStatus != SessionActive || !strings.Contains(bad.Messages[0].Text, "valid email") {
		t.Fatalf("expected email retry, got %+v", bad)
	}
	good, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m3", "conv-2", "ada@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if good.SessionStatus != SessionCompleted {
		t.Fatalf("expected completion after valid email, got %+v", good)
	}
}

func TestRuntimeEvaluatesConditions(t *testing.T) {
	questionID := uuid.New()
	conditionID := uuid.New()
	config := bot.VersionConfiguration{
		Version:    bot.BotVersion{StartStepKey: "ask"},
		Questions:  []bot.Question{{ID: questionID, QuestionKey: "ask_pickup", Text: "Pickup?", Type: "yes_no", ResponseMode: "free_text", Required: true, VariableName: "pickup"}},
		Conditions: []bot.Condition{{ID: conditionID, ConditionKey: "pickup_yes", Name: "Pickup yes", Combinator: "and", Rules: `[{"field":"variables.pickup","operator":"equals","value":"true"}]`}},
		Steps: []bot.Step{
			{ID: uuid.New(), StepKey: "ask", Type: bot.StepQuestion, Title: "Ask", QuestionID: &questionID, NextStepKey: "branch"},
			{ID: uuid.New(), StepKey: "branch", Type: bot.StepCondition, Title: "Branch", ConditionID: &conditionID, NextStepKey: "yes", FallbackStepKey: "no"},
			{ID: uuid.New(), StepKey: "yes", Type: bot.StepEnd, Title: "Yes", Message: "Pickup selected."},
			{ID: uuid.New(), StepKey: "no", Type: bot.StepEnd, Title: "No", Message: "Delivery selected."},
		},
	}
	fx := newRuntimeFixture(t, config)
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-3", "start")); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-3", "yes"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages[0].Text != "Pickup selected." {
		t.Fatalf("expected condition true branch, got %+v", result.Messages)
	}
}

func TestRuntimeExecutesCommerceActionsWithMappings(t *testing.T) {
	ids := actionFlowIDs()
	config := commerceActionConfig(ids)
	fx := newRuntimeFixture(t, config)
	customer, store, variant := seedCommerce(t, fx)
	messages := []string{store.ID.String(), customer.ID.String(), variant.ID.String(), "2"}
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m0", "conv-4", "start")); err != nil {
		t.Fatal(err)
	}
	for index, text := range messages {
		if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m"+strconv.Itoa(index+1), "conv-4", text)); err != nil {
			t.Fatal(err)
		}
	}
	var orders []core.Order
	if err := fx.db.Where("organization_id = ?", fx.actor.OrganizationID).Find(&orders).Error; err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].TotalMinor != 840000 {
		t.Fatalf("expected one mapped order, got %+v", orders)
	}
}

func TestRuntimeIsIdempotentForDuplicateInboundMessage(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	first, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "same", "conv-5", "hi"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "same", "conv-5", "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ConversationID != second.ConversationID {
		t.Fatalf("expected duplicate to return same result")
	}
	var count int64
	if err := fx.db.Model(&ConversationMessage{}).Where("session_id = ?", first.ConversationID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected one inbound and one outbound persisted, got %d", count)
	}
}

func TestRuntimeRejectsDuplicateMessageWhileProcessing(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Create(&ProcessedMessage{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: fx.channel.ID, ExternalMessageID: "same", ExternalConversationID: "conv-processing", Status: "processing", Result: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "same", "conv-processing", "hi"))
	if err == nil {
		t.Fatal("expected duplicate processing conflict")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != ErrSessionConflict {
		t.Fatalf("expected session conflict, got %v", err)
	}
	var count int64
	if err := fx.db.Model(&ConversationMessage{}).Where("external_message_id = ?", "same").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no duplicate conversation messages, got %d", count)
	}
}

func TestRuntimeRejectsBotWithoutPublishedSnapshot(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Where("organization_id = ?", fx.actor.OrganizationID).Delete(&bot.PublishedSnapshot{}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-6", "hi"))
	if err == nil {
		t.Fatal("expected missing published snapshot error")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != ErrNoPublishedVersion {
		t.Fatalf("expected no published version error, got %v", err)
	}
}

func TestRuntimeHandoffPausesAutomation(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "handoff"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "handoff", Type: bot.StepHandoff, Title: "Handoff", Message: "Connecting you."}}}
	fx := newRuntimeFixture(t, config)
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-7", "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Handoff || result.SessionStatus != SessionHandoff {
		t.Fatalf("expected handoff result, got %+v", result)
	}
	next, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-7", "hello again"))
	if err != nil {
		t.Fatal(err)
	}
	if !next.Handoff || !strings.Contains(next.Messages[0].Text, "team member") {
		t.Fatalf("expected automation to remain paused, got %+v", next)
	}
}

func TestRuntimePinsExistingSessionToOriginalPublishedVersion(t *testing.T) {
	questionID := uuid.New()
	v1Config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "ask"}, Questions: []bot.Question{{ID: questionID, QuestionKey: "ask", Text: "Name?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "name"}}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "ask", Type: bot.StepQuestion, Title: "Ask", QuestionID: &questionID, NextStepKey: "done"}, {ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Old {{variables.name}}."}}}
	fx := newRuntimeFixture(t, v1Config)
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-8", "start")); err != nil {
		t.Fatal(err)
	}
	v2ID := uuid.New()
	v2Config := bot.VersionConfiguration{Version: bot.BotVersion{ID: v2ID, OrganizationID: fx.actor.OrganizationID, BotID: fx.bot.ID, VersionNumber: 2, Status: bot.VersionStatusPublished, StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "New version."}}}
	raw, _ := json.Marshal(v2Config)
	if err := fx.db.Create(&bot.BotVersion{ID: v2ID, OrganizationID: fx.actor.OrganizationID, BotID: fx.bot.ID, VersionNumber: 2, Status: bot.VersionStatusPublished, StartStepKey: "start", ValidationErrors: "[]", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&bot.PublishedSnapshot{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, BotID: fx.bot.ID, VersionID: v2ID, VersionNumber: 2, Snapshot: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&bot.Bot{}).Where("id = ?", fx.bot.ID).Update("published_version_id", v2ID).Error; err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-8", "Ada"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages[0].Text != "Old Ada." {
		t.Fatalf("expected pinned v1 response, got %+v", result.Messages)
	}
	session, err := fx.service.GetConversation(context.Background(), fx.actor, result.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if session.BotVersionID != fx.version.ID {
		t.Fatalf("expected pinned version %s, got %s", fx.version.ID, session.BotVersionID)
	}
}

func TestRuntimeResetTextRestartsActiveSession(t *testing.T) {
	questionID := uuid.New()
	config := bot.VersionConfiguration{
		Version:   bot.BotVersion{StartStepKey: "ask"},
		Questions: []bot.Question{{ID: questionID, QuestionKey: "ask_name", Text: "Name?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "name"}},
		Steps:     []bot.Step{{ID: uuid.New(), StepKey: "ask", Type: bot.StepQuestion, Title: "Ask", QuestionID: &questionID, NextStepKey: "done"}, {ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Saved {{variables.name}}."}},
	}
	fx := newRuntimeFixture(t, config)
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-reset", "start")); err != nil {
		t.Fatal(err)
	}
	reset, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-reset", "restart"))
	if err != nil {
		t.Fatal(err)
	}
	if reset.SessionStatus != SessionActive || len(reset.Messages) != 1 || reset.Messages[0].Text != "Name?" {
		t.Fatalf("expected restarted active session asking name, got %+v", reset)
	}
	session, err := fx.service.GetConversation(context.Background(), fx.actor, reset.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if session.ExpectedInput != questionID.String() || session.Variables != "{}" {
		t.Fatalf("expected reset session with empty variables, got expected_input=%s variables=%s", session.ExpectedInput, session.Variables)
	}
}

func TestWhatsAppSignatureVerificationRequiresConfiguredSecret(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	whatsapp := core.Channel{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Provider: "whatsapp", DisplayName: "WhatsApp", PhoneNumberID: "phone-id", DisplayNumber: "234", Status: core.StatusActive, Config: `{"verify_token":"verify"}`, SecretConfig: `{"app_secret":"secret"}`}
	if err := fx.db.Create(&whatsapp).Error; err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"object":"whatsapp_business_account"}`)
	if fx.service.VerifyWhatsAppRequest(whatsapp, "sha256=bad", body) {
		t.Fatal("expected bad signature to fail")
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !fx.service.VerifyWhatsAppRequest(whatsapp, signature, body) {
		t.Fatal("expected valid signature to pass")
	}
	whatsapp.SecretConfig = "{}"
	if fx.service.VerifyWhatsAppRequest(whatsapp, signature, body) {
		t.Fatal("expected missing app secret to fail closed")
	}
}

func TestRuntimeSupportsNestedModuleReturn(t *testing.T) {
	outerID := uuid.New()
	innerID := uuid.New()
	config := bot.VersionConfiguration{
		Version: bot.BotVersion{StartStepKey: "outer"},
		Modules: []bot.VersionModule{
			{ID: outerID, ModuleKey: "outer", Name: "Outer", Parameters: `{"entry_step":"inner"}`},
			{ID: innerID, ModuleKey: "inner", Name: "Inner", Parameters: `{"entry_step":"inner_done"}`},
		},
		Steps: []bot.Step{
			{ID: uuid.New(), StepKey: "outer", Type: bot.StepModule, Title: "Outer", ModuleID: &outerID, NextStepKey: "after_outer"},
			{ID: uuid.New(), StepKey: "inner", Type: bot.StepModule, Title: "Inner", ModuleID: &innerID, NextStepKey: "outer_done"},
			{ID: uuid.New(), StepKey: "inner_done", Type: bot.StepEnd, Title: "Inner done", Message: "Inner complete."},
			{ID: uuid.New(), StepKey: "outer_done", Type: bot.StepEnd, Title: "Outer done", Message: "Outer complete."},
			{ID: uuid.New(), StepKey: "after_outer", Type: bot.StepEnd, Title: "Done", Message: "All done."},
		},
	}
	fx := newRuntimeFixture(t, config)
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-modules", "hi"))
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionStatus != SessionCompleted || len(result.Messages) != 3 {
		t.Fatalf("expected nested modules to complete with three messages, got %+v", result)
	}
	want := []string{"Inner complete.", "Outer complete.", "All done."}
	for index, message := range result.Messages {
		if message.Text != want[index] {
			t.Fatalf("message %d: expected %q, got %q", index, want[index], message.Text)
		}
	}
	var events int64
	if err := fx.db.Model(&RuntimeEvent{}).Where("organization_id = ? AND event_type = ?", fx.actor.OrganizationID, EventModuleCompleted).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("expected two module completion events, got %d", events)
	}
}

func TestRuntimeDispatchesOutboundThroughRegisteredSender(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	sender := &MockChannelSender{}
	fx.service.RegisterChannelSender("test", sender)
	input := inbound(fx.channel.ID, "m1", "conv-outbound", "hi")
	result, err := fx.service.ProcessMessage(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := fx.service.DispatchOutbound(context.Background(), fx.channel, input, result)
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.Sent) != 1 || len(deliveries) != 1 || deliveries[0].Status != OutboundSent || deliveries[0].ProviderMessageID == "" {
		t.Fatalf("expected one sent outbound delivery, sent=%+v deliveries=%+v", sender.Sent, deliveries)
	}
}

func inbound(channelID uuid.UUID, messageID, conversationID, text string) InboundMessage {
	return InboundMessage{ChannelID: &channelID, ExternalMessageID: messageID, ExternalConversationID: conversationID, Sender: "customer", Text: text}
}

type actionIDs struct {
	storeQuestion    uuid.UUID
	customerQuestion uuid.UUID
	variantQuestion  uuid.UUID
	quantityQuestion uuid.UUID
	createCart       uuid.UUID
	addToCart        uuid.UUID
	createOrder      uuid.UUID
	module           uuid.UUID
}

func actionFlowIDs() actionIDs {
	return actionIDs{storeQuestion: uuid.New(), customerQuestion: uuid.New(), variantQuestion: uuid.New(), quantityQuestion: uuid.New(), createCart: uuid.New(), addToCart: uuid.New(), createOrder: uuid.New(), module: uuid.New()}
}

func commerceActionConfig(ids actionIDs) bot.VersionConfiguration {
	return bot.VersionConfiguration{
		Version: bot.BotVersion{StartStepKey: "module"},
		Modules: []bot.VersionModule{{ID: ids.module, ModuleKey: "ORDER", Name: "Order", Parameters: `{"entry_step":"ask_store"}`}},
		Questions: []bot.Question{
			{ID: ids.storeQuestion, QuestionKey: "ask_store", Text: "Store ID?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "store_id"},
			{ID: ids.customerQuestion, QuestionKey: "ask_customer", Text: "Customer ID?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "customer_id"},
			{ID: ids.variantQuestion, QuestionKey: "ask_variant", Text: "Variant ID?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "variant_id"},
			{ID: ids.quantityQuestion, QuestionKey: "ask_quantity", Text: "Quantity?", Type: "number", ResponseMode: "free_text", Required: true, VariableName: "quantity"},
		},
		Actions: []bot.Action{
			{ID: ids.createCart, ActionKey: "create_cart", ActionType: "create_cart", Name: "Create cart", InputMappings: `{"customer_id":"variables.customer_id","store_id":"variables.store_id","currency":"NGN"}`, OutputMappings: `{"cart_id":"variables.cart_id"}`},
			{ID: ids.addToCart, ActionKey: "add_to_cart", ActionType: "add_to_cart", Name: "Add to cart", InputMappings: `{"cart_id":"variables.cart_id","variant_id":"variables.variant_id","quantity":"variables.quantity"}`, OutputMappings: `{"total_minor":"variables.cart_total_minor"}`},
			{ID: ids.createOrder, ActionKey: "create_order", ActionType: "create_order", Name: "Create order", InputMappings: `{"cart_id":"variables.cart_id","customer_id":"variables.customer_id","store_id":"variables.store_id","fulfilment_type":"pickup","currency":"NGN"}`, OutputMappings: `{"order_id":"variables.order_id","order_status":"variables.order_status","total_minor":"variables.order_total_minor"}`},
		},
		Steps: []bot.Step{
			{ID: uuid.New(), StepKey: "module", Type: bot.StepModule, Title: "Order module", ModuleID: &ids.module},
			{ID: uuid.New(), StepKey: "ask_store", Type: bot.StepQuestion, Title: "Store", QuestionID: &ids.storeQuestion, NextStepKey: "ask_customer"},
			{ID: uuid.New(), StepKey: "ask_customer", Type: bot.StepQuestion, Title: "Customer", QuestionID: &ids.customerQuestion, NextStepKey: "ask_variant"},
			{ID: uuid.New(), StepKey: "ask_variant", Type: bot.StepQuestion, Title: "Variant", QuestionID: &ids.variantQuestion, NextStepKey: "ask_quantity"},
			{ID: uuid.New(), StepKey: "ask_quantity", Type: bot.StepQuestion, Title: "Quantity", QuestionID: &ids.quantityQuestion, NextStepKey: "create_cart"},
			{ID: uuid.New(), StepKey: "create_cart", Type: bot.StepAction, Title: "Create cart", ActionID: &ids.createCart, NextStepKey: "add_to_cart"},
			{ID: uuid.New(), StepKey: "add_to_cart", Type: bot.StepAction, Title: "Add cart item", ActionID: &ids.addToCart, NextStepKey: "create_order"},
			{ID: uuid.New(), StepKey: "create_order", Type: bot.StepAction, Title: "Create order", ActionID: &ids.createOrder, NextStepKey: "done"},
			{ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Order {{variables.order_id}} is {{variables.order_status}}."},
		},
	}
}

func seedCommerce(t *testing.T, fx runtimeFixture) (core.Customer, core.Store, core.Variant) {
	t.Helper()
	store, err := fx.commerce.CreateStore(context.Background(), fx.actor, core.StoreInput{Name: "Main Store", Code: "MAIN", Status: core.StatusActive, Address: "1 Commerce Road", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	category, err := fx.commerce.CreateCategory(context.Background(), fx.actor, core.CategoryInput{Name: "Drinks", Slug: "drinks", Status: core.StatusActive})
	if err != nil {
		t.Fatal(err)
	}
	product, err := fx.commerce.CreateProduct(context.Background(), fx.actor, core.ProductInput{CategoryID: &category.ID, Name: "Milkshake", Slug: "milkshake", Status: core.StatusActive, Variants: []core.VariantInput{{SKU: "MILK-REG", Name: "Regular", PriceMinor: 420000, Currency: "NGN", Status: core.StatusActive}}})
	if err != nil {
		t.Fatal(err)
	}
	customer, err := fx.commerce.CreateCustomer(context.Background(), fx.actor, core.CustomerInput{Name: "Customer", Phone: "2348000000000", Email: "customer@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.UpsertInventory(context.Background(), fx.actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: product.Variants[0].ID, OnHand: 5, ReorderThreshold: 1}); err != nil {
		t.Fatal(err)
	}
	return customer, store, product.Variants[0]
}
