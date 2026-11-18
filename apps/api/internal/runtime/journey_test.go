package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
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
	customer, store, _ := seedCommerce(t, fx)
	if err := fx.db.Model(&store).Update("metadata", jsonValue(map[string]any{"notification_recipient": "test-store-recipient"})).Error; err != nil {
		t.Fatal(err)
	}
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
	// Pending provider state must keep the customer in checkout, even when
	// they explicitly request a payment-status check.
	request.Screen = stringValue(response["screen"])
	request.Data.Turn = stringValue(response["data"].(map[string]any)["turn"])
	request.Data.Answer = "1"
	response, err = fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil || response["screen"] != bot.FlowScreenID("payment_choice") {
		t.Fatalf("unpaid checkout advanced: %+v %v", response, err)
	}
	var payment core.Payment
	if err := fx.db.First(&payment).Error; err != nil {
		t.Fatal(err)
	}
	// Deterministic provider verification models an independently confirmed
	// payment; this is deliberately not evidence of a live Paystack charge.
	verifiedService := core.NewService(fx.db, runtimeTestPaymentProvider{name: "paystack", db: fx.db})
	if _, err := verifiedService.VerifyPayment(ctx, fx.actor, core.PaymentVerifyInput{Reference: payment.Reference}); err != nil {
		t.Fatal(err)
	}
	request.Screen = stringValue(response["screen"])
	request.Data.Turn = stringValue(response["data"].(map[string]any)["turn"])
	response, err = fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil || response["screen"] != "FINISH" {
		t.Fatalf("verified checkout did not finish: %+v %v", response, err)
	}
	finalReplay, err := fx.service.ExchangeJourneyFlow(ctx, fx.actor.OrganizationID, fx.channel.ID, request)
	if err != nil || jsonValue(finalReplay) != jsonValue(response) {
		t.Fatalf("verified checkout replay changed result: %+v %v", finalReplay, err)
	}
	var notifications int64
	fx.db.Model(&core.CommerceNotification{}).Where("order_id = ? AND notification_type IN ?", orders[0].ID, []string{"customer_notification", "store_notification"}).Count(&notifications)
	if notifications != 2 {
		t.Fatalf("expected store/customer notifications, got %d", notifications)
	}
	if _, err := verifiedService.TransitionOrder(ctx, fx.actor, orders[0].ID, core.TransitionInput{Status: core.OrderProcessing, IdempotencyKey: "flow-prepare"}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"ready", "completed"} {
		if _, err := verifiedService.UpdateFulfilment(ctx, fx.actor, orders[0].ID, core.FulfilmentInput{Status: status, IdempotencyKey: "flow-" + status}); err != nil {
			t.Fatal(err)
		}
	}
	finished, err := verifiedService.GetOrder(ctx, fx.actor, orders[0].ID)
	if err != nil || finished.Status != core.OrderCompleted {
		t.Fatalf("fulfilment incomplete: %+v %v", finished, err)
	}
}

func TestJourneyFlowCheckoutDispatchDoesNotDependOnWording(t *testing.T) {
	for _, text := range []string{"Complete checkout", "Payer votre commande", "https://checkout.paystack.com/example"} {
		result := RuntimeResult{SessionStatus: SessionActive, Messages: []OutboundMessage{{Type: MessageText, Text: text, Metadata: map[string]any{"journey_checkout": true}}}}
		if !journeyFlowNeedsDispatch(result) {
			t.Fatalf("checkout was suppressed: %s", text)
		}
	}
	if journeyFlowNeedsDispatch(RuntimeResult{SessionStatus: SessionActive, Messages: []OutboundMessage{{Text: "A payment link will be generated after confirmation."}}}) {
		t.Fatal("ordinary screen text dispatched as checkout")
	}
	if !journeyFlowNeedsDispatch(RuntimeResult{SessionStatus: SessionCompleted}) {
		t.Fatal("final confirmation suppressed")
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

func TestReleasedGuidedJourneyAnswersFreeFormAndPreservesCheckout(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	customer, _, _ := seedCommerce(t, fx)
	ctx := context.Background()
	if _, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, detail.Version.ID); err != nil {
		t.Fatal(err)
	}

	calls := 0
	fx.service.ConfigureAIInbound(func(_ context.Context, session ConversationSession, _ string) (AIInboundResponse, error) {
		calls++
		if calls == 1 {
			return AIInboundResponse{}, fmt.Errorf("provider unavailable")
		}
		if session.HandoffState != "released" {
			t.Fatalf("expected released session, got %q", session.HandoffState)
		}
		return AIInboundResponse{Reply: "The test mug costs NGN 2,500."}, nil
	})
	send := func(text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(ctx, InboundMessage{
			ChannelID: &fx.channel.ID, Sender: customer.Phone,
			ExternalConversationID: "guided-release", Text: text, ExternalMessageID: uuid.NewString(),
		})
		if err != nil || result.Error != nil {
			t.Fatalf("processing %q: %v %+v", text, err, result)
		}
		return result
	}
	start := send("order")
	handoff, err := fx.service.RequestSupportHandoff(ctx, fx.actor, start.ConversationID, "Test release")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ReleaseSupportHandoff(ctx, fx.actor, handoff.ID, SupportHandoffReleaseInput{ResumeBot: true}); err != nil {
		t.Fatal(err)
	}

	const question = "How much is the test mug?"
	fallback := send(question)
	if fallback.SessionStatus != SessionActive || len(fallback.Messages) != 1 || fallback.Messages[0].Text != "I couldn't answer that right now. Please try again, or send menu to continue." {
		t.Fatalf("expected one safe fallback with the conversation still active, got %+v", fallback)
	}
	answer := send(question)
	if len(answer.Messages) != 1 || answer.Messages[0].Text != "The test mug costs NGN 2,500." {
		t.Fatalf("expected one AI reply after provider recovery, got %+v", answer)
	}
	order := send("order")
	if len(order.Messages) == 0 || calls != 2 {
		t.Fatalf("expected guided ordering without an AI call, got %+v (%d AI calls)", order, calls)
	}
	send("1")
	if calls != 2 {
		t.Fatalf("guided checkout selection called AI (%d calls)", calls)
	}
}

func TestJourneyCustomKeywordAndTenantScopedDefault(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	ctx := context.Background()
	d := detail.Definition
	d.Trigger = "guided"
	d.EntryCTA = "Start guided order"
	d.Steps[0].Position = &bot.JourneyPosition{X: 12, Y: 34}
	d.Mode = "guided_questions"
	saved, err := bot.NewService(fx.db).SaveJourney(ctx, fx.actor, detail.Version.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	published, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, saved.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err = bot.NewService(fx.db).SetDefaultJourney(ctx, fx.actor, published.Bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ChannelDefault == nil || detail.ChannelDefault.VersionID != published.Version.ID || detail.ChannelDefault.Mode != d.Mode || detail.Definition.Steps[0].Position == nil || detail.Definition.Steps[0].Position.X != 12 {
		t.Fatalf("published default or position missing: %+v", detail)
	}
	menu, err := fx.service.JourneyEntryMenu(ctx, fx.actor.OrganizationID, fx.channel.ID)
	if err != nil || len(menu.Options) != 1 || menu.Options[0].Label != d.EntryCTA {
		t.Fatalf("menu: %+v %v", menu, err)
	}
	legacyMenu := bot.JourneyBinding{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, BotID: uuid.New(), VersionID: uuid.New(), ConnectionID: &fx.channel.ID, Trigger: "default", Mode: "guided_chat"}
	if err := fx.db.Create(&legacyMenu).Error; err != nil {
		t.Fatal(err)
	}
	var channel core.Channel
	if err := fx.db.First(&channel, "id = ?", fx.channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{"guided", "hello", "something unfamiliar"} {
		id, err := fx.service.journeyRoute(ctx, channel, keyword)
		if err != nil || id == nil || *id != published.Bot.ID {
			t.Fatalf("route %q: %v %v", keyword, id, err)
		}
	}
	other := channel
	other.OrganizationID = uuid.New()
	id, err := fx.service.journeyRoute(ctx, other, "hello")
	if err != nil || id != nil {
		t.Fatalf("default crossed tenant: %v %v", id, err)
	}
	viewer := fx.actor
	viewer.Role = authz.Viewer
	if _, err := bot.NewService(fx.db).SetDefaultJourney(ctx, viewer, published.Bot.ID); err == nil {
		t.Fatal("viewer changed default")
	}
}

func TestExplicitJourneySwitchKeepsHistoryAndProtectsCheckout(t *testing.T) {
	fx, order := journeyFixture(t, "order")
	customer, store, _ := seedCommerce(t, fx)
	ctx := context.Background()
	svc := bot.NewService(fx.db)
	if _, err := svc.PublishJourney(ctx, fx.actor, order.Version.ID); err != nil {
		t.Fatal(err)
	}
	ai, err := svc.CreateJourney(ctx, fx.actor, "product_enquiry", "Enquiry")
	if err != nil {
		t.Fatal(err)
	}
	d := ai.Definition
	d.ConnectionID = &fx.channel.ID
	d.Channel = "test"
	if _, err = svc.SaveJourney(ctx, fx.actor, ai.Version.ID, d); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.PublishJourney(ctx, fx.actor, ai.Version.ID); err != nil {
		t.Fatal(err)
	}
	send := func(text string) RuntimeResult {
		t.Helper()
		r, e := fx.service.ProcessMessage(ctx, InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "switch-test", Text: text, ExternalMessageID: uuid.NewString()})
		if e != nil || r.Error != nil {
			t.Fatalf("%s: %v %+v", text, e, r.Error)
		}
		return r
	}
	first := send("ask")
	second := send("order")
	var session ConversationSession
	if err := fx.db.First(&session, "id = ?", first.ConversationID).Error; err != nil {
		t.Fatal(err)
	}
	if session.BotVersionID != order.Version.ID || second.ConversationID != first.ConversationID || session.ExpectedInput != "store" {
		t.Fatalf("switch lost pin or history: %+v", session)
	}
	if err := fx.db.Model(&session).Update("variables", jsonValue(map[string]any{"cart_id": uuid.NewString()})).Error; err != nil {
		t.Fatal(err)
	}
	blocked := send("ask")
	if !strings.Contains(blocked.Messages[0].Text, "checkout is in progress") {
		t.Fatalf("cart lost on switch: %+v", blocked)
	}
	orderRecord := core.Order{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, CustomerID: customer.ID, StoreID: store.ID, OrderNumber: "QA-" + uuid.NewString(), Status: core.OrderCompleted, Currency: "NGN"}
	if err := fx.db.Create(&orderRecord).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&session).Update("variables", jsonValue(map[string]any{"order_id": orderRecord.ID.String(), "cart_id": uuid.NewString()})).Error; err != nil {
		t.Fatal(err)
	}
	send("ask")
	if err := fx.db.First(&session, "id = ?", first.ConversationID).Error; err != nil || session.BotVersionID != ai.Version.ID {
		t.Fatalf("completed order did not release checkout: %v %+v", err, session)
	}

}

func TestJourneyCancelOrderUsesAuthoritativeOrderState(t *testing.T) {
	fx, d := journeyFixture(t, "order")
	customer, store, variant := seedCommerce(t, fx)
	ctx := context.Background()
	if _, err := bot.NewService(fx.db).PublishJourney(ctx, fx.actor, d.Version.ID); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{core.OrderPaid, core.OrderAwaitingPayment} {
		order, err := fx.service.commerce.CreateOrder(ctx, fx.actor, core.OrderInput{CustomerID: customer.ID, StoreID: store.ID, FulfilmentType: core.FulfilmentPickup, Items: []core.OrderItemInput{{VariantID: variant.ID, Quantity: 1}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := fx.db.Model(&order).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		send := func(text string) RuntimeResult {
			t.Helper()
			r, e := fx.service.ProcessMessage(ctx, InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "cancel-" + status, Text: text, ExternalMessageID: uuid.NewString()})
			if e != nil || r.Error != nil {
				t.Fatalf("message failed %v %+v", e, r.Error)
			}
			return r
		}
		start := send("order")
		if err := fx.db.Model(&ConversationSession{}).Where("id = ?", start.ConversationID).Update("variables", jsonValue(map[string]any{"order_id": order.ID.String()})).Error; err != nil {
			t.Fatal(err)
		}
		result := send("cancel order")
		fresh, err := fx.service.commerce.GetOrder(ctx, fx.actor, order.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status == core.OrderPaid {
			if fresh.Status != core.OrderPaid || result.SessionStatus != SessionActive {
				t.Fatalf("paid order cancellation accepted: %s %+v", fresh.Status, result)
			}
		} else if fresh.Status != core.OrderCancelled || result.SessionStatus != SessionCancelled {
			t.Fatalf("unpaid order cancellation lost: %s %+v", fresh.Status, result)
		}
	}
}

func TestConfirmedReceiptEmailRejectionRetriesOnlyPayment(t *testing.T) {
	fx, journey := journeyFixture(t, "order")
	ctx := context.Background()
	customer, store, _ := seedCommerce(t, fx)
	graph, err := bot.NewService(fx.db).GetConfiguration(ctx, fx.actor, journey.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	session := ConversationSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, BotID: journey.Bot.ID, BotVersionID: journey.Version.ID, CustomerID: &customer.ID, ChannelID: fx.channel.ID, Status: SessionActive, CurrentStepKey: "payment", Variables: jsonValue(map[string]any{"order_id": id, "store_id": store.ID.String(), "customer_email": "invalid@example.test"}), SystemContext: `{"journey_mode":"guided_questions"}`}
	calls := 0
	fx.service.actions.Register("initialize_payment", func(_ context.Context, _ RuntimeContext, input map[string]any) (map[string]any, error) {
		calls++
		if input["order_id"] != id {
			t.Fatal("payment used a different order")
		}
		if calls == 1 {
			return nil, httperror.New(400, core.CheckoutEmailRejected, "rejected receipt")
		}
		if input["email"] != "buyer@example.com" {
			t.Fatal("did not pass corrected address")
		}
		return map[string]any{"payment_url": "https://example.test/checkout", "message": "Your test link"}, nil
	})
	send := func(text string) RuntimeResult {
		t.Helper()
		r, e := fx.service.executeJourney(ctx, graph, &session, InboundMessage{Text: text})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	send("")
	if session.ExpectedInput != paymentRetryEmail || calls != 1 {
		t.Fatal("missing payment email correction")
	}
	send("not an email")
	if calls != 1 {
		t.Fatal("invalid email reached payment provider")
	}
	send("buyer@example.com")
	if calls != 2 || session.CurrentStepKey != "payment_choice" || session.ExpectedInput != "payment_choice" {
		t.Fatalf("did not resume existing checkout: calls %d, step %s", calls, session.CurrentStepKey)
	}
}

func TestReopenNativeFlowResumesPinnedStepAndExpiresOldToken(t *testing.T) {
	fx, d := journeyFixture(t, "support")
	ctx := context.Background()
	graph, err := bot.NewService(fx.db).GetConfiguration(ctx, fx.actor, d.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	session := ConversationSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, BotID: d.Bot.ID, BotVersionID: d.Version.ID, ChannelID: fx.channel.ID, Status: SessionActive, CurrentStepKey: "details", ExpectedInput: "details", Variables: `{"saved":"answer"}`, SystemContext: `{"journey_mode":"whatsapp_flow","journey_flow_launched":true}`}
	deployment := bot.JourneyFlowDeployment{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, BotID: d.Bot.ID, VersionID: d.Version.ID, ConnectionID: fx.channel.ID, ProviderFlowID: "test-flow", Status: "published"}
	if err := fx.db.Create(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	var previous string
	for i := 0; i < 2; i++ {
		r, e := fx.service.executeConfiguredJourney(ctx, graph, &session, InboundMessage{Text: d.Definition.Trigger})
		if e != nil || len(r.Messages) != 1 || r.Messages[0].Type != "flow" || session.CurrentStepKey != "details" || session.Variables != `{"saved":"answer"}` {
			t.Fatalf("reopen: %v %+v", e, r)
		}
		if previous != "" {
			var n int64
			fx.db.Model(&JourneyFlowSession{}).Where("token_hash = ? AND expires_at > ?", previous, fx.service.now()).Count(&n)
			if n != 0 {
				t.Fatal("old form remained active after replacement")
			}
		}
		previous = flowTokenHash(stringValue(r.Messages[0].Metadata["flow_token"]))
	}
}

func TestFlowOutboxPreservesLaunchFieldsAndSkipsSupersededRetries(t *testing.T) {
	fx, d := journeyFixture(t, "support")
	ctx := context.Background()
	session := ConversationSession{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, ChannelID: fx.channel.ID, BotVersionID: d.Version.ID}
	deployment := bot.JourneyFlowDeployment{ID: uuid.New(), OrganizationID: session.OrganizationID, BotID: d.Bot.ID, VersionID: session.BotVersionID, ConnectionID: session.ChannelID, ProviderFlowID: "provider-flow", Status: "published"}
	if err := fx.db.Create(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	sender := &MockChannelSender{Errors: []error{fmt.Errorf("provider temporarily unavailable")}}
	fx.service.RegisterChannelSender("test", sender)
	for _, reopen := range []bool{false, true} {
		msg, err := fx.service.launchJourneyFlow(ctx, session, "Open form")
		if err != nil {
			t.Fatal(err)
		}
		msg.Metadata["private"] = "omit-this"
		del := ChannelOutboundMessage{ID: uuid.New(), OrganizationID: session.OrganizationID, ChannelID: session.ChannelID, SessionID: &session.ID, Recipient: "test-user", Provider: "test", MessageType: "flow", Status: OutboundQueued, Payload: jsonValue(fx.service.channelPayload(fx.channel, "test-user", msg)), ProviderResponse: "{}", IdempotencyKey: uuid.NewString()}
		if err := fx.db.Create(&del).Error; err != nil {
			t.Fatal(err)
		}
		if !reopen && fx.service.sendOutboundNow(ctx, session.OrganizationID, del.ID, 1, 5) == nil {
			t.Fatal("expected retry")
		}
		if reopen {
			if _, err := fx.service.launchJourneyFlow(ctx, session, "Reopen"); err != nil {
				t.Fatal(err)
			}
		}
		if err := fx.service.sendOutboundNow(ctx, session.OrganizationID, del.ID, 2, 5); err != nil {
			t.Fatal(err)
		}
		for _, attempt := range sender.Sent {
			out := outboundMessageFromPayload("flow", attempt["payload"].(map[string]any))
			cmd := ChannelOutboundCommand("whatsapp", session.ChannelID, &session.ID, "test-user", "", out, "")
			if !reopen && (len(cmd.Metadata) != 4 || cmd.Metadata["flow_token"] != msg.Metadata["flow_token"] || cmd.Metadata["flow_id"] != "provider-flow" || cmd.Metadata["flow_cta"] != "Open form" || cmd.Metadata["flow_action"] != "data_exchange") {
				t.Fatal("incomplete provider command")
			}
		}
		if len(sender.Sent) != 2 {
			t.Fatal("unexpected number of provider attempts")
		}
		rows, err := fx.service.ListOutboundMessages(ctx, session.OrganizationID, 100)
		if err != nil || len(rows) == 0 {
			t.Fatalf("list deliveries: %v", err)
		}
		for _, row := range rows {
			if strings.Contains(row.Payload, "flow_token") || strings.Contains(row.Payload, "omit-this") {
				t.Fatal("delivery list exposed private metadata")
			}
			if reopen && row.ID == del.ID && row.Status != OutboundSkipped {
				t.Fatal("superseded delivery not skipped")
			}
		}
	}
}
