package runtime

import (
	"context"
	"errors"
	"strings"
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

func TestInstagramAndFacebookInboundUseSharedAIAndOutboundRuntime(t *testing.T) {
	for _, provider := range []string{"instagram", "facebook"} {
		t.Run(provider, func(t *testing.T) {
			config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "start"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "start", Type: bot.StepEnd, Title: "Fallback", Message: "Fallback menu."}}}
			fx := newRuntimeFixture(t, config)
			if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Updates(map[string]any{"provider": provider, "status": channelplatform.StatusConnected}).Error; err != nil {
				t.Fatal(err)
			}
			fx.service.ConfigureAIInbound(func(_ context.Context, _ ConversationSession, text string) (AIInboundResponse, error) {
				if text != "What do you sell?" {
					t.Fatalf("unexpected %s inbound text %q", provider, text)
				}
				return AIInboundResponse{Reply: "We sell fruit tea and milk tea.", Variables: `{"ai_state":{"current_intent":"commerce"}}`}, nil
			})
			sender := &MockChannelSender{}
			fx.service.RegisterChannelSender(provider, sender)
			event := channelplatform.InboundEvent{
				Provider: provider, ChannelConnectionID: fx.channel.ID, ProviderEventID: provider + "-event-1",
				ExternalCustomerID: "customer-1", ExternalMessageID: provider + "-message-1", ExternalConversationID: "customer-1",
				MessageType: "text", Body: "What do you sell?", Timestamp: time.Now().UTC(),
			}

			if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err != nil {
				t.Fatal(err)
			}
			if len(sender.Sent) != 1 || sender.Sent[0]["recipient"] != "customer-1" {
				t.Fatalf("%s did not dispatch through the shared outbound boundary: %+v", provider, sender.Sent)
			}
			payload, ok := sender.Sent[0]["payload"].(map[string]any)
			if !ok || payload["text"] != "We sell fruit tea and milk tea." {
				t.Fatalf("%s lost the shared AI response: %+v", provider, sender.Sent[0])
			}
			var session ConversationSession
			if err := fx.db.Where("organization_id = ? AND channel_id = ? AND external_conversation_id = ?", fx.actor.OrganizationID, fx.channel.ID, "customer-1").First(&session).Error; err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(session.Variables, "commerce") {
				t.Fatalf("%s did not persist shared AI state: %s", provider, session.Variables)
			}
		})
	}
}

func TestInstagramAndFacebookCustomerIdentitiesRemainChannelScoped(t *testing.T) {
	config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "done"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "done", Type: bot.StepEnd, Title: "Done", Message: "Welcome."}}}
	fx := newRuntimeFixture(t, config)
	if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Updates(map[string]any{"provider": "instagram", "status": channelplatform.StatusConnected}).Error; err != nil {
		t.Fatal(err)
	}
	facebook := core.Channel{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Provider: "facebook", DisplayName: "Facebook", Status: channelplatform.StatusConnected, Config: fx.channel.Config, SecretConfig: "{}"}
	if err := fx.db.Create(&facebook).Error; err != nil {
		t.Fatal(err)
	}
	fx.service.RegisterChannelSender("instagram", &MockChannelSender{})
	fx.service.RegisterChannelSender("facebook", &MockChannelSender{})
	for _, input := range []struct {
		provider  string
		channelID uuid.UUID
	}{
		{provider: "instagram", channelID: fx.channel.ID},
		{provider: "facebook", channelID: facebook.ID},
	} {
		event := channelplatform.InboundEvent{Provider: input.provider, ChannelConnectionID: input.channelID, ProviderEventID: "shared-event", ExternalCustomerID: "same-provider-id", ExternalMessageID: "shared-message", ExternalConversationID: "same-provider-id", MessageType: "text", Body: "Hello", Timestamp: time.Now().UTC()}
		if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err != nil {
			t.Fatal(err)
		}
	}
	var sessions []ConversationSession
	if err := fx.db.Where("organization_id = ? AND external_conversation_id = ?", fx.actor.OrganizationID, "same-provider-id").Order("channel_id").Find(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].ChannelID == sessions[1].ChannelID {
		t.Fatalf("cross-channel identities were merged: %+v", sessions)
	}
	var processed int64
	if err := fx.db.Model(&ProcessedMessage{}).Where("organization_id = ? AND external_message_id = ?", fx.actor.OrganizationID, "shared-message").Count(&processed).Error; err != nil {
		t.Fatal(err)
	}
	if processed != 2 {
		t.Fatalf("provider message idempotency was not channel scoped, got %d records", processed)
	}
}

func TestInstagramAndFacebookInboundUseSharedHumanHandoff(t *testing.T) {
	for _, provider := range []string{"instagram", "facebook"} {
		t.Run(provider, func(t *testing.T) {
			config := bot.VersionConfiguration{Version: bot.BotVersion{StartStepKey: "handoff"}, Steps: []bot.Step{{ID: uuid.New(), StepKey: "handoff", Type: bot.StepHandoff, Title: "Handoff", Message: "Connecting you."}}}
			fx := newRuntimeFixture(t, config)
			if err := fx.db.Model(&core.Channel{}).Where("id = ?", fx.channel.ID).Updates(map[string]any{"provider": provider, "status": channelplatform.StatusConnected}).Error; err != nil {
				t.Fatal(err)
			}
			fx.service.RegisterChannelSender(provider, &MockChannelSender{})
			event := channelplatform.InboundEvent{Provider: provider, ChannelConnectionID: fx.channel.ID, ProviderEventID: provider + "-handoff-event", ExternalCustomerID: "customer-handoff", ExternalMessageID: provider + "-handoff-message", ExternalConversationID: "customer-handoff", MessageType: "text", Body: "I need a person", Timestamp: time.Now().UTC()}
			if err := fx.service.ProcessChannelInbound(context.Background(), fx.actor.OrganizationID, event); err != nil {
				t.Fatal(err)
			}
			var handoff SupportHandoff
			if err := fx.db.Where("organization_id = ? AND status = ?", fx.actor.OrganizationID, "open").First(&handoff).Error; err != nil {
				t.Fatal(err)
			}
			var session ConversationSession
			if err := fx.db.Where("id = ? AND channel_id = ? AND status = ?", handoff.SessionID, fx.channel.ID, SessionHandoff).First(&session).Error; err != nil {
				t.Fatalf("%s handoff did not preserve the channel conversation: %v", provider, err)
			}
		})
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

func TestCapabilityAwareSenderBlocksUnsupportedMessageBeforeProviderCall(t *testing.T) {
	parent := &recordingOutboundSender{}
	checker := &recordingCapabilityChecker{err: errors.New("capability unavailable")}
	sender := NewCapabilityAwareAdapterChannelSender(parent, checker)
	channel := core.Channel{ID: uuid.New(), OrganizationID: uuid.New(), Provider: "tiktok"}
	_, err := sender.Send(context.Background(), channel, "customer", OutboundMessage{Type: MessageText, Text: "Hello"}, nil, "delivery-unsupported")
	if err == nil || checker.capability != "outbound_text" {
		t.Fatalf("expected outbound text capability rejection, capability=%q err=%v", checker.capability, err)
	}
	if parent.command.ChannelConnectionID != uuid.Nil {
		t.Fatal("provider sender must not be called for an unsupported capability")
	}
}

type recordingCapabilityChecker struct {
	capability string
	err        error
}

func (c *recordingCapabilityChecker) RequireCapability(_ context.Context, _, _ uuid.UUID, capability string) error {
	c.capability = capability
	return c.err
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
