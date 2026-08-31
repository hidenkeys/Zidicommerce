package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type whatsappFixture struct {
	db         *gorm.DB
	platform   *channelplatform.Service
	service    *Service
	adapter    *Adapter
	actor      auth.CurrentUser
	other      auth.CurrentUser
	connection channelplatform.ConnectionView
}

func newWhatsAppFixture(t *testing.T) whatsappFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&organization.Organization{}, &organization.AuditLog{}, &channelplatform.ChannelConnection{}, &channelplatform.ProviderAccount{}, &channelplatform.ChannelIdentity{}, &channelplatform.CredentialReference{}, &channelplatform.HealthCheck{}, &channelplatform.ProviderEvent{}, &channelplatform.MetricDaily{}, &channelplatform.ProviderSecret{}, &Configuration{}, &MetaSignupAttempt{}, &ContactState{}, &MessageTemplate{}, &PolicyDecisionRecord{}, &OperationalMetricDaily{}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE channels ADD COLUMN phone_number_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE channels ADD COLUMN display_number TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE channels ADD COLUMN config TEXT NOT NULL DEFAULT '{}'",
		"ALTER TABLE channels ADD COLUMN secret_config TEXT NOT NULL DEFAULT '{}'",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	other := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	for _, org := range []organization.Organization{{ID: actor.OrganizationID, Name: "Merchant", Slug: "merchant", Status: "active", Currency: "NGN", Timezone: "Africa/Lagos", Metadata: "{}"}, {ID: other.OrganizationID, Name: "Other", Slug: "other", Status: "active", Currency: "NGN", Timezone: "Africa/Lagos", Metadata: "{}"}} {
		if err := db.Create(&org).Error; err != nil {
			t.Fatal(err)
		}
	}
	platform := channelplatform.NewService(db)
	connection, err := platform.CreateConnection(context.Background(), actor, channelplatform.CreateConnectionInput{Provider: Provider, DisplayName: "WhatsApp", OwnershipModel: channelplatform.OwnershipMerchantManaged, Environment: channelplatform.EnvironmentSandbox})
	if err != nil {
		t.Fatal(err)
	}
	store, err := channelplatform.NewEncryptedSecretStore(db, []byte("12345678901234567890123456789012"), "test")
	if err != nil {
		t.Fatal(err)
	}
	resolver := channelplatform.NewDatabaseSecretResolver(db, store)
	service := NewService(db, platform, resolver, store)
	service.now = func() time.Time { return time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC) }
	adapter := NewAdapter(service, platform, "https://graph.example.test", &fakeHTTPClient{}, false)
	adapter.now = service.now
	return whatsappFixture{db: db, platform: platform, service: service, adapter: adapter, actor: actor, other: other, connection: connection}
}

func configureWhatsApp(t *testing.T, fx whatsappFixture) ConfigurationView {
	t.Helper()
	phoneID := "1234567890"
	wabaID := "waba-1"
	display := "+234 800 000 0000"
	access := "access-secret"
	appSecret := "app-secret"
	verify := "verify-secret"
	view, err := fx.service.UpdateConfiguration(context.Background(), fx.actor, fx.connection.ID, ConfigurationInput{PhoneNumberID: &phoneID, WhatsAppBusinessAccountID: &wabaID, DisplayPhoneNumber: &display, AccessTokenValue: &access, AppSecretValue: &appSecret, VerifyTokenValue: &verify})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func openServiceWindow(t *testing.T, fx whatsappFixture, recipient string) ContactState {
	t.Helper()
	view, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := fx.service.RecordInboundContact(context.Background(), view.Configuration, channelplatform.InboundEvent{Provider: Provider, ChannelConnectionID: fx.connection.ID, ProviderEventID: "window-" + uuid.NewString(), ExternalCustomerID: recipient, ExternalMessageID: uuid.NewString(), Body: "Hello", Timestamp: fx.service.now()})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func createApprovedTemplate(t *testing.T, fx whatsappFixture, name string, variables []string) MessageTemplateView {
	t.Helper()
	view, err := fx.service.CreateTemplate(context.Background(), fx.actor, fx.connection.ID, MessageTemplateInput{Name: name, Language: "en", Category: "utility", Status: TemplateApproved, Body: "Hello {{1}}", VariableSchema: variables, SampleValues: map[string]string{"name": "Customer"}})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestLegacyCredentialMigrationEncryptsAndRedacts(t *testing.T) {
	fx := newWhatsAppFixture(t)
	if err := fx.db.Table("channels").Where("id = ?", fx.connection.ID).Updates(map[string]any{"phone_number_id": "1234567890", "display_number": "+234", "config": `{"verify_token":"legacy-verify","graph_version":"v20.0"}`, "secret_config": `{"access_token":"legacy-access","app_secret":"legacy-app"}`}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.MigrateLegacyCredentials(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.CredentialsMigrated != 3 || !result.Encrypted || !result.LegacyRetained {
		t.Fatalf("unexpected migration result: %+v", result)
	}
	view, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(view)
	for _, forbidden := range []string{"legacy-access", "legacy-app", "legacy-verify", "dbenc://", "secret_ref"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("configuration response exposed forbidden credential material: %s", forbidden)
		}
	}
	resolved, err := fx.service.ResolveCredential(context.Background(), fx.actor.OrganizationID, fx.connection.ID, CredentialAccessToken)
	if err != nil || resolved != "legacy-access" {
		t.Fatal("migrated encrypted credential could not be resolved")
	}
	var legacy struct{ SecretConfig string }
	if err := fx.db.Table("channels").Select("secret_config").Where("id = ?", fx.connection.ID).First(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(legacy.SecretConfig, "legacy-access") {
		t.Fatal("legacy credential was deleted without an explicit cleanup")
	}
	if _, err := fx.service.GetConfiguration(context.Background(), fx.other, fx.connection.ID); err == nil {
		t.Fatal("cross-tenant WhatsApp configuration must not be visible")
	}
}

func TestWhatsAppConfigurationRequiresManagePermission(t *testing.T) {
	fx := newWhatsAppFixture(t)
	phoneID := "1234567890"
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	if _, err := fx.service.UpdateConfiguration(context.Background(), viewer, fx.connection.ID, ConfigurationInput{PhoneNumberID: &phoneID}); err == nil {
		t.Fatal("viewer must not update WhatsApp configuration")
	}
}

func TestWebhookVerificationDoesNotConnectIncompleteConfiguration(t *testing.T) {
	fx := newWhatsAppFixture(t)
	verify := "verify-only"
	view, err := fx.service.UpdateConfiguration(context.Background(), fx.actor, fx.connection.ID, ConfigurationInput{VerifyTokenValue: &verify})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.service.MarkWebhookVerified(context.Background(), view.Configuration); err != nil {
		t.Fatal(err)
	}
	var connection channelplatform.ChannelConnection
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if connection.Status == channelplatform.StatusConnected {
		t.Fatal("verify-token success must not connect an incomplete WhatsApp configuration")
	}
	phoneID := "1234567890"
	accountID := "9876543210"
	accessToken := "access-after-verification"
	appSecret := "app-after-verification"
	if _, err := fx.service.UpdateConfiguration(context.Background(), fx.actor, fx.connection.ID, ConfigurationInput{
		PhoneNumberID: &phoneID, WhatsAppBusinessAccountID: &accountID,
		AccessTokenValue: &accessToken, AppSecretValue: &appSecret,
	}); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if connection.Status != channelplatform.StatusConnected {
		t.Fatalf("verified setup must connect after its remaining configuration is completed, got %q", connection.Status)
	}
}

func TestWebhookVerificationAndSignatureFailClosed(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	processor := &recordingProcessor{}
	handler := NewHandler(fx.service, fx.platform, fx.adapter, processor)
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	handler.RegisterPublic(app)

	request := httptest.NewRequest(http.MethodGet, "/runtime/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=verify-secret&hub.challenge=challenge-123", nil)
	response, err := app.Test(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("expected webhook verification success, status=%d err=%v", response.StatusCode, err)
	}
	challenge, _ := io.ReadAll(response.Body)
	if string(challenge) != "challenge-123" {
		t.Fatalf("unexpected challenge response %q", challenge)
	}
	updatedDisplay := "+234 811 111 1111"
	if _, err := fx.service.UpdateConfiguration(context.Background(), fx.actor, fx.connection.ID, ConfigurationInput{DisplayPhoneNumber: &updatedDisplay}); err != nil {
		t.Fatal(err)
	}
	var account channelplatform.ProviderAccount
	var identity channelplatform.ChannelIdentity
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&identity).Error; err != nil {
		t.Fatal(err)
	}
	if account.Status != channelplatform.StatusConnected || identity.Status != channelplatform.StatusConnected {
		t.Fatalf("editing verified configuration regressed provider identity state: account=%s identity=%s", account.Status, identity.Status)
	}
	bad, _ := app.Test(httptest.NewRequest(http.MethodGet, "/runtime/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=x", nil))
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong verify token must fail, got %d", bad.StatusCode)
	}

	body := []byte(textWebhook(view.PhoneNumberID, "wamid-1", "Hello"))
	envelope := channelplatform.ProviderEnvelope{ConnectionID: fx.connection.ID, Headers: map[string]string{"X-Hub-Signature-256": "sha256=bad"}, RawPayload: body}
	if err := fx.adapter.VerifySignature(context.Background(), envelope); err == nil {
		t.Fatal("invalid signature must fail")
	}
	bypass := NewAdapter(fx.service, fx.platform, "", &fakeHTTPClient{}, true)
	if err := bypass.VerifySignature(context.Background(), envelope); err != nil {
		t.Fatalf("explicit development bypass should pass: %v", err)
	}
	validSignature := sign(body, "app-secret")
	envelope.Headers["X-Hub-Signature-256"] = validSignature
	if err := fx.adapter.VerifySignature(context.Background(), envelope); err != nil {
		t.Fatalf("valid signature failed: %v", err)
	}
	missingIdentityBody := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba","changes":[{"field":"messages","value":{"messages":[]}}]}]}`)
	missingIdentity := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/whatsapp", bytes.NewReader(missingIdentityBody))
	missingIdentity.Header.Set("Content-Type", "application/json")
	missingIdentity.Header.Set("X-Hub-Signature-256", sign(missingIdentityBody, "app-secret"))
	missingIdentityResponse, _ := app.Test(missingIdentity)
	if missingIdentityResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("webhook without phone identity must fail closed, got %d", missingIdentityResponse.StatusCode)
	}
	unknownPhoneBody := []byte(textWebhook("9999999999", "wamid-unknown", "Hello"))
	unknownPhone := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/whatsapp", bytes.NewReader(unknownPhoneBody))
	unknownPhone.Header.Set("Content-Type", "application/json")
	unknownPhone.Header.Set("X-Hub-Signature-256", sign(unknownPhoneBody, "app-secret"))
	unknownPhoneResponse, _ := app.Test(unknownPhone)
	if unknownPhoneResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown phone identity must fail closed, got %d", unknownPhoneResponse.StatusCode)
	}
}

func TestWebhookNormalizesAndProcessesInboundIdempotently(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	if err := fx.service.MarkWebhookVerified(context.Background(), view.Configuration); err != nil {
		t.Fatal(err)
	}
	processor := &recordingProcessor{}
	handler := NewHandler(fx.service, fx.platform, fx.adapter, processor)
	app := fiber.New()
	handler.RegisterPublic(app)
	body := []byte(textWebhook(view.PhoneNumberID, "wamid-text", "I need two teas"))
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/whatsapp", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Hub-Signature-256", sign(body, "app-secret"))
		response, err := app.Test(request)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("webhook attempt %d failed: status=%d err=%v", attempt, response.StatusCode, err)
		}
	}
	if processor.Count() != 1 {
		t.Fatalf("duplicate webhook processed %d times", processor.Count())
	}
	event := processor.Last()
	if event.Body != "I need two teas" || event.Provider != Provider || event.ChannelConnectionID != fx.connection.ID || event.ExternalCustomerID != "2348000000000" {
		t.Fatalf("unexpected normalized event: %+v", event)
	}
	summary, err := fx.platform.GetMetricsSummary(context.Background(), fx.actor, fx.connection.ID, time.Time{}, time.Time{})
	if err != nil || summary.InboundCount != 1 {
		t.Fatalf("inbound metric was not idempotent: %+v err=%v", summary, err)
	}
}

func TestFailedInboundProcessingCanBeRetried(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	if err := fx.service.MarkWebhookVerified(context.Background(), view.Configuration); err != nil {
		t.Fatal(err)
	}
	processor := &failOnceProcessor{}
	handler := NewHandler(fx.service, fx.platform, fx.adapter, processor)
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	handler.RegisterPublic(app)
	body := []byte(textWebhook(view.PhoneNumberID, "wamid-retry", "Retry me"))
	statuses := []int{http.StatusInternalServerError, http.StatusOK}
	for attempt, expectedStatus := range statuses {
		request := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/whatsapp", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Hub-Signature-256", sign(body, "app-secret"))
		response, err := app.Test(request)
		if err != nil || response.StatusCode != expectedStatus {
			t.Fatalf("webhook retry %d status=%d expected=%d err=%v", attempt, response.StatusCode, expectedStatus, err)
		}
	}
	if processor.Calls() != 2 {
		t.Fatalf("failed provider event was not reclaimed exactly once, calls=%d", processor.Calls())
	}
	var event channelplatform.ProviderEvent
	if err := fx.db.Where("organization_id = ? AND provider_event_id = ?", fx.actor.OrganizationID, "wamid-retry").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.NormalizedStatus != "processed" || event.ProcessingError != "" {
		t.Fatalf("retried provider event did not recover: %+v", event)
	}
}

func TestInboundInteractiveMediaAndUnsupportedNormalization(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	if err := fx.service.MarkWebhookVerified(context.Background(), view.Configuration); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"` + view.PhoneNumberID + `"},"messages":[{"id":"m1","from":"234","timestamp":"1787930000","type":"interactive","interactive":{"type":"button_reply","button_reply":{"id":"buy","title":"Buy now"}}},{"id":"m2","from":"234","timestamp":"1787930001","type":"image","image":{"id":"media-1","mime_type":"image/jpeg","caption":"Receipt"}},{"id":"m3","from":"234","type":"contacts"}]}}]}]}`)
	envelope := channelplatform.ProviderEnvelope{ConnectionID: fx.connection.ID, RawPayload: body, ReceivedAt: time.Now()}
	events, err := fx.adapter.NormalizeInbound(context.Background(), envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Body != "buy" || len(events[1].Media) != 1 || events[1].Media[0].Reference != "meta-media://media-1" {
		t.Fatalf("unexpected normalized messages: %+v", events)
	}
	unsupported, err := fx.adapter.UnsupportedInbound(context.Background(), envelope)
	if err != nil || len(unsupported) != 1 || unsupported[0].MessageType != "contacts" {
		t.Fatalf("unsupported message was not safely identified: %+v err=%v", unsupported, err)
	}
}

func TestOutboundRequestSuccessRateLimitAndMetrics(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	openServiceWindow(t, fx, "2348000000000")
	successClient := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusOK, `{"messages":[{"id":"wamid-out"}]}`, nil)}}
	adapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", successClient, false)
	adapter.now = fx.service.now
	command := channelplatform.OutboundCommand{Provider: Provider, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "2348000000000", MessageType: "text", Body: "Hello", IdempotencyKey: "send-1"}
	result, err := adapter.Send(context.Background(), command)
	if err != nil || result.ProviderMessageID != "wamid-out" {
		t.Fatalf("outbound send failed: %+v err=%v", result, err)
	}
	if successClient.tokenLeakedOutsideAuthorization() {
		t.Fatal("access token leaked outside the Authorization header")
	}
	var providerEvent channelplatform.ProviderEvent
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ? AND event_type = ?", fx.actor.OrganizationID, fx.connection.ID, "outbound_send").First(&providerEvent).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(providerEvent.PayloadMetadata, "access-secret") || strings.Contains(providerEvent.ProcessingError, "access-secret") {
		t.Fatal("provider event exposed an access token")
	}
	rateClient := &fakeHTTPClient{responses: []*http.Response{jsonResponse(http.StatusTooManyRequests, `{"error":{"message":"Rate limited","type":"OAuthException","code":4}}`, map[string]string{"Retry-After": "60"})}}
	rateAdapter := NewAdapter(fx.service, fx.platform, "https://graph.example.test", rateClient, false)
	rateAdapter.now = fx.service.now
	command.IdempotencyKey = "send-2"
	if _, err := rateAdapter.Send(context.Background(), command); err == nil {
		t.Fatal("provider rate limit must be returned as a send failure")
	}
	updated, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || updated.RateLimitedUntil == nil {
		t.Fatalf("rate-limit health state was not persisted: %+v err=%v", updated, err)
	}
	summary, err := fx.platform.GetMetricsSummary(context.Background(), fx.actor, fx.connection.ID, time.Time{}, time.Time{})
	if err != nil || summary.OutboundCount != 1 || summary.FailedOutboundCount != 1 || summary.ProviderErrorCount != 1 {
		t.Fatalf("unexpected outbound metrics: %+v err=%v", summary, err)
	}
	advanced, err := fx.service.GetAdvancedMetrics(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || advanced.FreeformSends != 1 || advanced.RateLimits != 1 {
		t.Fatalf("unexpected WhatsApp operational metrics: %+v err=%v", advanced, err)
	}
}

func TestDeliveryStatusUpdatesExistingOutboundAndHealth(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	if err := fx.service.MarkWebhookVerified(context.Background(), view.Configuration); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Exec(`CREATE TABLE channel_outbound_messages (id TEXT PRIMARY KEY, organization_id TEXT, channel_id TEXT, provider_message_id TEXT, status TEXT, sent_at DATETIME, delivered_at DATETIME, read_at DATETIME, error_message TEXT, updated_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Exec(`INSERT INTO channel_outbound_messages (id, organization_id, channel_id, provider_message_id, status, error_message, updated_at) VALUES (?, ?, ?, ?, ?, '', ?)`, uuid.NewString(), fx.actor.OrganizationID.String(), fx.connection.ID.String(), "wamid-delivery", "sent", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	body := []byte(statusWebhook(view.PhoneNumberID, "wamid-delivery", "delivered"))
	updates, err := fx.adapter.NormalizeDelivery(context.Background(), channelplatform.ProviderEnvelope{ConnectionID: fx.connection.ID, RawPayload: body, ReceivedAt: time.Now()})
	if err != nil || len(updates) != 1 {
		t.Fatalf("delivery callback did not normalize: %+v err=%v", updates, err)
	}
	updated, err := fx.service.ApplyDeliveryUpdate(context.Background(), view.Configuration, updates[0])
	if err != nil || !updated {
		t.Fatalf("delivery row was not updated: updated=%v err=%v", updated, err)
	}
	var status string
	if err := fx.db.Raw(`SELECT status FROM channel_outbound_messages WHERE provider_message_id = ?`, "wamid-delivery").Scan(&status).Error; err != nil || status != "delivered" {
		t.Fatalf("unexpected outbound delivery status %q err=%v", status, err)
	}
	health, err := fx.service.EvaluateHealth(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || health.Status != channelplatform.HealthHealthy {
		t.Fatalf("configured WhatsApp health should be healthy: %+v err=%v", health, err)
	}
	first := channelplatform.DeliveryUpdate{ProviderMessageID: "wamid-delivery", Status: "delivered", OccurredAt: time.Now()}
	second := first
	second.OccurredAt = first.OccurredAt.Add(time.Minute)
	if statusEventID(first) != statusEventID(second) {
		t.Fatal("repeated delivery status must retain one idempotency identity")
	}
}

type recordingProcessor struct {
	mu     sync.Mutex
	events []channelplatform.InboundEvent
}

type failOnceProcessor struct {
	mu    sync.Mutex
	calls int
}

func (p *failOnceProcessor) ProcessChannelInbound(_ context.Context, _ uuid.UUID, _ channelplatform.InboundEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.calls == 1 {
		return errors.New("temporary runtime failure")
	}
	return nil
}

func (p *failOnceProcessor) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *recordingProcessor) ProcessChannelInbound(_ context.Context, _ uuid.UUID, event channelplatform.InboundEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

func (p *recordingProcessor) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *recordingProcessor) Last() channelplatform.InboundEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.events[len(p.events)-1]
}

type fakeHTTPClient struct {
	mu        sync.Mutex
	responses []*http.Response
	errors    []error
	requests  []*http.Request
}

func (c *fakeHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, request)
	index := len(c.requests) - 1
	if index < len(c.errors) && c.errors[index] != nil {
		return nil, c.errors[index]
	}
	if index < len(c.responses) {
		return c.responses[index], nil
	}
	return nil, errors.New("no fake response configured")
}

func (c *fakeHTTPClient) tokenLeakedOutsideAuthorization() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, request := range c.requests {
		if strings.Contains(request.URL.String(), "access-secret") {
			return true
		}
		for name, values := range request.Header {
			if strings.EqualFold(name, "Authorization") {
				continue
			}
			if strings.Contains(strings.Join(values, ","), "access-secret") {
				return true
			}
		}
	}
	return false
}

func jsonResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func textWebhook(phoneID, messageID, body string) string {
	return `{"object":"whatsapp_business_account","entry":[{"id":"waba","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"` + phoneID + `"},"messages":[{"id":"` + messageID + `","from":"2348000000000","timestamp":"1787930000","type":"text","text":{"body":"` + body + `"}}]}}]}]}`
}

func statusWebhook(phoneID, messageID, status string) string {
	return `{"object":"whatsapp_business_account","entry":[{"id":"waba","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"` + phoneID + `"},"statuses":[{"id":"` + messageID + `","status":"` + status + `","timestamp":"1787930000","recipient_id":"234"}]}}]}]}`
}

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
