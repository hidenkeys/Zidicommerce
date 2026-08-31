package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

func TestInboundContactOpensExtendsWindowAndHandlesConsentKeywords(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configuration := configureWhatsApp(t, fx).Configuration
	first := fx.service.now().Add(-time.Hour)
	state, changed, err := fx.service.RecordInboundContact(context.Background(), configuration, inboundContactEvent(fx, "2348000000001", "Hello", first))
	if err != nil || changed || state.ServiceWindowExpiresAt == nil || !state.ServiceWindowExpiresAt.Equal(first.Add(24*time.Hour)) {
		t.Fatalf("inbound did not open the service window: state=%+v changed=%v err=%v", state, changed, err)
	}
	second := fx.service.now()
	state, changed, err = fx.service.RecordInboundContact(context.Background(), configuration, inboundContactEvent(fx, "2348000000001", "stop!", second))
	if err != nil || !changed || state.ConsentStatus != ConsentOptedOut || state.OptedOutAt == nil || !state.ServiceWindowExpiresAt.Equal(second.Add(24*time.Hour)) {
		t.Fatalf("opt-out did not extend the window and update consent: state=%+v changed=%v err=%v", state, changed, err)
	}
	state, changed, err = fx.service.RecordInboundContact(context.Background(), configuration, inboundContactEvent(fx, "2348000000001", "START", second.Add(time.Minute)))
	if err != nil || !changed || state.ConsentStatus != ConsentOptedIn || state.OptedOutAt != nil {
		t.Fatalf("opt-in did not restore consent: state=%+v changed=%v err=%v", state, changed, err)
	}
	for _, message := range []string{"Please stop my order", "yes please", "cancel order 123", "starter kit", "subscribed"} {
		if status, matched := DetectConsentKeyword(message); matched {
			t.Fatalf("normal message %q accidentally matched consent keyword %q", message, status)
		}
	}
}

func TestPolicyAllowsFreeformInsideWindowAndBlocksOutsideWithoutMeta(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configuration := configureWhatsApp(t, fx).Configuration
	openServiceWindow(t, fx, "2348000000000")
	client := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusOK, `{"messages":[{"id":"wamid-policy-freeform"}]}`, nil)}}
	adapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", client, false)
	adapter.now = fx.service.now
	allowed := channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "2348000000000", MessageType: "text", Body: "Within the service window", IdempotencyKey: "policy-freeform-allowed"}
	if _, err := adapter.Send(context.Background(), allowed); err != nil {
		t.Fatalf("free-form send inside the window failed: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("allowed free-form send reached Meta %d times", len(client.requests))
	}
	if err := fx.db.Model(&ContactState{}).Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).Update("service_window_expires_at", fx.service.now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	blocked := allowed
	blocked.IdempotencyKey = "policy-freeform-blocked"
	_, err := adapter.Send(context.Background(), blocked)
	var policyErr PolicyBlockedError
	if !errors.As(err, &policyErr) || policyErr.Decision.Decision != PolicyBlockedWindowClosed {
		t.Fatalf("closed service window returned wrong error: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatal("policy-blocked send called Meta")
	}
	var record PolicyDecisionRecord
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ? AND idempotency_key = ?", fx.actor.OrganizationID, fx.connection.ID, blocked.IdempotencyKey).First(&record).Error; err != nil || record.Allowed || record.Decision != PolicyBlockedWindowClosed {
		t.Fatalf("policy decision was not safely persisted: %+v err=%v", record, err)
	}
	metrics, err := fx.service.GetAdvancedMetrics(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || metrics.FreeformSends != 1 || metrics.PolicyBlockedSends != 1 || metrics.ServiceWindowClosedBlocks != 1 || metrics.FailedSends != 0 || metrics.ProviderErrors != 0 {
		t.Fatalf("advanced policy metrics are incorrect: %+v err=%v", metrics, err)
	}
	var policyEvent channelplatform.ProviderEvent
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ? AND provider_event_id = ?", fx.actor.OrganizationID, fx.connection.ID, "policy:"+blocked.IdempotencyKey).First(&policyEvent).Error; err != nil || policyEvent.EventType != "policy_blocked" {
		t.Fatalf("policy block was not separated from provider failures: %+v err=%v", policyEvent, err)
	}
	_ = configuration
}

func TestApprovedTemplateOutsideWindowAndTemplateValidation(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	template := createApprovedTemplate(t, fx, "order_update", []string{"name"})
	client := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusOK, `{"messages":[{"id":"wamid-template"}]}`, nil)}}
	adapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", client, false)
	adapter.now = fx.service.now
	command := channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "2348000000000", MessageType: "template", Template: &channelplatform.TemplateReference{ID: template.ID, Variables: map[string]string{"name": "Ada"}}, IdempotencyKey: "template-approved"}
	if _, err := adapter.Send(context.Background(), command); err != nil {
		t.Fatalf("approved template outside window failed: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("approved template reached Meta %d times", len(client.requests))
	}
	body, _ := io.ReadAll(client.requests[0].Body)
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	if payload["type"] != "template" || strings.Contains(string(body), template.ID.String()) {
		t.Fatalf("unexpected or unsafe template payload: %s", body)
	}
	missing := command
	missing.IdempotencyKey = "template-missing-variable"
	missing.Template = &channelplatform.TemplateReference{ID: template.ID, Variables: map[string]string{}}
	_, err := adapter.Send(context.Background(), missing)
	var policyErr PolicyBlockedError
	if !errors.As(err, &policyErr) || policyErr.Decision.Decision != PolicyBlockedTemplateVariables {
		t.Fatalf("missing variables returned wrong result: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatal("invalid template variables called Meta")
	}
}

func TestNonApprovedTemplateStatesAreBlocked(t *testing.T) {
	for _, status := range []string{TemplateDraft, TemplatePending, TemplateRejected, TemplatePaused, TemplateDisabled, TemplateArchived} {
		t.Run(status, func(t *testing.T) {
			fx := newWhatsAppFixture(t)
			configuration := configureWhatsApp(t, fx).Configuration
			view, err := fx.service.CreateTemplate(context.Background(), fx.actor, fx.connection.ID, MessageTemplateInput{Name: "status_" + status, Language: "en", Category: "utility", Status: status, Body: "Status update"})
			if err != nil {
				t.Fatal(err)
			}
			decision, _, err := fx.service.EvaluateAndRecordPolicy(context.Background(), configuration, channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "2348000000000", MessageType: "template", Template: &channelplatform.TemplateReference{ID: view.ID}, IdempotencyKey: "status-" + status})
			if err != nil || decision.Allowed || decision.Decision != PolicyBlockedTemplateNotApproved {
				t.Fatalf("status %s was not blocked: %+v err=%v", status, decision, err)
			}
		})
	}
}

func TestOptedOutAndMissingContactAreBlocked(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configuration := configureWhatsApp(t, fx).Configuration
	decision, _, err := fx.service.EvaluateAndRecordPolicy(context.Background(), configuration, channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "2348000000000", MessageType: "text", IdempotencyKey: "missing-contact"})
	if err != nil || decision.Decision != PolicyBlockedNoContactState {
		t.Fatalf("missing contact returned %+v err=%v", decision, err)
	}
	state := openServiceWindow(t, fx, "2348000000000")
	if _, err := fx.service.UpdateContactConsent(context.Background(), fx.actor, fx.connection.ID, state.ID, ContactStateInput{ConsentStatus: ConsentOptedOut, ConsentSource: ConsentSourceManual}); err != nil {
		t.Fatal(err)
	}
	decision, _, err = fx.service.EvaluateAndRecordPolicy(context.Background(), configuration, channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "2348000000000", MessageType: "text", IdempotencyKey: "opted-out"})
	if err != nil || decision.Decision != PolicyBlockedOptedOut {
		t.Fatalf("opted-out contact returned %+v err=%v", decision, err)
	}
}

func TestTemplateAndContactTenantIsolationAndPermissions(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	template := createApprovedTemplate(t, fx, "tenant_template", nil)
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	if _, err := fx.service.CreateTemplate(context.Background(), viewer, fx.connection.ID, MessageTemplateInput{Name: "forbidden", Language: "en", Category: "utility", Body: "No"}); err == nil {
		t.Fatal("viewer created a template")
	}
	if _, err := fx.service.GetTemplate(context.Background(), fx.other, fx.connection.ID, template.ID); err == nil {
		t.Fatal("cross-tenant template lookup succeeded")
	}
	state := openServiceWindow(t, fx, "2348000000000")
	if _, err := fx.service.UpdateContactConsent(context.Background(), fx.other, fx.connection.ID, state.ID, ContactStateInput{ConsentStatus: ConsentOptedOut}); err == nil {
		t.Fatal("cross-tenant contact update succeeded")
	}
	contacts, err := fx.service.ListContactStates(context.Background(), fx.actor, fx.connection.ID, "", 10)
	if err != nil || len(contacts) != 1 || contacts[0].MaskedPhone != "****0000" || contacts[0].ExternalCustomerIDHash == "" {
		t.Fatalf("tenant contact list was not safely returned: %+v err=%v", contacts, err)
	}
	body, _ := json.Marshal(contacts)
	if strings.Contains(string(body), "2348000000000") || strings.Contains(string(body), contacts[0].ExternalCustomerIDHash) {
		t.Fatal("contact API representation exposed a raw or hashed external identifier")
	}
}

func TestTemplatePreviewArchiveAndManualStatusLifecycle(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	view, err := fx.service.CreateTemplate(context.Background(), fx.actor, fx.connection.ID, MessageTemplateInput{Name: "delivery_notice", Language: "en_GB", Category: "utility", Status: TemplateDraft, Body: "Hi {{name}}, order {{2}} is ready.", VariableSchema: []string{"name", "order_code"}, SampleValues: map[string]string{"name": "Ada", "order_code": "ZC-100"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := fx.service.PreviewTemplate(context.Background(), fx.actor, fx.connection.ID, view.ID, nil)
	if err != nil || !preview.Valid || preview.Body != "Hi Ada, order ZC-100 is ready." {
		t.Fatalf("unexpected template preview: %+v err=%v", preview, err)
	}
	approved := TemplateApproved
	view, err = fx.service.UpdateTemplate(context.Background(), fx.actor, fx.connection.ID, view.ID, MessageTemplateUpdate{Status: &approved})
	if err != nil || view.Status != TemplateApproved {
		t.Fatalf("manual status update failed: %+v err=%v", view, err)
	}
	view, err = fx.service.ArchiveTemplate(context.Background(), fx.actor, fx.connection.ID, view.ID)
	if err != nil || view.Status != TemplateArchived {
		t.Fatalf("template archive failed: %+v err=%v", view, err)
	}
}

func TestConversationPolicyContextIsTenantScopedAndTracksWindow(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	if err := fx.db.Exec(`CREATE TABLE conversation_sessions (
		id TEXT PRIMARY KEY,
		organization_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		external_conversation_id TEXT NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	conversationID := uuid.New()
	recipient := "2348000000000"
	if err := fx.db.Exec("INSERT INTO conversation_sessions (id, organization_id, channel_id, external_conversation_id) VALUES (?, ?, ?, ?)", conversationID, fx.actor.OrganizationID, fx.connection.ID, recipient).Error; err != nil {
		t.Fatal(err)
	}
	state := openServiceWindow(t, fx, recipient)

	policyContext, err := fx.service.GetConversationPolicyContext(context.Background(), fx.actor, fx.connection.ID, conversationID)
	if err != nil || !policyContext.CanSendFreeform || !policyContext.ServiceWindowOpen || policyContext.TemplateRequired || policyContext.MaskedPhone != "****0000" {
		t.Fatalf("open conversation policy context is incorrect: %+v err=%v", policyContext, err)
	}
	if _, err := fx.service.GetConversationPolicyContext(context.Background(), fx.other, fx.connection.ID, conversationID); err == nil {
		t.Fatal("cross-tenant conversation policy context lookup succeeded")
	}
	if err := fx.db.Model(&ContactState{}).Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).Update("service_window_expires_at", fx.service.now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	policyContext, err = fx.service.GetConversationPolicyContext(context.Background(), fx.actor, fx.connection.ID, conversationID)
	if err != nil || policyContext.CanSendFreeform || policyContext.ServiceWindowOpen || !policyContext.TemplateRequired {
		t.Fatalf("closed conversation policy context is incorrect: %+v err=%v", policyContext, err)
	}
	if _, err := fx.service.UpdateContactConsent(context.Background(), fx.actor, fx.connection.ID, state.ID, ContactStateInput{ConsentStatus: ConsentOptedOut, ConsentSource: ConsentSourceManual}); err != nil {
		t.Fatal(err)
	}
	policyContext, err = fx.service.GetConversationPolicyContext(context.Background(), fx.actor, fx.connection.ID, conversationID)
	if err != nil || policyContext.CanSendFreeform || policyContext.TemplateRequired || policyContext.ConsentStatus != ConsentOptedOut {
		t.Fatalf("opted-out conversation policy context is incorrect: %+v err=%v", policyContext, err)
	}
}

func inboundContactEvent(fx whatsappFixture, recipient, body string, at time.Time) channelplatform.InboundEvent {
	id := uuid.NewString()
	return channelplatform.InboundEvent{Provider: Provider, ChannelConnectionID: fx.connection.ID, ProviderEventID: id, ExternalMessageID: id, ExternalCustomerID: recipient, Body: body, Timestamp: at}
}
