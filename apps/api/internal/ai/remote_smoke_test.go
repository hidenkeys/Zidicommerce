package ai

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai/provider"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRemoteOllamaToolSmoke(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("AI_REMOTE_SMOKE_DATABASE_URL"))
	if dsn == "" {
		t.Skip("AI_REMOTE_SMOKE_DATABASE_URL is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect remote database: %v", err)
	}
	if !remoteColumnExists(t, db, "products", "organization_id") {
		t.Fatalf("remote database is not a compatible ZidiCommerce schema: products.organization_id is missing")
	}
	if !remoteColumnExists(t, db, "conversation_sessions", "organization_id") {
		t.Fatalf("remote database is not a compatible ZidiCommerce schema: conversation_sessions.organization_id is missing")
	}
	var orgIDText string
	err = db.Table("organizations").
		Select("organizations.id").
		Joins("JOIN products ON products.organization_id = organizations.id").
		Where("organizations.status = ? AND products.status = ?", core.StatusActive, core.StatusActive).
		Order("organizations.created_at DESC").
		Limit(1).
		Scan(&orgIDText).Error
	if err != nil {
		t.Fatalf("find organization with products: %v", err)
	}
	orgID, err := uuid.Parse(orgIDText)
	if err != nil {
		t.Fatalf("parse organization ID %q: %v", orgIDText, err)
	}
	if orgID == uuid.Nil {
		t.Skip("remote database has no active organization with active products")
	}

	commerce := core.NewService(db, core.SafeTestProvider{})
	chat := provider.NewOllamaChatProvider(os.Getenv("OLLAMA_BASE_URL"), os.Getenv("OLLAMA_CHAT_MODEL"))
	service := NewService(db, commerce, chat, 4)
	actor := auth.CurrentUser{ID: uuid.Nil, OrganizationID: orgID, Role: authz.MerchantAdmin}

	session, err := service.Start(context.Background(), actor, StartInput{OrganizationID: orgID})
	if err != nil {
		t.Fatalf("start AI smoke session: %v", err)
	}
	response, err := service.Message(context.Background(), actor, MessageInput{SessionID: session.SessionID, Text: "What products do you have?"})
	if err != nil {
		t.Fatalf("send AI smoke message: %v", err)
	}
	if len(response.Debug.Tools) == 0 {
		t.Fatalf("expected deterministic grounding debug, debug=%#v response=%q", response.Debug, response.Message.Body)
	}
	found := false
	for _, call := range response.Debug.Tools {
		if call.Name == "products" && call.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected product grounding, got %#v", response.Debug.Tools)
	}
	if strings.TrimSpace(response.Message.Body) == "" {
		t.Fatal("expected non-empty AI response")
	}
}

func remoteColumnExists(t *testing.T, db *gorm.DB, tableName string, columnName string) bool {
	t.Helper()
	var count int64
	err := db.Table("information_schema.columns").
		Where("table_schema = current_schema() AND table_name = ? AND column_name = ?", tableName, columnName).
		Count(&count).Error
	if err != nil {
		t.Fatalf("inspect remote schema: %v", err)
	}
	return count > 0
}
