package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

func TestClassifyCommandRecognizesGreetingsAndControls(t *testing.T) {
	cases := map[string]lifecycleCommand{
		"hi":           commandGreeting,
		"Hello!":       commandGreeting,
		"heyyy":        commandGreeting,
		"good morning": commandGreeting,
		"menu":         commandMenu,
		"Main menu":    commandMenu,
		"restart":      commandRestart,
		"start":        commandRestart,
		"cancel":       commandCancel,
		"back":         commandBack,
		"2":            commandNone,
		"track_order":  commandNone,
		"I want tea":   commandNone,
	}
	for text, want := range cases {
		if got := classifyCommand(text); got != want {
			t.Fatalf("classifyCommand(%q)=%q want %q", text, got, want)
		}
	}
}

func TestRuntimeGreetingAtMenuDoesNotRejectInput(t *testing.T) {
	fx := publishedSelfServiceRuntime(t)
	first, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "g1", "conv-greet", "heyyy"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joinMessageTexts(first.Messages), "Welcome") && len(first.Messages) == 0 {
		t.Fatalf("expected greeting to show the entry experience, got %+v", first.Messages)
	}
	second, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, "g2", "conv-greet", "heyyy"))
	if err != nil {
		t.Fatal(err)
	}
	text := joinMessageTexts(second.Messages)
	if strings.Contains(text, "Please choose one of the available options") || strings.Contains(text, "I didn't recognize that option") {
		t.Fatalf("greeting at the main menu should not be treated as an invalid option, got %+v", second.Messages)
	}
	if len(second.Messages) == 0 {
		t.Fatal("expected the menu to be presented again")
	}
}

func TestRuntimeTrackOrderWithZeroOrdersOffersRecovery(t *testing.T) {
	fx := publishedSelfServiceRuntime(t)
	sendSelf := func(id, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-empty-track", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	sendSelf("e1", "hi")
	track := sendSelf("e2", "2")
	text := joinMessageTexts(track.Messages)
	if strings.Contains(text, "Choose an order using its number") {
		t.Fatalf("zero orders must not ask for an order number, got %+v", track.Messages)
	}
	if !strings.Contains(text, "couldn't find any recent orders") && !strings.Contains(strings.ToLower(text), "could not find") {
		t.Fatalf("expected empty-order recovery copy, got %+v", track.Messages)
	}
	invalid := sendSelf("e3", "8")
	invalidText := joinMessageTexts(invalid.Messages)
	if strings.Contains(invalidText, "Choose an order using its number") {
		t.Fatalf("invalid empty-track choice still used the old copy, got %+v", invalid.Messages)
	}
	if !strings.Contains(invalidText, "I didn't recognize that option") {
		t.Fatalf("expected useful invalid-input copy, got %+v", invalid.Messages)
	}
	menu := sendSelf("e4", "3")
	if !strings.Contains(strings.ToLower(joinMessageTexts(menu.Messages)), "welcome") && menu.SessionStatus != SessionActive {
		t.Fatalf("expected return to main menu, got %+v", menu)
	}
}

func TestRuntimeInvalidMenuOptionIsHelpfulAndEventuallyResets(t *testing.T) {
	fx := publishedSelfServiceRuntime(t)
	sendSelf := func(id, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-invalid", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	sendSelf("i1", "hello")
	first := sendSelf("i2", "8")
	if !strings.Contains(joinMessageTexts(first.Messages), "I didn't recognize that option") {
		t.Fatalf("expected helpful invalid option copy, got %+v", first.Messages)
	}
	sendSelf("i3", "nope")
	reset := sendSelf("i4", "zzzz")
	text := joinMessageTexts(reset.Messages)
	if !strings.Contains(text, "main menu") && !strings.Contains(strings.ToLower(text), "welcome") {
		t.Fatalf("expected retry limit to return to the menu, got %+v", reset.Messages)
	}
}

func TestRuntimeMenuAndCancelReturnToEntry(t *testing.T) {
	fx := publishedSelfServiceRuntime(t)
	sendSelf := func(id, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-nav", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	sendSelf("n1", "hi")
	sendSelf("n2", "1")
	cancel := sendSelf("n3", "cancel")
	if !strings.Contains(joinMessageTexts(cancel.Messages), "cancelled") && !strings.Contains(strings.ToLower(joinMessageTexts(cancel.Messages)), "welcome") {
		t.Fatalf("expected cancel to leave the order flow, got %+v", cancel.Messages)
	}
	menu := sendSelf("n4", "menu")
	if menu.SessionStatus != SessionActive {
		t.Fatalf("expected menu command to keep the session active, got %+v", menu)
	}
}

func TestRuntimeTrackOrderWithMultipleOrdersAcceptsSelection(t *testing.T) {
	fx := publishedSelfServiceRuntime(t)
	customer, err := fx.commerce.FindOrCreateCustomer(context.Background(), fx.actor, core.CustomerInput{Name: "Ada", Phone: "customer"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := fx.commerce.CreateStore(context.Background(), fx.actor, core.StoreInput{Name: "Lekki", Code: "LK", Status: core.StatusActive, Address: "Lekki", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	category, err := fx.commerce.CreateCategory(context.Background(), fx.actor, core.CategoryInput{Name: "Tea", Slug: "tea", Status: core.StatusActive})
	if err != nil {
		t.Fatal(err)
	}
	product, err := fx.commerce.CreateProduct(context.Background(), fx.actor, core.ProductInput{CategoryID: &category.ID, Name: "Milk Tea", Slug: "milk-tea", Status: core.StatusActive, Variants: []core.VariantInput{{SKU: "MT-1", Name: "Regular", PriceMinor: 150000, Currency: "NGN", Status: core.StatusActive}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.UpsertInventory(context.Background(), fx.actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: product.Variants[0].ID, OnHand: 20, ReorderThreshold: 1}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		cart, err := fx.commerce.CreateCart(context.Background(), fx.actor, core.CartInput{StoreID: store.ID, CustomerID: customer.ID, Currency: "NGN"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fx.commerce.AddCartItem(context.Background(), fx.actor, cart.ID, core.CartItemInput{VariantID: product.Variants[0].ID, Quantity: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.commerce.CreateOrder(context.Background(), fx.actor, core.OrderInput{CartID: &cart.ID, StoreID: store.ID, CustomerID: customer.ID, FulfilmentType: core.FulfilmentPickup, Currency: "NGN", IdempotencyKey: "track-multi-" + strings.Repeat("x", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	sendSelf := func(id, text string) RuntimeResult {
		t.Helper()
		result, err := fx.service.ProcessMessage(context.Background(), inbound(fx.channel.ID, id, "conv-multi-track", text))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	sendSelf("m1", "hi")
	list := sendSelf("m2", "2")
	listText := joinMessageTexts(list.Messages)
	if !strings.Contains(listText, "Which order would you like to track") {
		t.Fatalf("expected a selectable order list, got %+v", list.Messages)
	}
	if strings.Contains(listText, customer.ID.String()) {
		t.Fatalf("customer-facing track list exposed an internal id: %+v", list.Messages)
	}
	picked := sendSelf("m3", "1")
	if !strings.Contains(joinMessageTexts(picked.Messages), "Here is the latest status") {
		t.Fatalf("expected selected order status, got %+v", picked.Messages)
	}
}

func publishedSelfServiceRuntime(t *testing.T) runtimeFixture {
	t.Helper()
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Done."}}}
	fx := newRuntimeFixture(t, config)
	botService := bot.NewService(fx.db)
	created, err := botService.CreateSelfServiceBot(context.Background(), fx.actor, bot.SelfServiceBotInput{Name: "Customer Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := botService.ListVersions(context.Background(), fx.actor, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := botService.PublishVersion(context.Background(), fx.actor, versions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Update("config", `{"bot_id":"`+created.ID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	return fx
}
