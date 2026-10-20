package runtime

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"strings"
	"testing"
	"time"
)

func journeyFixture(t *testing.T, category string) (runtimeFixture, bot.JourneyDetail) {
	t.Helper()
	fx := newRuntimeFixtureWithPaymentProvider(t, bot.VersionConfiguration{}, runtimeTestPaymentProvider{name: "paystack", forceUnpaid: true})
	if err := fx.db.AutoMigrate(&bot.JourneyBinding{}, &bot.JourneyFlowDeployment{}, &JourneyPreviewSession{}, &JourneyFlowSession{}, &JourneyExchangeReceipt{}, &JourneyExecutionLease{}); err != nil {
		t.Fatal(err)
	}
	fx.service.RegisterChannelSender("test", &MockChannelSender{})
	detail, err := bot.NewService(fx.db).CreateJourney(context.Background(), fx.actor, category, "Test journey")
	if err != nil {
		t.Fatal(err)
	}
	d := detail.Definition
	d.Channel = "test"
	d.ConnectionID = &fx.channel.ID
	detail, err = bot.NewService(fx.db).SaveJourney(context.Background(), fx.actor, detail.Version.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	return fx, detail
}
func journeyPreviewAnswer(t *testing.T, fx runtimeFixture, p JourneyPreviewResult, text string) JourneyPreviewResult {
	t.Helper()
	next, err := fx.service.AnswerJourneyPreview(context.Background(), fx.actor, p.SessionID, JourneyAnswer{EventID: uuid.NewString(), Turn: p.Turn, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func TestJourneyPreviewSameInterpreterNoWritesAndFreshness(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	_, _, variant := seedCommerce(t, fx)
	ctx := context.Background()
	p, err := fx.service.StartJourneyPreview(ctx, fx.actor, detail.Version.ID, "normal")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"1", "1", "1", "1", "2", "2", "1", "Ada", "+2348010000001", "ada@example.test"} {
		p = journeyPreviewAnswer(t, fx, p, text)
	}
	if p.CurrentStep != "confirm" || len(p.Cart) != 1 {
		t.Fatalf("wrong path: %+v", p)
	}
	if err := fx.db.Model(&core.Variant{}).Where("id = ?", variant.ID).Update("price_minor", 999000).Error; err != nil {
		t.Fatal(err)
	}
	// The preview cart review reads current prices rather than the config/model.
	p = journeyPreviewAnswer(t, fx, p, "1")
	if p.CurrentStep != "payment_choice" {
		t.Fatalf("confirmation: %+v", p)
	}
	p = journeyPreviewAnswer(t, fx, p, "1")
	if p.Result.SessionStatus != SessionCompleted {
		t.Fatalf("finish: %+v", p)
	}
	for _, model := range []any{&core.Cart{}, &core.Order{}, &core.Payment{}, &core.CommerceNotification{}, &SupportTicket{}, &SupportHandoff{}} {
		var count int64
		if err := fx.db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("preview wrote %T: %d %v", model, count, err)
		}
	}
}
func TestJourneyPreviewTenantActorStaleReplayAndResume(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	seedCommerce(t, fx)
	ctx := context.Background()
	p, err := fx.service.StartJourneyPreview(ctx, fx.actor, detail.Version.ID, "normal")
	if err != nil {
		t.Fatal(err)
	}
	input := JourneyAnswer{EventID: "same", Turn: p.Turn, Text: "1"}
	p2, err := fx.service.AnswerJourneyPreview(ctx, fx.actor, p.SessionID, input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := NewService(fx.db, fx.commerce, nil).AnswerJourneyPreview(ctx, fx.actor, p.SessionID, input)
	if err != nil || jsonValue(again) != jsonValue(p2) {
		t.Fatal("replay or restart changed result", err)
	}
	input.EventID = "stale"
	if _, err := fx.service.AnswerJourneyPreview(ctx, fx.actor, p.SessionID, input); err == nil {
		t.Fatal("stale turn")
	}
	for _, actor := range []auth.CurrentUser{{ID: fx.actor.ID, OrganizationID: uuid.New(), Role: authz.MerchantAdmin}, {ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.MerchantAdmin}} {
		if _, err := fx.service.AnswerJourneyPreview(ctx, actor, p.SessionID, JourneyAnswer{EventID: "other", Turn: p2.Turn, Text: "1"}); err == nil {
			t.Fatal("preview identity leak")
		}
	}
	fx.db.Model(&JourneyPreviewSession{}).Where("id = ?", p.SessionID).Update("expires_at", time.Now().Add(-time.Minute))
	if _, err := fx.service.AnswerJourneyPreview(ctx, fx.actor, p.SessionID, JourneyAnswer{EventID: "expired", Turn: p2.Turn, Text: "1"}); err == nil {
		t.Fatal("expired")
	}
}
func TestJourneyLivePinnedIdempotentOrderAndPaymentSafety(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	customer, _, _ := seedCommerce(t, fx)
	ctx := context.Background()
	service := bot.NewService(fx.db)
	published, err := service.PublishJourney(ctx, fx.actor, detail.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", jsonValue(map[string]any{"bot_id": detail.Bot.ID.String()}))
	input := InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "journey-live", Text: "order", ExternalMessageID: "initial"}
	result, err := fx.service.ProcessMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var session ConversationSession
	fx.db.First(&session, "id = ?", result.ConversationID)
	clone, err := service.CloneJourneyDraft(ctx, fx.actor, detail.Bot.ID, published.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishJourney(ctx, fx.actor, clone.Version.ID); err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"1", "1", "1", "1", "2", "2", "1", "Ada", "+2348010000001", "ada@example.test", "1"} {
		input.Text = text
		input.ExternalMessageID = uuid.NewString()
		result, err = fx.service.ProcessMessage(ctx, input)
		if err != nil || result.Error != nil {
			t.Fatalf("turn %d: %v %+v", index, err, result)
		}
		again, err := fx.service.ProcessMessage(ctx, input)
		if err != nil || jsonValue(again) != jsonValue(result) {
			t.Fatal("inbound replay", err)
		}
	}
	fx.db.First(&session, "id = ?", result.ConversationID)
	if session.BotVersionID != published.Version.ID || session.CurrentStepKey != "payment_choice" {
		t.Fatalf("pin: %+v", session)
	}
	var orders []core.Order
	fx.db.Find(&orders)
	if len(orders) != 1 || orders[0].TotalMinor != 840000 {
		t.Fatalf("orders: %+v", orders)
	}
	input.Text = "I have paid"
	input.ExternalMessageID = uuid.NewString()
	result, err = fx.service.ProcessMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Text = "1"
	input.ExternalMessageID = uuid.NewString()
	result, err = fx.service.ProcessMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	fx.db.First(&session, "id = ?", result.ConversationID)
	if session.CurrentStepKey != "payment_choice" || stringValue(parseJSONMap(session.Variables)["payment_status"]) == core.PaymentPaid {
		t.Fatal("customer claim verified payment")
	}
	var notifications int64
	fx.db.Model(&core.CommerceNotification{}).Count(&notifications)
	if notifications != 0 {
		t.Fatal("unverified payment notifications")
	}
}
func TestJourneyInventoryChangeAndOpaqueSelection(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	_, store, variant := seedCommerce(t, fx)
	ctx := context.Background()
	p, err := fx.service.StartJourneyPreview(ctx, fx.actor, detail.Version.ID, "normal")
	if err != nil {
		t.Fatal(err)
	}
	bad := journeyPreviewAnswer(t, fx, p, store.ID.String())
	if bad.CurrentStep != "store" {
		t.Fatal("raw entity ID accepted")
	}
	for _, text := range []string{"1", "1", "1", "1"} {
		p = journeyPreviewAnswer(t, fx, p, text)
	}
	fx.db.Model(&core.InventoryLevel{}).Where("store_id = ? AND variant_id = ?", store.ID, variant.ID).Update("on_hand", 0)
	p = journeyPreviewAnswer(t, fx, p, "2")
	if !p.Result.Handoff || len(p.Cart) != 0 {
		t.Fatalf("stock change ignored: %+v", p)
	}
}

func TestJourneyInventoryChangedAfterReviewDoesNotCreateOrder(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	customer, store, variant := seedCommerce(t, fx)
	ctx := context.Background()
	if _, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID); err != nil {
		t.Fatal(err)
	}
	fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", jsonValue(map[string]any{"bot_id": detail.Bot.ID.String()}))
	input := InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "late-stock", Text: "order", ExternalMessageID: uuid.NewString()}
	result, err := fx.service.ProcessMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"1", "1", "1", "1", "2", "2", "1", "Ada", "+2348010000001", "ada@example.test"} {
		input.Text, input.ExternalMessageID = text, uuid.NewString()
		result, err = fx.service.ProcessMessage(ctx, input)
		if err != nil || result.Error != nil {
			t.Fatalf("checkout: %v %+v", err, result)
		}
	}
	var session ConversationSession
	fx.db.First(&session, "id = ?", result.ConversationID)
	if session.CurrentStepKey != "confirm" {
		t.Fatalf("expected review: %+v", session)
	}
	if err := fx.db.Model(&core.InventoryLevel{}).Where("store_id = ? AND variant_id = ?", store.ID, variant.ID).Update("on_hand", 0).Error; err != nil {
		t.Fatal(err)
	}
	input.Text, input.ExternalMessageID = "1", uuid.NewString()
	result, err = fx.service.ProcessMessage(ctx, input)
	if err != nil || !result.Handoff {
		t.Fatalf("late inventory failure did not stop checkout: %v %+v", err, result)
	}
	for _, model := range []any{&core.Order{}, &core.Payment{}, &core.CommerceNotification{}} {
		var count int64
		if err := fx.db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("late stock failure wrote %T: %d %v", model, count, err)
		}
	}
}

func TestJourneyFlowBackExpiryAndFrontendTokenRedaction(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	customer, _, _ := seedCommerce(t, fx)
	ctx := context.Background()
	if _, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID); err != nil {
		t.Fatal(err)
	}
	fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", jsonValue(map[string]any{"bot_id": detail.Bot.ID.String()}))
	input := InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "flow-back", Text: "order", ExternalMessageID: uuid.NewString()}
	result, err := fx.service.ProcessMessage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var session ConversationSession
	fx.db.First(&session, "id = ?", result.ConversationID)
	token := "secret-flow-session-token"
	flow := JourneyFlowSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ConnectionID: fx.channel.ID, ConversationID: session.ID, VersionID: detail.Version.ID, TokenHash: flowTokenHash(token), ExpiresAt: time.Now().Add(time.Hour)}
	if err := fx.db.Create(&flow).Error; err != nil {
		t.Fatal(err)
	}
	request := JourneyFlowRequest{Version: "3.0", Action: "data_exchange", FlowToken: token, Screen: bot.FlowScreenID(session.CurrentStepKey)}
	request.Data.Answer, request.Data.Turn = "1", journeyTurn(session)
	response, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Action, request.Screen = "BACK", stringValue(response["screen"])
	request.Data.Answer, request.Data.Turn = "", ""
	response, err = fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil || response["screen"] != bot.FlowScreenID("store") {
		t.Fatalf("native BACK with empty data: %v %+v", err, response)
	}
	fx.db.Model(&JourneyFlowSession{}).Where("id = ?", flow.ID).Update("expires_at", fx.service.now().Add(-time.Minute))
	request.Action = "INIT"
	if _, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request); err == nil {
		t.Fatal("expired Flow token accepted")
	}
	message := ConversationMessage{ID: uuid.New(), OrganizationID: session.OrganizationID, SessionID: session.ID, ChannelID: session.ChannelID, Direction: DirectionOutbound, MessageType: "flow", Body: "Continue", Metadata: jsonValue(map[string]any{"metadata": map[string]any{"flow_token": token, "flow_id": "provider-flow"}})}
	if err := fx.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	messages, err := fx.service.ListConversationMessages(ctx, fx.actor, session.ID)
	if err != nil || strings.Contains(jsonValue(messages), token) {
		t.Fatal("Flow credential exposed to admin frontend", err)
	}
	fx.db.First(&message, "id = ?", message.ID)
	if !strings.Contains(message.Metadata, token) {
		t.Fatal("redaction changed durable delivery metadata")
	}
}
func TestJourneyFlowTenantReplayAndExecutionLease(t *testing.T) {
	fx, detail := journeyFixture(t, "support")
	ctx := context.Background()
	published, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", jsonValue(map[string]any{"bot_id": published.Bot.ID.String()}))
	result, err := fx.service.ProcessMessage(ctx, InboundMessage{ChannelID: &fx.channel.ID, Sender: "+2348019999999", ExternalConversationID: "flow", ExternalMessageID: "flow-start", Text: "support"})
	if err != nil {
		t.Fatal(err)
	}
	var session ConversationSession
	fx.db.First(&session, "id = ?", result.ConversationID)
	token := "unpredictable-test-token"
	flow := JourneyFlowSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ConnectionID: fx.channel.ID, ConversationID: session.ID, VersionID: published.Version.ID, TokenHash: flowTokenHash(token), ExpiresAt: time.Now().Add(time.Hour)}
	if err := fx.db.Create(&flow).Error; err != nil {
		t.Fatal(err)
	}
	request := JourneyFlowRequest{Version: "3.0", Action: "INIT", FlowToken: token}
	response, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ExchangeJourneyFlow(ctx, uuid.New(), fx.channel.ID, request); err == nil {
		t.Fatal("cross tenant flow token")
	}
	request.Action = "data_exchange"
	request.Screen = bot.FlowScreenID(session.CurrentStepKey)
	request.Data.Answer = "I need help"
	request.Data.Turn = journeyTurn(session)
	response, err = fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil || jsonValue(response) != jsonValue(again) {
		t.Fatal("flow replay", err)
	}
	release, err := fx.service.acquireJourney(ctx, &session)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := fx.service.acquireJourney(ctx, &session); err == nil {
		t.Fatal("concurrent execution")
	}
	var count int64
	fx.db.Model(&SupportHandoff{}).Count(&count)
	if count != 1 {
		t.Fatal("handoff duplicates", count)
	}
}

func TestJourneyFlowCommerceSubmissionCreatesOrderExactlyOnce(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	customer, _, _ := seedCommerce(t, fx)
	ctx := context.Background()
	if _, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID); err != nil {
		t.Fatal(err)
	}
	fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", jsonValue(map[string]any{"bot_id": detail.Bot.ID.String()}))
	initial, err := fx.service.ProcessMessage(ctx, InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "flow-order", ExternalMessageID: uuid.NewString(), Text: "order"})
	if err != nil {
		t.Fatal(err)
	}
	var session ConversationSession
	fx.db.First(&session, "id = ?", initial.ConversationID)
	system := parseJSONMap(session.SystemContext)
	system["journey_mode"] = "whatsapp_flow"
	fx.db.Model(&session).Update("system_context", jsonValue(system))
	token := "flow-checkout-token"
	flow := JourneyFlowSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ConnectionID: fx.channel.ID, ConversationID: session.ID, VersionID: detail.Version.ID, TokenHash: flowTokenHash(token), ExpiresAt: fx.service.now().Add(time.Hour)}
	if err := fx.db.Create(&flow).Error; err != nil {
		t.Fatal(err)
	}
	request := JourneyFlowRequest{Version: "3.0", Action: "INIT", FlowToken: token}
	response, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	for index, answer := range []string{"1", "1", "1", "1", "2", "2", "1", "Ada", "+2348010000001", "ada@example.test", "1"} {
		request.Action, request.Screen = "data_exchange", stringValue(response["screen"])
		request.Data.Turn = stringValue(response["data"].(map[string]any)["turn"])
		request.Data.Answer = answer
		response, err = fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
		if err != nil {
			t.Fatalf("Flow checkout turn %d: %v", index, err)
		}
		replay, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
		if err != nil || jsonValue(replay) != jsonValue(response) {
			t.Fatalf("Flow replay %d: %v", index, err)
		}
	}
	if response["screen"] != bot.FlowScreenID("payment_choice") {
		t.Fatalf("wrong checkout response: %+v", response)
	}
	var orders []core.Order
	fx.db.Find(&orders)
	if len(orders) != 1 || orders[0].TotalMinor != 840000 || orders[0].Status != core.OrderAwaitingPayment {
		t.Fatalf("unsafe Flow order: %+v", orders)
	}
	var payments int64
	fx.db.Model(&core.Payment{}).Count(&payments)
	if payments != 1 {
		t.Fatal("duplicate Flow payment", payments)
	}
}
func TestJourneyAIIntentAndScenarioFallbacks(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	seedCommerce(t, fx)
	ctx := context.Background()
	_, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, err := fx.service.journeyRoute(ctx, fx.channel, "I would like to place an order")
	if err != nil || id == nil || *id != detail.Bot.ID {
		t.Fatal("intent routing", err)
	}
	for _, scenario := range []string{"empty_catalogue", "handoff", "unsupported_channel"} {
		p, err := fx.service.StartJourneyPreview(ctx, fx.actor, detail.Version.ID, scenario)
		if err != nil {
			t.Fatal(err)
		}
		if scenario == "handoff" && !p.Result.Handoff {
			t.Fatal("handoff scenario")
		}
		if scenario == "unsupported_channel" {
			raw, _ := json.Marshal(p.Result.Messages)
			if !strings.Contains(string(raw), "1.") {
				t.Fatal("no text fallback")
			}
		}
	}
}

func TestJourneyVersionedMenuRoutesSameInterpreter(t *testing.T) {
	fx, order := journeyFixture(t, "order")
	seedCommerce(t, fx)
	ctx := context.Background()
	service := bot.NewService(fx.db)
	if _, err := service.PublishJourney(ctx, fx.actor, order.Version.ID); err != nil {
		t.Fatal(err)
	}
	menu, err := service.CreateJourney(ctx, fx.actor, "entry_menu", "Welcome menu")
	if err != nil {
		t.Fatal(err)
	}
	d := menu.Definition
	d.Channel = "test"
	d.ConnectionID = &fx.channel.ID
	menu, err = service.SaveJourney(ctx, fx.actor, menu.Version.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	menu, err = service.PublishJourney(ctx, fx.actor, menu.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	routed, err := fx.service.journeyRoute(ctx, fx.channel, "hello")
	if err != nil || routed == nil || *routed != menu.Bot.ID {
		t.Fatal("default routing", err)
	}
	p, err := fx.service.StartJourneyPreview(ctx, fx.actor, menu.Version.ID, "normal")
	if err != nil {
		t.Fatal(err)
	}
	p = journeyPreviewAnswer(t, fx, p, "1")
	if p.CurrentStep != "store" {
		t.Fatalf("menu did not enter order: %+v", p)
	}
	p = journeyPreviewAnswer(t, fx, p, "1")
	if p.CurrentStep != "category" {
		t.Fatalf("routed preview lost its pinned snapshot: %+v", p)
	}
}

func TestJourneyControlledTestSafetyAndFreshPrices(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	_, _, variant := seedCommerce(t, fx)
	ctx := context.Background()
	published, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.StartControlledJourneyTest(ctx, fx.actor, published.Version.ID, false); err == nil {
		t.Fatal("implicit test writes")
	}
	viewer := fx.actor
	viewer.Role = authz.Viewer
	if _, err := fx.service.StartControlledJourneyTest(ctx, viewer, published.Version.ID, true); err == nil {
		t.Fatal("viewer test writes")
	}
	p, err := fx.service.StartControlledJourneyTest(ctx, fx.actor, published.Version.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"1", "1", "1", "1", "2", "2", "1", "Ada", "+2348010000001", "ada@example.test"} {
		p, err = fx.service.AnswerControlledJourneyTest(ctx, fx.actor, p.SessionID, JourneyAnswer{EventID: uuid.NewString(), Text: text, Turn: p.Turn})
		if err != nil {
			t.Fatal(err)
		}
	}
	if p.CurrentStep != "confirm" {
		t.Fatalf("test checkout: %+v", p)
	}
	if err := fx.db.Model(&core.Variant{}).Where("id = ?", variant.ID).Update("price_minor", 500000).Error; err != nil {
		t.Fatal(err)
	}
	p, err = fx.service.AnswerControlledJourneyTest(ctx, fx.actor, p.SessionID, JourneyAnswer{EventID: uuid.NewString(), Text: "1", Turn: p.Turn})
	if err != nil {
		t.Fatal(err)
	}
	var orders []core.Order
	fx.db.Find(&orders)
	if len(orders) != 1 || orders[0].TotalMinor != 1000000 {
		t.Fatalf("current DB price ignored: %+v", orders)
	}
	var payments []core.Payment
	fx.db.Find(&payments)
	if len(payments) != 1 || payments[0].Provider != "test" || payments[0].Status == core.PaymentPaid {
		t.Fatalf("test payment safety: %+v (%+v)", payments, p)
	}
	var notifications int64
	fx.db.Model(&core.CommerceNotification{}).Count(&notifications)
	if notifications != 0 {
		t.Fatal("test sent notifications")
	}
	fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("provider", "whatsapp")
	if _, err := fx.service.StartControlledJourneyTest(ctx, fx.actor, published.Version.ID, true); err == nil {
		t.Fatal("real channel allowed")
	}
}
