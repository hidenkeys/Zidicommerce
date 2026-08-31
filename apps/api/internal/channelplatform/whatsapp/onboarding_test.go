package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
)

func TestPhaseLOnboardingLifecycleAndControlledMessageFlow(t *testing.T) {
	fx := newWhatsAppFixture(t)
	initial, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || initial.SetupState != SetupNotConnected {
		t.Fatalf("unexpected initial setup: state=%q err=%v", initial.SetupState, err)
	}
	configured := configureWhatsApp(t, fx)
	if configured.SetupState != SetupCredentialsAdded {
		t.Fatalf("credentials should advance setup, got %q", configured.SetupState)
	}
	if _, err := fx.service.CompleteSetup(context.Background(), fx.actor, fx.connection.ID); err == nil {
		t.Fatal("setup must not complete before provider validation")
	}
	if err := fx.service.MarkWebhookVerified(context.Background(), configured.Configuration); err != nil {
		t.Fatal(err)
	}
	ready, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || ready.SetupState != SetupTestMessageReady || !ready.Checklist.TestMessageReady {
		t.Fatalf("webhook-ready setup did not become test-ready: %+v err=%v", ready.Checklist, err)
	}
	client := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusOK, `{"messages":[{"id":"wamid-phase-l"}]}`, nil)}}
	adapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", client, false)
	adapter.now = fx.service.now
	template := createApprovedTemplate(t, fx, "setup_validation", []string{"name"})
	result, err := fx.service.SendTestMessage(context.Background(), fx.actor, fx.connection.ID, TestMessageInput{Recipient: "+234 800 000 0000", TemplateID: &template.ID, TemplateVariables: map[string]string{"name": "Customer"}}, adapter)
	if err != nil || result.Status != SetupTestMessageSent || result.RecipientDisplay != "****0000" {
		t.Fatalf("test send failed: %+v err=%v", result, err)
	}
	if client.tokenLeakedOutsideAuthorization() {
		t.Fatal("test send leaked the access token")
	}
	view, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || view.SetupState != SetupTestMessageSent || view.TestRecipientHash == "" {
		t.Fatalf("test state was not persisted safely: state=%q err=%v", view.SetupState, err)
	}
	body, _ := json.Marshal(view)
	if bytes.Contains(body, []byte("2348000000000")) {
		t.Fatal("configuration response exposed the full test recipient")
	}
	if matched, err := fx.service.MarkInboundTest(context.Background(), view.Configuration, "2348111111111"); err != nil || matched {
		t.Fatalf("unapproved inbound sender matched the setup test: matched=%v err=%v", matched, err)
	}
	handler := NewHandler(fx.service, fx.platform, fx.adapter, &recordingProcessor{})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	handler.RegisterPublic(app)
	inboundBody := []byte(textWebhook(view.PhoneNumberID, "wamid-phase-l-reply", "Received"))
	request := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/whatsapp", bytes.NewReader(inboundBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Hub-Signature-256", sign(inboundBody, "app-secret"))
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("signed inbound setup reply failed: status=%d err=%v", response.StatusCode, err)
	}
	view, err = fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || view.SetupState != SetupInboundTestReceived || !view.Checklist.ReadyToComplete {
		t.Fatalf("inbound validation was not recorded: state=%q checklist=%+v err=%v", view.SetupState, view.Checklist, err)
	}
	if err := fx.db.Model(&channelplatform.ProviderAccount{}).Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).Update("status", channelplatform.StatusConnecting).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&channelplatform.ChannelIdentity{}).Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).Update("status", channelplatform.StatusConnecting).Error; err != nil {
		t.Fatal(err)
	}
	view, err = fx.service.CompleteSetup(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || view.SetupState != SetupConnected {
		t.Fatalf("validated setup did not connect: state=%q err=%v", view.SetupState, err)
	}
	assertProviderRecordsConnected(t, fx)
	health, err := fx.service.EvaluateHealth(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || health.Status != channelplatform.HealthHealthy || health.SetupState != SetupHealthy {
		t.Fatalf("final health check did not finish onboarding: %+v err=%v", health, err)
	}
}

func TestSignedProviderActivityRepairsConnectingProviderRecords(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	if err := fx.db.Model(&channelplatform.ChannelConnection{}).Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, fx.connection.ID).Update("status", channelplatform.StatusConnected).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(fx.service, fx.platform, fx.adapter, &recordingProcessor{})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	handler.RegisterPublic(app)
	body := []byte(textWebhook(view.PhoneNumberID, "wamid-provider-ready", "Hello"))
	request := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/whatsapp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Hub-Signature-256", sign(body, "app-secret"))
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("signed provider activity failed: status=%d err=%v", response.StatusCode, err)
	}
	assertProviderRecordsConnected(t, fx)
}

func assertProviderRecordsConnected(t *testing.T, fx whatsappFixture) {
	t.Helper()
	var connection channelplatform.ChannelConnection
	var account channelplatform.ProviderAccount
	var identity channelplatform.ChannelIdentity
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&identity).Error; err != nil {
		t.Fatal(err)
	}
	connectionReady := connection.Status == channelplatform.StatusConnected || connection.Status == channelplatform.StatusHealthy
	if !connectionReady || account.Status != channelplatform.StatusConnected || identity.Status != channelplatform.StatusConnected {
		t.Fatalf("provider lifecycle was not synchronized: connection=%s account=%s identity=%s", connection.Status, account.Status, identity.Status)
	}
}

func TestWebhookCallbackAndVerificationAttemptsAreRecorded(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	fx.service.ConfigureWebhookPublicBaseURL("https://api.example.test")
	expectedURL := "https://api.example.test/v1/runtime/webhooks/whatsapp?connection_id=" + fx.connection.ID.String()
	if actual := fx.service.WebhookCallbackURL(fx.connection.ID); actual != expectedURL {
		t.Fatalf("unexpected callback URL %q", actual)
	}
	handler := NewHandler(fx.service, fx.platform, fx.adapter, &recordingProcessor{})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	handler.RegisterPublic(app)
	badURL := "/runtime/webhooks/whatsapp?connection_id=" + fx.connection.ID.String() + "&hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=x"
	bad, _ := app.Test(httptest.NewRequest(http.MethodGet, badURL, nil))
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token should return 403, got %d", bad.StatusCode)
	}
	failed, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || failed.WebhookStatus != WebhookFailed || failed.LastWebhookFailedAt == nil || failed.LastWebhookError != "verify_token_mismatch" {
		t.Fatalf("failed verification was not recorded safely: %+v err=%v", failed.Configuration, err)
	}
	goodURL := "/runtime/webhooks/whatsapp?connection_id=" + fx.connection.ID.String() + "&hub.mode=subscribe&hub.verify_token=verify-secret&hub.challenge=verified"
	good, _ := app.Test(httptest.NewRequest(http.MethodGet, goodURL, nil))
	challenge, _ := io.ReadAll(good.Body)
	if good.StatusCode != http.StatusOK || string(challenge) != "verified" {
		t.Fatalf("valid verification failed: status=%d body=%q", good.StatusCode, challenge)
	}
	updated, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || updated.WebhookStatus != WebhookVerified || updated.LastWebhookAttemptAt == nil || updated.LastWebhookError != "" {
		t.Fatalf("successful verification was not recorded: %+v err=%v", updated.Configuration, err)
	}
	verifiedEventFound := false
	failedEventFound := false
	for _, event := range updated.OperationalEvents {
		verifiedEventFound = verifiedEventFound || event.Title == "Webhook verified"
		failedEventFound = failedEventFound || event.Title == "Webhook verification failed"
	}
	if !verifiedEventFound || !failedEventFound {
		t.Fatalf("verification events were not mapped for operators: %+v", updated.OperationalEvents)
	}
	_ = view
}

func TestCredentialRotationIsTenantScopedRedactedAndAudited(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	replacement := "new-access-token-value"
	result, err := fx.service.RotateCredential(context.Background(), fx.actor, fx.connection.ID, CredentialRotationInput{CredentialType: CredentialAccessToken, SecretValue: &replacement})
	if err != nil || !result.Rotated || !result.Configured {
		t.Fatalf("credential rotation failed: %+v err=%v", result, err)
	}
	resultBody, _ := json.Marshal(result)
	if bytes.Contains(resultBody, []byte(replacement)) {
		t.Fatal("rotation response exposed credential material")
	}
	resolved, err := fx.service.ResolveCredential(context.Background(), fx.actor.OrganizationID, fx.connection.ID, CredentialAccessToken)
	if err != nil || resolved != replacement {
		t.Fatal("rotated credential was not stored")
	}
	var auditLog organization.AuditLog
	if err := fx.db.Where("organization_id = ? AND action = ?", fx.actor.OrganizationID, "whatsapp_credential_rotated").Order("created_at DESC").First(&auditLog).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditLog.Metadata, replacement) {
		t.Fatal("rotation audit exposed credential material")
	}
	if _, err := fx.service.RotateCredential(context.Background(), fx.other, fx.connection.ID, CredentialRotationInput{CredentialType: CredentialAccessToken, SecretValue: &replacement}); err == nil {
		t.Fatal("cross-tenant rotation must fail")
	}
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	if _, err := fx.service.RotateCredential(context.Background(), viewer, fx.connection.ID, CredentialRotationInput{CredentialType: CredentialAccessToken, SecretValue: &replacement}); err == nil {
		t.Fatal("viewer credential rotation must fail")
	}
	verifyReplacement := "new-verify-token"
	if _, err := fx.service.RotateCredential(context.Background(), fx.actor, fx.connection.ID, CredentialRotationInput{CredentialType: CredentialVerifyToken, SecretValue: &verifyReplacement}); err != nil {
		t.Fatal(err)
	}
	view, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || view.WebhookStatus != WebhookPending || view.LastWebhookVerifiedAt != nil {
		t.Fatalf("verify-token rotation must require webhook revalidation: %+v err=%v", view.Configuration, err)
	}
}

func TestTestMessageFailureHealthAttentionAndSafeEventMapping(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configured := configureWhatsApp(t, fx)
	if err := fx.service.MarkWebhookVerified(context.Background(), configured.Configuration); err != nil {
		t.Fatal(err)
	}
	client := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusTooManyRequests, `{"error":{"message":"private provider detail","code":4}}`, map[string]string{"Retry-After": "60"})}}
	adapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", client, false)
	adapter.now = fx.service.now
	openServiceWindow(t, fx, "2348000000000")
	if _, err := fx.service.SendTestMessage(context.Background(), fx.actor, fx.connection.ID, TestMessageInput{Recipient: "+2348000000000"}, adapter); err == nil {
		t.Fatal("provider failure must fail the controlled test send")
	}
	view, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || view.SetupState != SetupRequiresAttention || view.LastTestMessageError == "" {
		t.Fatalf("failed test send did not require attention: %+v err=%v", view.Configuration, err)
	}
	for _, event := range view.OperationalEvents {
		encoded, _ := json.Marshal(event)
		if bytes.Contains(encoded, []byte("private provider detail")) {
			t.Fatal("safe operational event exposed a provider payload")
		}
	}

	incomplete := newWhatsAppFixture(t)
	health, err := incomplete.service.EvaluateHealth(context.Background(), incomplete.actor, incomplete.connection.ID)
	if err != nil || health.Status != channelplatform.HealthFailed || health.SetupState != SetupRequiresAttention {
		t.Fatalf("missing credentials must require attention: %+v err=%v", health, err)
	}
	var connection channelplatform.ChannelConnection
	if err := incomplete.db.Where("organization_id = ? AND id = ?", incomplete.actor.OrganizationID, incomplete.connection.ID).First(&connection).Error; err != nil || connection.Status != channelplatform.StatusRequiresAttention {
		t.Fatalf("connection did not enter requires_attention: status=%q err=%v", connection.Status, err)
	}
}

func TestSafeOperationalEventCategories(t *testing.T) {
	tests := []struct {
		event    channelplatform.ProviderEvent
		title    string
		guidance string
	}{
		{channelplatform.ProviderEvent{EventType: "outbound_send", NormalizedStatus: "sent"}, "Message accepted by Meta", ""},
		{channelplatform.ProviderEvent{EventType: "outbound_send", NormalizedStatus: "failed", ProcessingError: "WhatsApp provider rate limit reached"}, "Meta rate limited messaging", "rate-limit"},
		{channelplatform.ProviderEvent{EventType: "outbound_send", NormalizedStatus: "failed", PayloadMetadata: `{"status_code":401,"error_code":190}`}, "Credential rejected", "access token"},
		{channelplatform.ProviderEvent{EventType: "outbound_send", NormalizedStatus: "failed", PayloadMetadata: `{"status_code":400,"error_code":131030}`}, "Recipient rejected", "allow-list"},
		{channelplatform.ProviderEvent{EventType: "outbound_send", NormalizedStatus: "failed", PayloadMetadata: `{"status_code":503}`}, "Meta delivery unavailable", "availability"},
		{channelplatform.ProviderEvent{EventType: "delivery_status", NormalizedStatus: "delivered"}, "Message delivered", ""},
		{channelplatform.ProviderEvent{EventType: "delivery_status", NormalizedStatus: "read"}, "Message read", ""},
		{channelplatform.ProviderEvent{EventType: "webhook_rejected", NormalizedStatus: "rejected"}, "Signed webhook rejected", "app secret"},
	}
	for _, test := range tests {
		view := safeOperationalEvent(test.event)
		if view.Title != test.title || (test.guidance != "" && !strings.Contains(view.Guidance, test.guidance)) {
			t.Fatalf("unexpected safe event mapping: %+v", view)
		}
	}
}

func TestSetupAndTestOperationsRequireTenantAndManagePermission(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configured := configureWhatsApp(t, fx)
	if err := fx.service.MarkWebhookVerified(context.Background(), configured.Configuration); err != nil {
		t.Fatal(err)
	}
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	sender := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusOK, `{"messages":[{"id":"never-sent"}]}`, nil)}}
	adapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", sender, false)
	if _, err := fx.service.SendTestMessage(context.Background(), viewer, fx.connection.ID, TestMessageInput{Recipient: "+2348000000000"}, adapter); err == nil {
		t.Fatal("viewer must not send a setup test message")
	}
	if _, err := fx.service.SendTestMessage(context.Background(), fx.other, fx.connection.ID, TestMessageInput{Recipient: "+2348000000000"}, adapter); err == nil {
		t.Fatal("another tenant must not send a setup test message")
	}
	if _, err := fx.service.CompleteSetup(context.Background(), viewer, fx.connection.ID); err == nil {
		t.Fatal("viewer must not complete setup")
	}
	if len(sender.requests) != 0 {
		t.Fatal("unauthorized setup operations reached the provider client")
	}
}
