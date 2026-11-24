package runtime

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
)

func TestAIJourneyExplicitHandoffAndRelease(t *testing.T) {
	fx, detail := journeyFixture(t, "order")
	customer, _, _ := seedCommerce(t, fx)
	ctx := context.Background()
	svc := bot.NewService(fx.db)
	d := detail.Definition
	d.Mode = "ai"
	d.Trigger = "ask"
	if _, err := svc.SaveJourney(ctx, fx.actor, detail.Version.ID, d); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishJourney(ctx, fx.actor, detail.Version.ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fx.service.ConfigureAIInbound(func(context.Context, ConversationSession, string) (AIInboundResponse, error) {
		calls++
		return AIInboundResponse{Reply: "Welcome back."}, nil
	})
	send := func(text string) RuntimeResult {
		t.Helper()
		r, err := fx.service.ProcessMessage(ctx, InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "handoff-qa", Text: text, ExternalMessageID: uuid.NewString()})
		if err != nil || r.Error != nil {
			t.Fatalf("unexpected error for %q: %v %+v", text, err, r.Error)
		}
		return r
	}
	send("ask")
	r := send("I want to speak to a human about my order.")
	if r.SessionStatus != SessionHandoff || !r.Handoff || calls != 0 {
		t.Fatal("request did not open human handoff")
	}
	send("What is the price?")
	if calls != 0 {
		t.Fatal("AI replied during handoff")
	}
	handoff, err := fx.service.activeHandoffForSession(ctx, fx.actor, r.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.service.ReleaseSupportHandoff(ctx, fx.actor, handoff.ID, SupportHandoffReleaseInput{ResumeBot: true}); err != nil {
		t.Fatal(err)
	}
	send("Which stores offer pickup?")
	if calls != 1 {
		t.Fatal("AI did not resume")
	}
}

func TestReleasedGuidedJourneyEntryBypassesAI(t *testing.T) {
	for _, trigger := range []string{"guided", "dessert_help"} {
		t.Run(trigger, func(t *testing.T) {
			fx, detail := journeyFixture(t, "guided_order")
			customer, _, _ := seedCommerce(t, fx)
			ctx := context.Background()
			svc := bot.NewService(fx.db)
			d := detail.Definition
			d.Trigger = trigger
			if _, err := svc.SaveJourney(ctx, fx.actor, detail.Version.ID, d); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.PublishJourney(ctx, fx.actor, detail.Version.ID); err != nil {
				t.Fatal(err)
			}
			calls := 0
			fx.service.ConfigureAIInbound(func(context.Context, ConversationSession, string) (AIInboundResponse, error) {
				calls++
				return AIInboundResponse{Reply: "Grounded answer."}, nil
			})
			send := func(text string) RuntimeResult {
				t.Helper()
				r, err := fx.service.ProcessMessage(ctx, InboundMessage{ChannelID: &fx.channel.ID, Sender: customer.Phone, ExternalConversationID: "released-entry", Text: text, ExternalMessageID: uuid.NewString()})
				if err != nil || r.Error != nil {
					t.Fatalf("send %q: %v %+v", text, err, r.Error)
				}
				return r
			}
			started := send(trigger)
			handoff, err := fx.service.RequestSupportHandoff(ctx, fx.actor, started.ConversationID, "Test release")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fx.service.ReleaseSupportHandoff(ctx, fx.actor, handoff.ID, SupportHandoffReleaseInput{ResumeBot: true}); err != nil {
				t.Fatal(err)
			}
			send("What products are available?")
			if calls != 1 {
				t.Fatalf("natural question should reach AI once, got %d", calls)
			}
			restarted := send(trigger)
			if calls != 1 {
				t.Fatalf("published entry keyword reached AI: %d calls", calls)
			}
			stored, err := fx.service.GetConversationDetail(ctx, fx.actor, restarted.ConversationID)
			if err != nil || stored.CurrentStepKey != "goal" || len(restarted.Messages) == 0 {
				t.Fatalf("guided questions did not restart: %+v", restarted)
			}
		})
	}
}
