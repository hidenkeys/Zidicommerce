package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCheckoutNotificationsUseConversationChannel(t *testing.T) {
	fx := newCommerceFixture(t, 2)
	ctx := context.Background()
	if err := fx.db.Exec("CREATE TABLE conversation_sessions (id UUID PRIMARY KEY, organization_id UUID NOT NULL, customer_id UUID, channel_id UUID NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	older := Channel{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Provider: "whatsapp", Status: StatusActive, CreatedAt: time.Now().Add(-time.Hour)}
	checkout := Channel{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Provider: "whatsapp", Status: "healthy"}
	if err := fx.db.Create(&[]Channel{older, checkout}).Error; err != nil {
		t.Fatal(err)
	}
	order, err := fx.service.CreateOrder(ctx, fx.actor, OrderInput{
		StoreID: fx.store.ID, CustomerID: fx.customer.ID, FulfilmentType: FulfilmentPickup,
		Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := uuid.New()
	if err := fx.db.Exec("INSERT INTO conversation_sessions (id, organization_id, customer_id, channel_id) VALUES (?, ?, ?, ?)",
		sessionID, order.OrganizationID, order.CustomerID, checkout.ID).Error; err != nil {
		t.Fatal(err)
	}
	link := ConversationOrderLink{
		ID: uuid.New(), OrganizationID: order.OrganizationID, OrderID: order.ID,
		CustomerID: order.CustomerID, StoreID: order.StoreID, ConversationSessionID: sessionID,
	}
	if err := fx.db.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.service.recordOrderNotificationTx(fx.db, order.OrganizationID, order.ID, "payment_confirmed", "Test"); err != nil {
		t.Fatal(err)
	}
	var notification CommerceNotification
	if err := fx.db.Where("order_id = ? AND notification_type = ?", order.ID, "payment_confirmed").First(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if notification.ChannelID == nil || *notification.ChannelID != checkout.ID {
		t.Fatalf("expected checkout channel %s, got %v", checkout.ID, notification.ChannelID)
	}
	if err := fx.db.Exec("UPDATE conversation_sessions SET organization_id = ? WHERE id = ?", uuid.New(), sessionID).Error; err != nil {
		t.Fatal(err)
	}
	channelID, err := orderNotificationChannel(fx.db, order)
	if err != nil || channelID != nil {
		t.Fatalf("cross-tenant session should not resolve or fall back, got %v, %v", channelID, err)
	}
}
