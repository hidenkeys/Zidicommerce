package runtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
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
	return newRuntimeFixtureWithPaymentProvider(t, config, runtimeTestPaymentProvider{name: "paystack"})
}

func newRuntimeFixtureWithPaymentProvider(t *testing.T, config bot.VersionConfiguration, provider core.PaymentProvider) runtimeFixture {
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
		&core.PaymentConfiguration{},
		&core.PaymentProviderSecret{},
		&core.Fulfilment{},
		&core.CommerceNotification{},
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
		&bot.FAQ{},
		&bot.PublishedSnapshot{},
		&ConversationSession{},
		&ConversationMessage{},
		&ProcessedMessage{},
		&RuntimeEvent{},
		&ChannelOutboundMessage{},
		&SupportHandoff{},
		&SupportTicket{},
		&SupportHandoffNote{},
		&jobs.Job{},
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
	commerce := core.NewService(db, provider)
	return runtimeFixture{db: db, commerce: commerce, service: NewService(db, commerce, nil), actor: actor, channel: channel, bot: botRecord, version: version}
}

type runtimeTestPaymentProvider struct {
	name        string
	forceUnpaid bool
}

type failOnceRuntimePaymentProvider struct {
	mu       sync.Mutex
	attempts int
}

func (p *failOnceRuntimePaymentProvider) Name() string {
	return "paystack"
}

func (p *failOnceRuntimePaymentProvider) Initialize(_ context.Context, req core.PaymentInitializeRequest) (core.PaymentInitializeResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.attempts++
	if p.attempts == 1 {
		return core.PaymentInitializeResponse{}, errors.New("temporary provider failure")
	}
	return core.PaymentInitializeResponse{Reference: req.Reference, AuthorizationURL: "https://payments.zidicommerce.local/pay/" + req.Reference, ProviderMetadata: "{}"}, nil
}

func (p *failOnceRuntimePaymentProvider) Verify(_ context.Context, reference string) (core.PaymentVerification, error) {
	return core.PaymentVerification{Reference: reference, Paid: false, Status: "pending"}, nil
}

func (p runtimeTestPaymentProvider) Name() string {
	return p.name
}

func (p runtimeTestPaymentProvider) Initialize(_ context.Context, req core.PaymentInitializeRequest) (core.PaymentInitializeResponse, error) {
	return core.PaymentInitializeResponse{Reference: req.Reference, AuthorizationURL: "https://payments.zidicommerce.local/pay/" + req.Reference, ProviderMetadata: "{}"}, nil
}

func (p runtimeTestPaymentProvider) Verify(_ context.Context, reference string) (core.PaymentVerification, error) {
	if p.forceUnpaid {
		return core.PaymentVerification{Reference: reference, Paid: false, Status: "pending"}, nil
	}
	return core.PaymentVerification{Reference: reference, Paid: true, Status: "success"}, nil
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

func TestRuntimeExecutesGeneratedSelfServiceOrderAndTrackingFlow(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	botService := bot.NewService(fx.db)
	createdBot, err := botService.CreateSelfServiceBot(context.Background(), fx.actor, bot.SelfServiceBotInput{Name: "Acme Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := botService.ListVersions(context.Background(), fx.actor, createdBot.ID)
	if err != nil {
		t.Fatal(err)
	}
	validation, err := botService.ValidateVersion(context.Background(), fx.actor, versions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid {
		t.Fatalf("expected generated self-service bot to validate, got %+v", validation.Issues)
	}
	if _, err := botService.PublishVersion(context.Background(), fx.actor, versions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", `{"bot_id":"`+createdBot.ID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	customer, store, firstVariant := seedCommerce(t, fx)
	var firstProduct core.Product
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, firstVariant.ProductID).First(&firstProduct).Error; err != nil {
		t.Fatal(err)
	}
	secondProduct, err := fx.commerce.CreateProduct(context.Background(), fx.actor, core.ProductInput{CategoryID: firstProduct.CategoryID, Name: "Iced Tea", Slug: "iced-tea", Status: core.StatusActive, Variants: []core.VariantInput{{SKU: "TEA-REG", Name: "Regular", PriceMinor: 150000, Currency: "NGN", Status: core.StatusActive}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.UpsertInventory(context.Background(), fx.actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: secondProduct.Variants[0].ID, OnHand: 5, ReorderThreshold: 1}); err != nil {
		t.Fatal(err)
	}

	send := func(id string, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-self-service", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := send("ss-1", "start")
	if len(first.Messages) == 0 || !strings.Contains(first.Messages[0].Text, "Welcome") {
		t.Fatalf("expected welcome menu, got %+v", first.Messages)
	}
	var last RuntimeResult
	for index, text := range []string{"order", "1", "1", "2", "2", "add_more", "1", "1", "1", "review", "1", "customer@example.com"} {
		last = send("ss-order-"+strconv.Itoa(index), text)
	}
	var orders []core.Order
	if err := fx.db.Where("organization_id = ? AND customer_id = ?", fx.actor.OrganizationID, customer.ID).Find(&orders).Error; err != nil {
		t.Fatal(err)
	}
	if len(orders) == 0 {
		session, err := fx.service.GetConversation(context.Background(), fx.actor, last.ConversationID)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("expected order to be created, session step=%s expected_input=%s messages=%s variables=%s", session.CurrentStepKey, session.ExpectedInput, joinMessageTexts(last.Messages), session.Variables)
	}
	if len(orders) != 1 || orders[0].Status != core.OrderAwaitingPayment || orders[0].TotalMinor != 990000 {
		t.Fatalf("expected one awaiting-payment order, got %+v", orders)
	}
	var orderItems []core.OrderItem
	if err := fx.db.Where("organization_id = ? AND order_id = ?", fx.actor.OrganizationID, orders[0].ID).Order("created_at ASC").Find(&orderItems).Error; err != nil {
		t.Fatal(err)
	}
	if len(orderItems) != 2 {
		t.Fatalf("expected two order items, got %+v", orderItems)
	}
	var firstInventory core.InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, store.ID, firstVariant.ID).First(&firstInventory).Error; err != nil {
		t.Fatal(err)
	}
	if firstInventory.OnHand != 3 {
		t.Fatalf("expected first product inventory decremented to 3, got %d", firstInventory.OnHand)
	}
	var secondInventory core.InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, store.ID, secondProduct.Variants[0].ID).First(&secondInventory).Error; err != nil {
		t.Fatal(err)
	}
	if secondInventory.OnHand != 4 {
		t.Fatalf("expected second product inventory decremented to 4, got %d", secondInventory.OnHand)
	}
	var payments []core.Payment
	if err := fx.db.Where("organization_id = ? AND order_id = ?", fx.actor.OrganizationID, orders[0].ID).Find(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if len(payments) != 1 || !strings.Contains(payments[0].AuthorizationURL, "https://payments.zidicommerce.local/pay/") {
		t.Fatalf("expected initialized payment, got %+v", payments)
	}
	confirmed := send("ss-payment-paid", "paid")
	confirmationText := joinMessageTexts(confirmed.Messages)
	if !strings.Contains(confirmationText, "Payment confirmed") ||
		!strings.Contains(confirmationText, orders[0].OrderNumber) ||
		!strings.Contains(confirmationText, "Milkshake x2") ||
		!strings.Contains(confirmationText, "Iced Tea x1") ||
		!strings.Contains(confirmationText, "Main Store") ||
		!strings.Contains(confirmationText, "Total: NGN 9900") ||
		!strings.Contains(confirmationText, "Payment status: Paid") {
		t.Fatalf("expected authoritative payment confirmation summary, got %+v", confirmed.Messages)
	}
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, orders[0].ID).First(&orders[0]).Error; err != nil {
		t.Fatal(err)
	}
	if orders[0].Status != core.OrderPaid {
		t.Fatalf("expected paid order after customer confirmation, got %+v", orders[0])
	}
	if err := fx.db.Where("organization_id = ? AND order_id = ?", fx.actor.OrganizationID, orders[0].ID).Find(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if len(payments) != 1 || payments[0].Status != core.PaymentPaid {
		t.Fatalf("expected paid payment, got %+v", payments)
	}

	trackStart := send("ss-track-start", "start")
	if trackStart.SessionStatus != SessionActive {
		t.Fatalf("expected reset to reactivate session, got %+v", trackStart)
	}
	trackMenu := send("ss-track-menu", "track_order")
	if len(trackMenu.Messages) == 0 || !strings.Contains(joinMessageTexts(trackMenu.Messages), orders[0].OrderNumber) {
		t.Fatalf("expected recent order list, got %+v", trackMenu.Messages)
	}
	trackStatus := send("ss-track-select", "1")
	if !strings.Contains(joinMessageTexts(trackStatus.Messages), "Order confirmed") {
		t.Fatalf("expected order status message, got %+v", trackStatus.Messages)
	}
	sendOther := func(id string, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), InboundMessage{ChannelID: &fx.channel.ID, ExternalMessageID: id, ExternalConversationID: "conv-self-service-other", Sender: "other-self-service", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	otherTrack := sendOther("ss-track-other", "start")
	if otherTrack.SessionStatus != SessionActive {
		t.Fatalf("expected reset for other customer track test, got %+v", otherTrack)
	}
	otherTrack = sendOther("ss-track-other-menu", "track_order")
	if strings.Contains(joinMessageTexts(otherTrack.Messages), orders[0].OrderNumber) {
		t.Fatalf("expected other customer not to see first customer's order, got %+v", otherTrack.Messages)
	}
}

func TestRuntimePhase9BingChunPilotOrderPaymentAndTrackingFlow(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	botService := bot.NewService(fx.db)
	createdBot, err := botService.CreateSelfServiceBot(context.Background(), fx.actor, bot.SelfServiceBotInput{Name: "Bing Chun Nigeria"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := botService.ListVersions(context.Background(), fx.actor, createdBot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := botService.PublishVersion(context.Background(), fx.actor, versions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", `{"bot_id":"`+createdBot.ID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	store, err := fx.commerce.CreateStore(context.Background(), fx.actor, core.StoreInput{Name: "Lagos Test Store", Code: "LTS", Status: core.StatusActive, Address: "Lagos", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	category, err := fx.commerce.CreateCategory(context.Background(), fx.actor, core.CategoryInput{Name: "Drinks", Slug: "drinks", Status: core.StatusActive})
	if err != nil {
		t.Fatal(err)
	}
	fruitTea, err := fx.commerce.CreateProduct(context.Background(), fx.actor, core.ProductInput{CategoryID: &category.ID, Name: "Fruit Tea", Slug: "fruit-tea", Status: core.StatusActive, Variants: []core.VariantInput{{SKU: "BCNG-FRUIT-001", Name: "Regular", PriceMinor: 180000, Currency: "NGN", Status: core.StatusActive}}})
	if err != nil {
		t.Fatal(err)
	}
	milkTea, err := fx.commerce.CreateProduct(context.Background(), fx.actor, core.ProductInput{CategoryID: &category.ID, Name: "Milk Tea", Slug: "milk-tea", Status: core.StatusActive, Variants: []core.VariantInput{{SKU: "BCNG-MILK-001", Name: "Regular", PriceMinor: 150000, Currency: "NGN", Status: core.StatusActive}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.UpsertInventory(context.Background(), fx.actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: milkTea.Variants[0].ID, OnHand: 10, ReorderThreshold: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.UpsertInventory(context.Background(), fx.actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: fruitTea.Variants[0].ID, OnHand: 5, ReorderThreshold: 1}); err != nil {
		t.Fatal(err)
	}

	send := func(id string, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), InboundMessage{ChannelID: &fx.channel.ID, ExternalMessageID: id, ExternalConversationID: "2348000000000", Sender: "2348000000000", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	send("bc-1", "start")
	for index, text := range []string{"order", "1", "1", "1", "2", "add_more", "1", "2", "1", "review", "1", "pilot@example.com"} {
		send("bc-order-"+strconv.Itoa(index), text)
	}
	var customer core.Customer
	if err := fx.db.Where("organization_id = ? AND phone = ?", fx.actor.OrganizationID, "2348000000000").First(&customer).Error; err != nil {
		t.Fatal(err)
	}
	var orders []core.Order
	if err := fx.db.Where("organization_id = ? AND customer_id = ?", fx.actor.OrganizationID, customer.ID).Find(&orders).Error; err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].TotalMinor != 480000 || orders[0].Status != core.OrderAwaitingPayment {
		t.Fatalf("expected one NGN 4800 awaiting-payment order, got %+v", orders)
	}
	var createdEvent core.OrderEvent
	if err := fx.db.Where("organization_id = ? AND order_id = ? AND event_type = ?", fx.actor.OrganizationID, orders[0].ID, "order_created").First(&createdEvent).Error; err != nil {
		t.Fatal(err)
	}
	if createdEvent.ActorUserID != nil {
		t.Fatalf("expected bot-created order event to have no user actor, got %s", createdEvent.ActorUserID.String())
	}
	paid := send("bc-paid", "paid")
	if !strings.Contains(joinMessageTexts(paid.Messages), "Payment confirmed") {
		t.Fatalf("expected payment confirmation, got %+v", paid.Messages)
	}
	var milkInventory core.InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, store.ID, milkTea.Variants[0].ID).First(&milkInventory).Error; err != nil {
		t.Fatal(err)
	}
	if milkInventory.OnHand != 8 {
		t.Fatalf("expected milk tea inventory 8, got %d", milkInventory.OnHand)
	}
	var fruitInventory core.InventoryLevel
	if err := fx.db.Where("organization_id = ? AND store_id = ? AND variant_id = ?", fx.actor.OrganizationID, store.ID, fruitTea.Variants[0].ID).First(&fruitInventory).Error; err != nil {
		t.Fatal(err)
	}
	if fruitInventory.OnHand != 4 {
		t.Fatalf("expected fruit tea inventory 4, got %d", fruitInventory.OnHand)
	}
	send("bc-track-start", "start")
	track := send("bc-track-menu", "Track order")
	if !strings.Contains(joinMessageTexts(track.Messages), orders[0].OrderNumber) {
		t.Fatalf("expected customer to track own order, got %+v", track.Messages)
	}
	other, err := fx.service.ProcessMessage(context.Background(), InboundMessage{ChannelID: &fx.channel.ID, ExternalMessageID: "bc-other", ExternalConversationID: "2348000000001", Sender: "2348000000001", Text: "Track order"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(joinMessageTexts(other.Messages), orders[0].OrderNumber) {
		t.Fatalf("expected second customer isolation, got %+v", other.Messages)
	}
}

func TestRuntimeEntryMenuUsesEnabledModulesInSnapshotOrder(t *testing.T) {
	config := moduleMenuConfig([]bot.VersionModule{
		moduleForMenu("ORDER", "Place order", "order", 10, true),
		moduleForMenu("TRACK_ORDER", "Track order", "track_order", 30, true),
		moduleForMenu("FAQ", "Ask a question", "faq", 20, true),
		moduleForMenu("COMPLAINT", "Report a complaint", "complaint", 40, false),
	})
	fx := newRuntimeFixture(t, config)
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "menu-1", "conv-menu", "start"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected one menu message, got %+v", result.Messages)
	}
	got := optionLabels(result.Messages[0].Options)
	want := []string{"Place order", "Ask a question", "Track order"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("expected enabled module menu order %v, got %v", want, got)
	}
}

func TestRuntimeDisabledModuleCannotBeReachedFromEntryMenu(t *testing.T) {
	config := moduleMenuConfig([]bot.VersionModule{
		moduleForMenu("ORDER", "Place order", "order", 10, true),
		moduleForMenu("FAQ", "Ask a question", "faq", 20, false),
	})
	fx := newRuntimeFixture(t, config)
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "disabled-1", "conv-disabled-menu", "start")); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "disabled-2", "conv-disabled-menu", "faq"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 || !strings.Contains(result.Messages[0].Text, "available options") {
		t.Fatalf("expected disabled module option to be rejected at entry, got %+v", result.Messages)
	}
	var session ConversationSession
	if err := fx.db.Where("id = ?", result.ConversationID).First(&session).Error; err != nil {
		t.Fatal(err)
	}
	if session.CurrentStepKey != "start" || session.ExpectedInput == "" {
		t.Fatalf("expected session to remain at entry menu, got step=%s expected=%s", session.CurrentStepKey, session.ExpectedInput)
	}
}

func TestRuntimePublishedModuleSnapshotIsImmutableAfterDraftEdit(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	botService := bot.NewService(fx.db)
	createdBot, err := botService.CreateSelfServiceBot(context.Background(), fx.actor, bot.SelfServiceBotInput{Name: "Acme Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := botService.ListVersions(context.Background(), fx.actor, createdBot.ID)
	if err != nil {
		t.Fatal(err)
	}
	draftID := versions[0].ID
	modules, err := botService.ListModules(context.Background(), fx.actor, draftID)
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range modules {
		if module.ModuleKey == "FAQ" {
			if _, err := botService.UpdateModule(context.Background(), fx.actor, module.ID, bot.ModuleInput{SortOrder: 25}); err != nil {
				t.Fatal(err)
			}
		}
		if module.ModuleKey == "COMPLAINT" {
			if _, err := botService.SetModuleEnabled(context.Background(), fx.actor, module.ID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := botService.PublishVersion(context.Background(), fx.actor, draftID); err != nil {
		t.Fatal(err)
	}
	nextDraft, err := botService.CreateVersion(context.Background(), fx.actor, createdBot.ID, bot.VersionInput{SourceVersionID: &draftID})
	if err != nil {
		t.Fatal(err)
	}
	nextModules, err := botService.ListModules(context.Background(), fx.actor, nextDraft.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range nextModules {
		if module.ModuleKey == "FAQ" {
			if _, err := botService.UpdateModule(context.Background(), fx.actor, module.ID, bot.ModuleInput{SortOrder: 90}); err != nil {
				t.Fatal(err)
			}
		}
		if module.ModuleKey == "COMPLAINT" {
			if _, err := botService.SetModuleEnabled(context.Background(), fx.actor, module.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", `{"bot_id":"`+createdBot.ID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "immutable-1", "conv-immutable-menu", "start"))
	if err != nil {
		t.Fatal(err)
	}
	labels := optionLabels(result.Messages[0].Options)
	if strings.Contains(strings.Join(labels, "|"), "Report a complaint") {
		t.Fatalf("expected published snapshot to keep complaint disabled after draft edit, got %v", labels)
	}
	if len(labels) < 3 || strings.Join(labels[:3], "|") != "Place order|Ask a question|Track order" {
		t.Fatalf("expected published order to remain immutable, got %v", labels)
	}
}

func TestRuntimeTenantModuleMenusAreIsolated(t *testing.T) {
	config := moduleMenuConfig([]bot.VersionModule{
		moduleForMenu("ORDER", "Tenant A order", "order", 10, true),
		moduleForMenu("FAQ", "Tenant A FAQ", "faq", 20, true),
	})
	fx := newRuntimeFixture(t, config)
	otherOrg := uuid.New()
	otherBotID := uuid.New()
	otherVersionID := uuid.New()
	otherQuestionID := uuid.New()
	otherConfig := moduleMenuConfig([]bot.VersionModule{
		moduleForMenu("ORDER", "Tenant B order", "order", 10, true),
		moduleForMenu("FAQ", "Tenant B FAQ", "faq", 20, false),
	})
	otherConfig.Version = bot.BotVersion{ID: otherVersionID, OrganizationID: otherOrg, BotID: otherBotID, VersionNumber: 1, Status: bot.VersionStatusPublished, StartStepKey: "start", ValidationErrors: "[]", Metadata: "{}"}
	for index := range otherConfig.Questions {
		otherConfig.Questions[index].ID = otherQuestionID
	}
	for index := range otherConfig.Steps {
		if otherConfig.Steps[index].StepKey == "start" {
			otherConfig.Steps[index].QuestionID = &otherQuestionID
		}
	}
	raw, err := json.Marshal(otherConfig)
	if err != nil {
		t.Fatal(err)
	}
	otherChannel := core.Channel{ID: uuid.New(), OrganizationID: otherOrg, Provider: "test", DisplayName: "Other", PhoneNumberID: "other-phone", DisplayNumber: "other", Status: core.StatusActive, Config: `{"bot_id":"` + otherBotID.String() + `"}`, SecretConfig: "{}"}
	if err := fx.db.Create(&organization.Organization{ID: otherOrg, Name: "Other Merchant", Slug: "other", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&bot.Bot{ID: otherBotID, OrganizationID: otherOrg, Name: "Other Bot", Status: bot.BotStatusActive, DefaultLanguage: "en", Timezone: "Africa/Lagos", PublishedVersionID: &otherVersionID, FallbackConfig: "{}", HandoffConfig: "{}", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&bot.BotVersion{ID: otherVersionID, OrganizationID: otherOrg, BotID: otherBotID, VersionNumber: 1, Status: bot.VersionStatusPublished, StartStepKey: "start", ValidationErrors: "[]", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&bot.PublishedSnapshot{ID: uuid.New(), OrganizationID: otherOrg, BotID: otherBotID, VersionID: otherVersionID, VersionNumber: 1, Snapshot: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&otherChannel).Error; err != nil {
		t.Fatal(err)
	}
	tenantA, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "tenant-a", "conv-tenant-a", "start"))
	if err != nil {
		t.Fatal(err)
	}
	tenantB, err := fx.service.ProcessMessage(context.Background(), inbound(otherChannel.ID, "tenant-b", "conv-tenant-b", "start"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(optionLabels(tenantA.Messages[0].Options), "|"), "Tenant B") {
		t.Fatalf("tenant A menu leaked tenant B data: %+v", tenantA.Messages[0].Options)
	}
	if strings.Contains(strings.Join(optionLabels(tenantB.Messages[0].Options), "|"), "Tenant A") || strings.Contains(strings.Join(optionLabels(tenantB.Messages[0].Options), "|"), "FAQ") {
		t.Fatalf("tenant B menu leaked tenant A or disabled data: %+v", tenantB.Messages[0].Options)
	}
}

func TestRuntimeGeneratedOrderPaymentFailureCanCancel(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixtureWithPaymentProvider(t, config, runtimeTestPaymentProvider{name: "paystack", forceUnpaid: true})
	botService := bot.NewService(fx.db)
	createdBot, err := botService.CreateSelfServiceBot(context.Background(), fx.actor, bot.SelfServiceBotInput{Name: "Acme Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := botService.ListVersions(context.Background(), fx.actor, createdBot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := botService.PublishVersion(context.Background(), fx.actor, versions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", `{"bot_id":"`+createdBot.ID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	customer, _, _ := seedCommerce(t, fx)

	send := func(id string, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-payment-failure", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	send("pf-start", "start")
	for index, text := range []string{"order", "1", "1", "1", "1", "review", "1", "customer@example.com"} {
		send("pf-order-"+strconv.Itoa(index), text)
	}
	paidAttempt := send("pf-paid", "paid")
	if !strings.Contains(joinMessageTexts(paidAttempt.Messages), "not been completed") {
		t.Fatalf("expected payment failure recovery message, got %+v", paidAttempt.Messages)
	}
	var order core.Order
	if err := fx.db.Where("organization_id = ? AND customer_id = ?", fx.actor.OrganizationID, customer.ID).First(&order).Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != core.OrderAwaitingPayment {
		t.Fatalf("expected order to remain awaiting payment, got %+v", order)
	}
	cancelled := send("pf-cancel", "cancel")
	if !strings.Contains(joinMessageTexts(cancelled.Messages), "cancelled") {
		t.Fatalf("expected cancellation message, got %+v", cancelled.Messages)
	}
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, order.ID).First(&order).Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != core.OrderCancelled {
		t.Fatalf("expected cancelled order, got %+v", order)
	}
}

func TestRuntimePersistsOrderOutputsWhenPaymentInitializationFails(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	provider := &failOnceRuntimePaymentProvider{}
	fx := newRuntimeFixtureWithPaymentProvider(t, config, provider)
	botService := bot.NewService(fx.db)
	createdBot, err := botService.CreateSelfServiceBot(context.Background(), fx.actor, bot.SelfServiceBotInput{Name: "Acme Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := botService.ListVersions(context.Background(), fx.actor, createdBot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := botService.PublishVersion(context.Background(), fx.actor, versions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", `{"bot_id":"`+createdBot.ID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	seedCommerce(t, fx)

	send := func(id string, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-payment-retry", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	send("pr-start", "start")
	var failed RuntimeResult
	for index, text := range []string{"order", "1", "1", "1", "1", "review", "1", "customer@example.com"} {
		failed = send("pr-order-"+strconv.Itoa(index), text)
	}
	if !strings.Contains(joinMessageTexts(failed.Messages), "could not initialize payment") {
		t.Fatalf("expected the first payment initialization to fail, got %+v", failed.Messages)
	}

	var session ConversationSession
	if err := fx.db.Where("external_conversation_id = ?", "conv-payment-retry").First(&session).Error; err != nil {
		t.Fatal(err)
	}
	variables := parseJSONMap(session.Variables)
	if stringValue(variables["order_id"]) == "" {
		t.Fatalf("expected the successfully created order ID to be persisted, got %s", session.Variables)
	}

	retried := send("pr-retry", "retry")
	if !strings.Contains(joinMessageTexts(retried.Messages), "https://payments.zidicommerce.local/pay/") {
		t.Fatalf("expected payment initialization retry to return a link, got %+v", retried.Messages)
	}
	provider.mu.Lock()
	attempts := provider.attempts
	provider.mu.Unlock()
	if attempts != 2 {
		t.Fatalf("expected exactly two payment initialization attempts, got %d", attempts)
	}
}

func TestRuntimeFAQActionReturnsConfiguredTenantAnswer(t *testing.T) {
	questionID := uuid.New()
	actionID := uuid.New()
	config := bot.VersionConfiguration{
		Version:   bot.BotVersion{StartStepKey: "ask_faq"},
		Questions: []bot.Question{{ID: questionID, QuestionKey: "faq_query", Text: "What would you like to know?", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "faq_query"}},
		Actions:   []bot.Action{{ID: actionID, ActionKey: "answer_faq", ActionType: "match_faq", Name: "Answer FAQ", InputMappings: `{"query":"faq_query"}`, OutputMappings: `{"answer":"variables.faq_answer"}`}},
		Steps: []bot.Step{
			{ID: uuid.New(), StepKey: "ask_faq", Type: bot.StepQuestion, Title: "Ask FAQ", QuestionID: &questionID, NextStepKey: "answer_faq"},
			{ID: uuid.New(), StepKey: "answer_faq", Type: bot.StepAction, Title: "Answer FAQ", ActionID: &actionID, NextStepKey: "done"},
			{ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Done."},
		},
	}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Create(&bot.FAQ{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Question: "What time do you open?", Answer: "We open by 10am.", Keywords: `["opening hours"]`, Status: core.StatusActive, Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-faq", "start")); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m2", "conv-faq", "opening hours"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) < 1 || result.Messages[0].Text != "We open by 10am." {
		t.Fatalf("expected configured FAQ answer, got %+v", result.Messages)
	}
	session, err := fx.service.GetConversation(context.Background(), fx.actor, result.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(session.Variables, "We open by 10am.") {
		t.Fatalf("expected FAQ answer to be persisted, got %s", session.Variables)
	}
}

func TestRuntimeFAQActionIgnoresDisabledFAQ(t *testing.T) {
	actionID := uuid.New()
	config := bot.VersionConfiguration{
		Version: bot.BotVersion{StartStepKey: "answer_faq"},
		Actions: []bot.Action{{ID: actionID, ActionKey: "answer_faq", ActionType: "match_faq", Name: "Answer FAQ", InputMappings: `{"query":"opening hours"}`, OutputMappings: `{"answer":"variables.faq_answer"}`}},
		Steps:   []bot.Step{{ID: uuid.New(), StepKey: "answer_faq", Type: bot.StepAction, Title: "Answer FAQ", ActionID: &actionID}},
	}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Create(&bot.FAQ{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Question: "What time do you open?", Answer: "We open by 10am.", Keywords: `["opening hours"]`, Status: core.StatusInactive, Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "m1", "conv-faq-disabled", "start"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 || !strings.Contains(result.Messages[0].Text, "do not have a configured answer") {
		t.Fatalf("expected disabled FAQ to be ignored, got %+v", result.Messages)
	}
}

func TestRuntimeCommerceActionsRejectOtherCustomerOrderAndCart(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	currentCustomer, store, variant := seedCommerce(t, fx)
	otherCustomer, err := fx.commerce.CreateCustomer(context.Background(), fx.actor, core.CustomerInput{Name: "Other", Phone: "2348000000001", Email: "other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	cart, err := fx.commerce.CreateCart(context.Background(), fx.actor, core.CartInput{CustomerID: otherCustomer.ID, StoreID: store.ID, Currency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.AddCartItem(context.Background(), fx.actor, cart.ID, core.CartItemInput{VariantID: variant.ID, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	order, err := fx.commerce.CreateOrder(context.Background(), fx.actor, core.OrderInput{CartID: &cart.ID, CustomerID: otherCustomer.ID, StoreID: store.ID, FulfilmentType: core.FulfilmentPickup, Currency: "NGN", IdempotencyKey: "other-order"})
	if err != nil {
		t.Fatal(err)
	}
	session := ConversationSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: fx.channel.ID, BotID: fx.bot.ID, BotVersionID: fx.version.ID, CustomerID: &currentCustomer.ID, Status: SessionActive}
	runtimeContext := RuntimeContext{Session: session, Variables: map[string]any{}, System: map[string]any{}}
	if _, err := fx.service.actions.Execute(context.Background(), bot.Action{ActionType: "get_order", InputMappings: `{"order_id":"` + order.ID.String() + `"}`, OutputMappings: "{}"}, runtimeContext); err == nil {
		t.Fatal("expected get_order to reject another customer's order")
	}
	if _, err := fx.service.actions.Execute(context.Background(), bot.Action{ActionType: "add_to_cart", InputMappings: `{"cart_id":"` + cart.ID.String() + `","variant_id":"` + variant.ID.String() + `","quantity":"1"}`, OutputMappings: "{}"}, runtimeContext); err == nil {
		t.Fatal("expected add_to_cart to reject another customer's cart")
	}
}

func TestRuntimeCustomerSensitiveActionsFailClosedWithoutSessionCustomer(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	customer, store, variant := seedCommerce(t, fx)
	session := ConversationSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: fx.channel.ID, BotID: fx.bot.ID, BotVersionID: fx.version.ID, Status: SessionActive}
	runtimeContext := RuntimeContext{Session: session, Variables: map[string]any{}, System: map[string]any{}}
	cartID := uuid.New()
	orderID := uuid.New()
	actions := []bot.Action{
		{ActionType: "create_cart", InputMappings: `{"customer_id":"` + customer.ID.String() + `","store_id":"` + store.ID.String() + `","currency":"NGN"}`, OutputMappings: "{}"},
		{ActionType: "add_to_cart", InputMappings: `{"cart_id":"` + cartID.String() + `","variant_id":"` + variant.ID.String() + `","quantity":"1"}`, OutputMappings: "{}"},
		{ActionType: "create_order", InputMappings: `{"cart_id":"` + cartID.String() + `","customer_id":"` + customer.ID.String() + `","store_id":"` + store.ID.String() + `","fulfilment_type":"pickup","currency":"NGN"}`, OutputMappings: "{}"},
		{ActionType: "initialize_payment", InputMappings: `{"order_id":"` + orderID.String() + `","provider":"paystack","email":"customer@example.com"}`, OutputMappings: "{}"},
		{ActionType: "check_payment", InputMappings: `{"payment_reference":"ref_missing_customer"}`, OutputMappings: "{}"},
		{ActionType: "get_order", InputMappings: `{"order_id":"` + orderID.String() + `"}`, OutputMappings: "{}"},
		{ActionType: "get_order_status", InputMappings: `{"order_id":"` + orderID.String() + `"}`, OutputMappings: "{}"},
		{ActionType: "get_customer_orders", InputMappings: `{"customer_id":"` + customer.ID.String() + `"}`, OutputMappings: "{}"},
	}
	for _, action := range actions {
		if _, err := fx.service.actions.Execute(context.Background(), action, runtimeContext); err == nil {
			t.Fatalf("expected %s to fail closed without session customer", action.ActionType)
		}
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
	var handoffs int64
	if err := fx.db.Model(&SupportHandoff{}).Where("organization_id = ? AND session_id = ? AND status = ?", fx.actor.OrganizationID, result.ConversationID, "open").Count(&handoffs).Error; err != nil {
		t.Fatal(err)
	}
	if handoffs != 1 {
		t.Fatalf("expected one open support handoff, got %d", handoffs)
	}
	var handoff SupportHandoff
	if err := fx.db.Where("organization_id = ? AND session_id = ?", fx.actor.OrganizationID, result.ConversationID).First(&handoff).Error; err != nil {
		t.Fatal(err)
	}
	resolved, err := fx.service.ResolveSupportHandoff(context.Background(), fx.actor, handoff.ID, SupportHandoffResolveInput{ResolutionNote: "Handled"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("expected resolved handoff, got %+v", resolved)
	}
}

func TestRuntimeComplaintCreatesSupportTicketAndHandoff(t *testing.T) {
	questionID := uuid.New()
	actionID := uuid.New()
	config := bot.VersionConfiguration{
		Version:   bot.BotVersion{StartStepKey: "ask_complaint"},
		Questions: []bot.Question{{ID: questionID, QuestionKey: "complaint_text", Text: "Tell us what happened.", Type: "text", ResponseMode: "free_text", Required: true, VariableName: "complaint_text"}},
		Actions:   []bot.Action{{ID: actionID, ActionKey: "create_complaint", ActionType: "create_complaint", Name: "Create complaint", InputMappings: `{"message":"variables.complaint_text"}`, OutputMappings: `{"support_ticket_id":"variables.support_ticket_id"}`}},
		Steps: []bot.Step{
			{ID: uuid.New(), StepKey: "ask_complaint", Type: bot.StepQuestion, Title: "Complaint", QuestionID: &questionID, NextStepKey: "create_complaint"},
			{ID: uuid.New(), StepKey: "create_complaint", Type: bot.StepAction, Title: "Create complaint", ActionID: &actionID, NextStepKey: "done"},
			{ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Complaint {{variables.support_ticket_id}} recorded."},
		},
	}
	fx := newRuntimeFixture(t, config)
	if _, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "cmp-1", "conv-complaint", "start")); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "cmp-2", "conv-complaint", "The drink spilled"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Handoff || result.SessionStatus != SessionHandoff {
		t.Fatalf("expected complaint to trigger handoff, got %+v", result)
	}
	var ticket SupportTicket
	if err := fx.db.Where("organization_id = ? AND session_id = ?", fx.actor.OrganizationID, result.ConversationID).First(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.CustomerID == nil || ticket.TicketType != "complaint" || !strings.Contains(ticket.Description, "spilled") {
		t.Fatalf("expected persisted complaint ticket, got %+v", ticket)
	}
	var handoff SupportHandoff
	if err := fx.db.Where("organization_id = ? AND session_id = ?", fx.actor.OrganizationID, result.ConversationID).First(&handoff).Error; err != nil {
		t.Fatal(err)
	}
	claimed, err := fx.service.ClaimSupportHandoff(context.Background(), fx.actor, handoff.ID, SupportHandoffClaimInput{Note: "Taking this"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != "assigned" || claimed.AssignedUserID == nil || *claimed.AssignedUserID != fx.actor.ID {
		t.Fatalf("expected claimed handoff, got %+v", claimed)
	}
	note, err := fx.service.AddSupportHandoffNote(context.Background(), fx.actor, handoff.ID, SupportHandoffNoteInput{Note: "Customer needs replacement", Internal: true})
	if err != nil {
		t.Fatal(err)
	}
	if note.ID == uuid.Nil || !note.Internal {
		t.Fatalf("expected internal note, got %+v", note)
	}
	notes, err := fx.service.ListSupportHandoffNotes(context.Background(), fx.actor, handoff.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected claim note and manual note, got %+v", notes)
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

func TestRuntimeQueuesOutboundAndWorkerSendsIt(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	jobService := jobs.NewService(fx.db, nil)
	fx.service.ConfigureJobs(jobService)
	jobService.Register(jobs.JobTypeChannelOutbound, fx.service.ProcessOutboundJob)
	sender := &MockChannelSender{}
	fx.service.RegisterChannelSender("test", sender)
	input := inbound(fx.channel.ID, "m1", "conv-worker-outbound", "hi")
	result, err := fx.service.ProcessMessage(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := fx.service.DispatchOutbound(context.Background(), fx.channel, input, result)
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.Sent) != 0 || len(deliveries) != 1 || deliveries[0].Status != OutboundQueued {
		t.Fatalf("expected one queued outbound delivery before worker, sent=%+v deliveries=%+v", sender.Sent, deliveries)
	}
	processed, err := jobService.ProcessDue(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || len(sender.Sent) != 1 {
		t.Fatalf("expected worker to send one delivery, processed=%d sent=%+v", processed, sender.Sent)
	}
	var updated ChannelOutboundMessage
	if err := fx.db.Where("id = ?", deliveries[0].ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != OutboundSent || updated.ProviderMessageID == "" {
		t.Fatalf("expected outbound sent after worker, got %+v", updated)
	}
}

func TestRuntimeDispatchOutboundIsIdempotentForDuplicateWebhookResult(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	sender := &MockChannelSender{}
	fx.service.RegisterChannelSender("test", sender)
	input := inbound(fx.channel.ID, "same-webhook", "conv-duplicate-webhook", "hi")
	result, err := fx.service.ProcessMessage(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.DispatchOutbound(context.Background(), fx.channel, input, result); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.DispatchOutbound(context.Background(), fx.channel, input, result); err != nil {
		t.Fatal(err)
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("expected one provider send for duplicate dispatch, got %d", len(sender.Sent))
	}
	var deliveries int64
	if err := fx.db.Model(&ChannelOutboundMessage{}).Where("organization_id = ? AND idempotency_key <> ''", fx.actor.OrganizationID).Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 {
		t.Fatalf("expected one outbound delivery row, got %d", deliveries)
	}
}

func TestRuntimeDuplicateOutboundJobsSendOnce(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	sender := &MockChannelSender{Delay: 25 * time.Millisecond}
	fx.service.RegisterChannelSender("test", sender)
	delivery := ChannelOutboundMessage{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: fx.channel.ID, Recipient: "customer", Provider: "test", MessageType: MessageText, Status: OutboundQueued, Payload: jsonValue(map[string]any{"to": "customer", "type": MessageText, "text": "Hello"}), ProviderResponse: "{}"}
	if err := fx.db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{ID: uuid.New(), JobType: jobs.JobTypeChannelOutbound, Payload: jsonValue(map[string]any{"outbound_message_id": delivery.ID.String()}), MaxAttempts: 3}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = fx.service.ProcessOutboundJob(context.Background(), job)
	}()
	go func() {
		defer wg.Done()
		_ = fx.service.ProcessOutboundJob(context.Background(), job)
	}()
	wg.Wait()
	if len(sender.Sent) != 1 {
		t.Fatalf("expected duplicate outbound jobs to send once, got %d sends", len(sender.Sent))
	}
}

func TestRuntimeNotificationPermanentFailureIsRecorded(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	sender := &MockChannelSender{Errors: []error{errors.New("provider unavailable")}}
	fx.service.RegisterChannelSender("test", sender)
	channelID := fx.channel.ID
	notification := core.CommerceNotification{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: &channelID, NotificationType: "order_paid", Recipient: "customer", Status: "queued", Payload: jsonValue(map[string]any{"message": "Paid"})}
	if err := fx.db.Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{ID: uuid.New(), JobType: jobs.JobTypeNotificationDelivery, Payload: jsonValue(map[string]any{"notification_id": notification.ID.String()}), MaxAttempts: 1}
	if err := fx.service.ProcessNotificationJob(context.Background(), job); err == nil {
		t.Fatal("expected notification send failure")
	}
	var updated core.CommerceNotification
	if err := fx.db.Where("id = ?", notification.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "failed_permanently" {
		t.Fatalf("expected failed_permanently notification, got %+v", updated)
	}
}

func inbound(channelID uuid.UUID, messageID, conversationID, text string) InboundMessage {
	return InboundMessage{ChannelID: &channelID, ExternalMessageID: messageID, ExternalConversationID: conversationID, Sender: "customer", Text: text}
}

func moduleMenuConfig(modules []bot.VersionModule) bot.VersionConfiguration {
	questionID := uuid.New()
	return bot.VersionConfiguration{
		Version: bot.BotVersion{StartStepKey: "start"},
		Modules: modules,
		Questions: []bot.Question{{
			ID:           questionID,
			QuestionKey:  "main_menu",
			Text:         "How can we help?",
			Type:         "single_choice",
			ResponseMode: "list",
			Required:     true,
			VariableName: "intent",
			Options:      jsonValue([]map[string]string{{"id": "order", "label": "Static order"}, {"id": "faq", "label": "Static FAQ"}, {"id": "complaint", "label": "Static complaint"}, {"id": "track_order", "label": "Static track"}}),
		}},
		Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepChoice, Title: "Main menu", QuestionID: &questionID, NextStepKey: "done"}, {ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Done."}},
	}
}

func moduleForMenu(key, label, intent string, sortOrder int, enabled bool) bot.VersionModule {
	return bot.VersionModule{ID: uuid.New(), ModuleKey: key, Name: label, Source: bot.ModuleSourceSystem, Parameters: jsonValue(map[string]any{"menu_intent": intent, "menu_label": label}), Metadata: jsonValue(map[string]any{"enabled": enabled}), SortOrder: sortOrder}
}

func optionLabels(options []MessageOption) []string {
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	return labels
}

func joinMessageTexts(messages []OutboundMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Text)
	}
	return strings.Join(parts, "\n")
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
	customer, err := fx.commerce.CreateCustomer(context.Background(), fx.actor, core.CustomerInput{Name: "Customer", Phone: "customer", Email: "customer@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.UpsertInventory(context.Background(), fx.actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: product.Variants[0].ID, OnHand: 5, ReorderThreshold: 1}); err != nil {
		t.Fatal(err)
	}
	return customer, store, product.Variants[0]
}
