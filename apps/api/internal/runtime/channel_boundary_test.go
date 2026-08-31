package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

func TestChannelBoundaryConvertsNormalizedInboundWithoutProviderAssumptions(t *testing.T) {
	channelID := uuid.New()
	event := channelplatform.InboundEvent{
		Provider: "instagram", ChannelConnectionID: channelID, ProviderEventID: "event-1",
		ExternalCustomerID: "customer-1", ExternalMessageID: "message-1", ExternalConversationID: "conversation-1",
		MessageType: "text", Body: "Do you have this in stock?", Timestamp: time.Now().UTC(),
		Media: []channelplatform.MediaReference{{Type: "image", Reference: "provider-media://image-1"}},
	}
	inbound := InboundMessageFromChannelEvent(event)
	if inbound.ChannelID == nil || *inbound.ChannelID != channelID || inbound.Provider != "instagram" {
		t.Fatalf("channel identity was not preserved: %+v", inbound)
	}
	if inbound.Sender != "customer-1" || inbound.Text != event.Body || len(inbound.Attachments) != 1 {
		t.Fatalf("message content was not preserved: %+v", inbound)
	}
}

func TestChannelBoundaryBuildsGenericOutboundCommand(t *testing.T) {
	channelID := uuid.New()
	conversationID := uuid.New()
	command := ChannelOutboundCommand("web", channelID, &conversationID, "customer-1", "conversation-1", OutboundMessage{Type: MessageText, Text: "Your order is ready."}, "outbound-1")
	if command.Provider != "web" || command.ChannelConnectionID != channelID || command.ConversationID == nil || *command.ConversationID != conversationID {
		t.Fatalf("unexpected outbound command identity: %+v", command)
	}
	if command.ExternalCustomerID != "customer-1" || command.Body != "Your order is ready." || command.IdempotencyKey != "outbound-1" {
		t.Fatalf("unexpected outbound command body: %+v", command)
	}
}

func TestProcessChannelInboundEnforcesTenantAndPreservesIdempotency(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Welcome."}}}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Updates(map[string]any{"provider": "whatsapp", "status": channelplatform.StatusConnected}).Error; err != nil {
		t.Fatal(err)
	}
	sender := &MockChannelSender{}
	fx.service.RegisterChannelSender("whatsapp", sender)
	event := channelplatform.InboundEvent{Provider: "whatsapp", ChannelConnectionID: fx.channel.ID, ProviderEventID: "wamid-runtime", ExternalCustomerID: "2348000000000", ExternalMessageID: "wamid-runtime", ExternalConversationID: "2348000000000", MessageType: "text", Body: "Hello", Timestamp: time.Now().UTC()}

	if err := fx.service.ProcessChannelInbound(context.Background(), uuid.New(), event); err == nil {
		t.Fatal("cross-tenant channel event must be rejected")
	}
	if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err != nil {
		t.Fatal(err)
	}
	if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err != nil {
		t.Fatal(err)
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("duplicate inbound dispatched %d outbound messages", len(sender.Sent))
	}
	var processed int64
	if err := fx.db.Model(&ProcessedMessage{}).Where("organization_id = ? AND channel_id = ? AND external_message_id = ?", fx.actor.OrganizationID, fx.channel.ID, event.ExternalMessageID).Count(&processed).Error; err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected one tenant-scoped processed message, got %d", processed)
	}
}

func TestProcessChannelInboundRetriesFailedRuntimeMessageAfterSnapshotRepair(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Done", Message: "Welcome."}}}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Updates(map[string]any{"provider": "whatsapp", "status": channelplatform.StatusConnected}).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot bot.PublishedSnapshot
	if err := fx.db.Where("organization_id = ? AND version_id = ?", fx.actor.OrganizationID, fx.version.ID).First(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Delete(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	sender := &MockChannelSender{}
	fx.service.RegisterChannelSender("whatsapp", sender)
	event := channelplatform.InboundEvent{Provider: "whatsapp", ChannelConnectionID: fx.channel.ID, ProviderEventID: "wamid-retry", ExternalCustomerID: "2348000000000", ExternalMessageID: "wamid-retry", ExternalConversationID: "2348000000000", MessageType: "text", Body: "Hello", Timestamp: time.Now().UTC()}

	if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err == nil {
		t.Fatal("missing published snapshot should fail the first runtime attempt")
	}
	var failed ProcessedMessage
	if err := fx.db.Where("organization_id = ? AND channel_id = ? AND external_message_id = ?", fx.actor.OrganizationID, fx.channel.ID, event.ExternalMessageID).First(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" || failed.SessionID != nil {
		t.Fatalf("expected failed runtime record without a session, got %+v", failed)
	}
	snapshot.ID = uuid.New()
	if err := fx.db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err != nil {
		t.Fatalf("failed runtime message was not retried after repair: %v", err)
	}
	if err := fx.db.Where("id = ?", failed.ID).First(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.Status != "processed" || failed.SessionID == nil {
		t.Fatalf("expected retried runtime record to complete with a session, got %+v", failed)
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("expected one outbound response after retry, got %d", len(sender.Sent))
	}
}

func TestAdapterChannelSenderMapsGenericRuntimeMessage(t *testing.T) {
	parent := &recordingOutboundSender{result: channelplatform.SendResult{ProviderMessageID: "provider-1", ResponseMetadata: map[string]any{"status_code": 200}}}
	sender := NewAdapterChannelSender(parent)
	channel := core.Channel{ID: uuid.New(), OrganizationID: uuid.New(), Provider: "whatsapp"}
	payload := map[string]any{"type": MessageButtons, "text": "Choose", "options": []map[string]any{{"id": "buy", "label": "Buy"}}}
	message := outboundMessageFromPayload(MessageButtons, payload)
	result, err := sender.Send(context.Background(), channel, "2348000000000", message, payload, "delivery-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderMessageID != "provider-1" || parent.command.Body != "Choose" || parent.command.IdempotencyKey != "delivery-1" || len(parent.command.Actions) != 1 || parent.command.Actions[0].ID != "buy" {
		t.Fatalf("runtime sender lost provider-neutral command data: result=%+v command=%+v", result, parent.command)
	}
}

type recordingOutboundSender struct {
	command channelplatform.OutboundCommand
	result  channelplatform.SendResult
	err     error
}

func (s *recordingOutboundSender) Send(_ context.Context, command channelplatform.OutboundCommand) (channelplatform.SendResult, error) {
	s.command = command
	if s.err != nil {
		return channelplatform.SendResult{}, s.err
	}
	return s.result, nil
}
