package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

type messagingFixture struct {
	db         *gorm.DB
	platform   *channelplatform.Service
	service    *Service
	adapter    *MessagingAdapter
	actor      auth.CurrentUser
	connection channelplatform.ConnectionView
	now        time.Time
}

func newMessagingFixture(t *testing.T, provider string, client HTTPClient) messagingFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{}, &organization.AuditLog{},
		&channelplatform.ChannelConnection{}, &channelplatform.ChannelCapability{},
		&channelplatform.ProviderAccount{}, &channelplatform.ChannelIdentity{},
		&channelplatform.CredentialReference{}, &channelplatform.ProviderSecret{},
		&channelplatform.ProviderEvent{}, &channelplatform.HealthCheck{}, &channelplatform.MetricDaily{},
		&ConnectionState{}, &ContactState{},
	); err != nil {
		t.Fatal(err)
	}
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	if err := db.Create(&organization.Organization{ID: actor.OrganizationID, Name: "Merchant", Slug: "merchant-" + uuid.NewString(), Status: "active", Currency: "NGN", Timezone: "Africa/Lagos", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	platform := channelplatform.NewService(db)
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	connection, err := platform.CreateConnection(context.Background(), actor, channelplatform.CreateConnectionInput{Provider: provider, DisplayName: provider, OwnershipModel: channelplatform.OwnershipMerchantManaged, Environment: channelplatform.EnvironmentSandbox})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&channelplatform.ChannelConnection{}).Where("id = ?", connection.ID).Update("status", channelplatform.StatusConnected).Error; err != nil {
		t.Fatal(err)
	}
	identityType := "page"
	identityID := "page-1"
	graphBase := "https://graph.facebook.test"
	if provider == ProviderInstagram {
		identityType = "instagram_business_account"
		identityID = "ig-1"
		graphBase = "https://graph.instagram.test"
	}
	if err := db.Create(&channelplatform.ChannelIdentity{ID: uuid.New(), OrganizationID: actor.OrganizationID, ConnectionID: connection.ID, Provider: provider, IdentityType: identityType, DisplayName: "Merchant Social", ProviderIdentityID: identityID, ExternalHandle: "merchant", Status: channelplatform.StatusConnected, Metadata: "{}", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	store, err := channelplatform.NewEncryptedSecretStore(db, []byte("0123456789abcdef0123456789abcdef"), "test")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.Save(context.Background(), actor.OrganizationID, connection.ID, channelplatform.OAuthCredentialAccessToken, "provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := platform.UpsertCredentialReference(context.Background(), actor, connection.ID, channelplatform.CredentialReferenceInput{CredentialType: channelplatform.OAuthCredentialAccessToken, SecretRef: ref, Status: channelplatform.CredentialPresent}); err != nil {
		t.Fatal(err)
	}
	service := NewService(db, platform, channelplatform.NewDatabaseSecretResolver(db, store))
	service.now = func() time.Time { return now }
	adapter := NewMessagingAdapter(provider, service, platform, graphBase, "v24.0", "app-secret", client)
	adapter.now = service.now
	connection.Status = channelplatform.StatusConnected
	return messagingFixture{db: db, platform: platform, service: service, adapter: adapter, actor: actor, connection: connection, now: now}
}

func TestMetaAdapterVerifiesAndNormalizesSupportedEvents(t *testing.T) {
	fx := newMessagingFixture(t, ProviderInstagram, nil)
	body := []byte(metaWebhook("instagram", "ig-1"))
	envelope := channelplatform.ProviderEnvelope{Provider: ProviderInstagram, ConnectionID: fx.connection.ID, Headers: map[string]string{"X-Hub-Signature-256": signature(body, "app-secret")}, RawPayload: body, ReceivedAt: fx.now}
	if err := fx.adapter.VerifySignature(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Headers["X-Hub-Signature-256"] = "sha256=00"
	if err := fx.adapter.VerifySignature(context.Background(), envelope); err == nil {
		t.Fatal("invalid Meta signature must be rejected")
	}
	envelope.Headers["X-Hub-Signature-256"] = signature(body, "app-secret")

	events, err := fx.adapter.NormalizeInbound(context.Background(), envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected text, postback, and reaction events, got %+v", events)
	}
	if events[0].ExternalCustomerID != "customer-1" || events[0].ExternalConversationID != "customer-1" || events[0].Body != "Hello" || len(events[0].Media) != 1 {
		t.Fatalf("unexpected normalized message: %+v", events[0])
	}
	if events[1].MessageType != "postback" || events[1].Metadata["action_id"] != "BUY_NOW" {
		t.Fatalf("unexpected postback: %+v", events[1])
	}
	if events[2].MessageType != "reaction" || events[2].Metadata["reacted_to_message_id"] != "mid-1" {
		t.Fatalf("unexpected reaction: %+v", events[2])
	}
	deliveries, err := fx.adapter.NormalizeDelivery(context.Background(), envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 2 || deliveries[0].ProviderMessageID != "outbound-1" || deliveries[0].Status != "delivered" || deliveries[1].Status != "read" {
		t.Fatalf("unexpected delivery events: %+v", deliveries)
	}
	unsupported, err := fx.adapter.UnsupportedInbound(context.Background(), envelope)
	if err != nil || len(unsupported) != 1 || unsupported[0].ProviderEventID == "" {
		t.Fatalf("unexpected unsupported events: %+v err=%v", unsupported, err)
	}
}

func TestMetaOutboundRequiresCustomerWindowAndUsesSelectedIdentity(t *testing.T) {
	var receivedPath, receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedToken = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if json.Unmarshal(body, &payload) != nil || payload["messaging_type"] != "RESPONSE" {
			t.Errorf("unexpected outbound payload: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recipient_id":"customer-1","message_id":"provider-mid-1"}`))
	}))
	defer server.Close()
	fx := newMessagingFixture(t, ProviderFacebook, server.Client())
	fx.adapter.graphBaseURL = server.URL
	command := channelplatform.OutboundCommand{Provider: ProviderFacebook, ChannelConnectionID: fx.connection.ID, ExternalCustomerID: "customer-1", ExternalConversationID: "customer-1", MessageType: "text", Body: "Welcome back", IdempotencyKey: "send-1", Metadata: map[string]any{"organization_id": fx.actor.OrganizationID.String()}}
	if _, err := fx.adapter.Send(context.Background(), command); err == nil {
		t.Fatal("unsolicited Meta message must be blocked")
	}
	resolved, err := fx.service.ResolveForSend(context.Background(), fx.actor.OrganizationID, fx.connection.ID, ProviderFacebook)
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.service.RecordInboundContact(context.Background(), resolved, "customer-1", fx.now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	result, err := fx.adapter.Send(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderMessageID != "provider-mid-1" || receivedPath != "/v24.0/page-1/messages" || receivedToken != "Bearer provider-token" {
		t.Fatalf("outbound request used wrong identity or credential: result=%+v path=%s token=%s", result, receivedPath, receivedToken)
	}
	fx.service.now = func() time.Time { return fx.now.Add(25 * time.Hour) }
	if _, err := fx.adapter.Send(context.Background(), command); err == nil {
		t.Fatal("expired Meta reply window must be blocked")
	}
}

type recordingProcessor struct {
	events []channelplatform.InboundEvent
}

func (p *recordingProcessor) ProcessChannelInbound(_ context.Context, _ uuid.UUID, event channelplatform.InboundEvent) error {
	p.events = append(p.events, event)
	return nil
}

func TestMetaWebhookHandlerIsSignatureCheckedAndIdempotent(t *testing.T) {
	fx := newMessagingFixture(t, ProviderInstagram, nil)
	processor := &recordingProcessor{}
	handler := NewHandler(fx.service, fx.platform, processor, "verify-token", fx.adapter)
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	handler.RegisterPublic(app)

	verify := httptest.NewRequest(http.MethodGet, "/runtime/webhooks/meta?hub.mode=subscribe&hub.verify_token=verify-token&hub.challenge=challenge", nil)
	response, err := app.Test(verify)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("webhook verification failed: status=%d err=%v", response.StatusCode, err)
	}
	body := []byte(singleMessageWebhook("instagram", "ig-1", "mid-handler"))
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/meta", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Hub-Signature-256", signature(body, "app-secret"))
		response, err = app.Test(request)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("webhook attempt %d failed: status=%d err=%v", attempt, response.StatusCode, err)
		}
	}
	if len(processor.events) != 1 {
		t.Fatalf("duplicate webhook dispatched %d runtime events", len(processor.events))
	}
	var eventCount int64
	if err := fx.db.Model(&channelplatform.ProviderEvent{}).Where("organization_id = ? AND provider = ? AND provider_event_id = ?", fx.actor.OrganizationID, ProviderInstagram, "mid-handler").Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("expected one provider event, got %d", eventCount)
	}

	badRequest := httptest.NewRequest(http.MethodPost, "/runtime/webhooks/meta", bytes.NewReader(body))
	badRequest.Header.Set("Content-Type", "application/json")
	badRequest.Header.Set("X-Hub-Signature-256", "sha256=00")
	response, err = app.Test(badRequest)
	if err != nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("invalid signature was not rejected: status=%d err=%v", response.StatusCode, err)
	}
	var state ConnectionState
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.WebhookStatus != WebhookActive || state.SignatureRejectionCount != 1 || state.LastSignatureVerifiedAt == nil {
		t.Fatalf("invalid probe damaged valid webhook readiness: %+v", state)
	}
}

func signature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func singleMessageWebhook(object, identityID, messageID string) string {
	return `{"object":"` + object + `","entry":[{"id":"` + identityID + `","time":1789992000,"messaging":[{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992000000,"message":{"mid":"` + messageID + `","text":"Hello"}}]}]}`
}

func metaWebhook(object, identityID string) string {
	return `{"object":"` + object + `","entry":[{"id":"` + identityID + `","time":1789992000,"messaging":[` +
		`{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992000000,"message":{"mid":"mid-1","text":"Hello","attachments":[{"type":"image","payload":{"url":"https://cdn.example/image.jpg"}}]}},` +
		`{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992001000,"postback":{"mid":"postback-1","title":"Buy now","payload":"BUY_NOW"}},` +
		`{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992002000,"reaction":{"mid":"mid-1","action":"react","reaction":"love","emoji":"heart"}},` +
		`{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992003000,"delivery":{"mids":["outbound-1"],"watermark":1789992003000}},` +
		`{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992004000,"read":{"watermark":1789992004000}},` +
		`{"sender":{"id":"customer-1"},"recipient":{"id":"` + identityID + `"},"timestamp":1789992005000,"referral":{"ref":"unsupported"}}` +
		`]}]}`
}
