package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai/provider"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeChatProvider struct {
	requests []provider.ChatRequest
	replies  []provider.Message
}

func (f *fakeChatProvider) Name() string  { return "fake" }
func (f *fakeChatProvider) Model() string { return "fake-model" }
func (f *fakeChatProvider) Complete(_ context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
	f.requests = append(f.requests, req)
	if len(f.replies) > 0 {
		reply := f.replies[0]
		f.replies = f.replies[1:]
		return provider.ChatResponse{Message: reply}, nil
	}
	return provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: "Yes, this came from the AI provider."}}, nil
}

func newTestService(t *testing.T, chat *fakeChatProvider) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&core.Channel{}, &core.Customer{}, &core.Store{}, &core.StoreHour{}, &core.StoreFulfilmentMode{}, &core.Category{}, &core.Product{}, &core.Variant{}, &core.ProductImage{}, &core.InventoryLevel{}, &bot.Bot{}, &bot.CommerceWorkflowConfiguration{}, &bot.BotVersion{}, &bot.FAQ{}, &bot.KnowledgeEntry{}, &bot.DocumentSource{}, &bot.DocumentChunk{}, &runtime.ConversationSession{}, &core.ConversationOrderLink{}, &runtime.ConversationMessage{}, &runtime.SupportHandoff{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return NewService(db, nil, chat, 2), db
}

func TestStartRejectsCrossTenantMerchantRequest(t *testing.T) {
	service, _ := newTestService(t, &fakeChatProvider{})
	actorOrg := uuid.New()
	otherOrg := uuid.New()
	_, err := service.Start(context.Background(), auth.CurrentUser{ID: uuid.New(), OrganizationID: actorOrg, Role: authz.MerchantAdmin}, StartInput{OrganizationID: otherOrg})
	if err == nil {
		t.Fatal("expected cross-tenant start to be rejected")
	}
}

func TestMessagePersistsConversationAndUsesProvider(t *testing.T) {
	chat := &fakeChatProvider{}
	service, db := newTestService(t, chat)
	orgID := uuid.New()
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: orgID, Role: authz.MerchantAdmin}

	session, err := service.Start(context.Background(), actor, StartInput{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	response, err := service.Message(context.Background(), actor, MessageInput{SessionID: session.SessionID, Text: "What products do you have?"})
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	if response.Debug.Provider != "fake" || response.Debug.Model != "fake-model" {
		t.Fatalf("unexpected debug provider/model: %#v", response.Debug)
	}
	if !strings.Contains(response.Message.Body, "AI provider") {
		t.Fatalf("unexpected assistant body %q", response.Message.Body)
	}
	if len(chat.requests) != 1 {
		t.Fatalf("expected one provider request, got %d", len(chat.requests))
	}

	var count int64
	if err := db.Model(&runtime.ConversationMessage{}).Where("organization_id = ? AND session_id = ?", orgID, session.SessionID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected inbound and outbound messages, got %d", count)
	}
}

func TestMessagePausesWhenSupportHandoffIsActive(t *testing.T) {
	chat := &fakeChatProvider{}
	service, db := newTestService(t, chat)
	orgID := uuid.New()
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: orgID, Role: authz.MerchantAdmin}

	session, err := service.Start(context.Background(), actor, StartInput{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := db.Create(&runtime.SupportHandoff{ID: uuid.New(), OrganizationID: orgID, SessionID: session.SessionID, Status: "assigned", AssignedUserID: &actor.ID, Reason: "Human owns this", Priority: "normal", Metadata: "{}"}).Error; err != nil {
		t.Fatalf("seed handoff: %v", err)
	}
	response, err := service.Message(context.Background(), actor, MessageInput{SessionID: session.SessionID, Text: "Are you there?"})
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	if response.Debug.Error != "ai_paused_human_handoff_active" {
		t.Fatalf("expected AI pause debug marker, got %#v", response.Debug)
	}
	if len(chat.requests) != 0 {
		t.Fatalf("expected provider not to be called while human handoff is active, got %d calls", len(chat.requests))
	}
	var count int64
	if err := db.Model(&runtime.ConversationMessage{}).Where("organization_id = ? AND session_id = ?", orgID, session.SessionID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected only inbound message to be stored, got %d", count)
	}
}

func TestMessageExecutesModelSelectedReadOnlyTool(t *testing.T) {
	orgID := uuid.New()
	chat := &fakeChatProvider{replies: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{{Function: provider.ToolCallFunction{Name: "get_products", Arguments: map[string]any{}}}}},
		{Role: "assistant", Content: "We have Strawberry Tea for NGN 2,500."},
	}}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&core.Channel{}, &core.Customer{}, &core.Store{}, &core.StoreHour{}, &core.StoreFulfilmentMode{}, &core.Category{}, &core.Product{}, &core.Variant{}, &core.ProductImage{}, &core.InventoryLevel{}, &bot.Bot{}, &bot.CommerceWorkflowConfiguration{}, &bot.BotVersion{}, &bot.FAQ{}, &bot.KnowledgeEntry{}, &bot.DocumentSource{}, &bot.DocumentChunk{}, &runtime.ConversationSession{}, &core.ConversationOrderLink{}, &runtime.ConversationMessage{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	commerce := core.NewService(db, core.SafeTestProvider{})
	service := NewService(db, commerce, chat, 2)
	productID := uuid.New()
	if err := db.Create(&core.Product{ID: productID, OrganizationID: orgID, Name: "Strawberry Tea", Slug: "strawberry-tea", Description: "Fresh fruit tea", Status: core.StatusActive, Metadata: "{}"}).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := db.Create(&core.Variant{ID: uuid.New(), OrganizationID: orgID, ProductID: productID, SKU: "STRAWBERRY-TEA", Name: "Regular", PriceMinor: 250000, Currency: "NGN", Status: core.StatusActive, Metadata: "{}"}).Error; err != nil {
		t.Fatalf("seed variant: %v", err)
	}

	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: orgID, Role: authz.MerchantAdmin}
	session, err := service.Start(context.Background(), actor, StartInput{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	response, err := service.Message(context.Background(), actor, MessageInput{SessionID: session.SessionID, Text: "Do you have strawberry tea?"})
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	foundProducts := false
	for _, call := range response.Debug.Tools {
		if call.Name == "products" && call.Status == "ok" {
			foundProducts = true
		}
	}
	if !foundProducts {
		t.Fatalf("expected deterministic product grounding, got %#v", response.Debug.Tools)
	}
	if !strings.Contains(response.Message.Body, "Strawberry Tea") {
		t.Fatalf("expected final answer to mention seeded product, got %q", response.Message.Body)
	}
	if len(chat.requests) == 0 || len(chat.requests) > 2 {
		t.Fatalf("expected one generation call and optional retry, got %d", len(chat.requests))
	}
}
