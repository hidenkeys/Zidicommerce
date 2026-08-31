package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type fakeEmbeddedSignupClient struct {
	grant          MetaAccessGrant
	assets         MetaBusinessAssets
	exchangeErr    error
	validationErr  error
	subscribeErr   error
	exchangeCalls  int
	validateCalls  int
	subscribeCalls int
	lastCode       string
	lastToken      string
	lastWABA       string
	lastPhone      string
}

func (f *fakeEmbeddedSignupClient) ExchangeCode(_ context.Context, code string) (MetaAccessGrant, error) {
	f.exchangeCalls++
	f.lastCode = code
	return f.grant, f.exchangeErr
}

func (f *fakeEmbeddedSignupClient) ValidateAssets(_ context.Context, token, wabaID, phoneID string) (MetaBusinessAssets, error) {
	f.validateCalls++
	f.lastToken = token
	f.lastWABA = wabaID
	f.lastPhone = phoneID
	return f.assets, f.validationErr
}

func (f *fakeEmbeddedSignupClient) SubscribeApp(_ context.Context, token, wabaID string) error {
	f.subscribeCalls++
	f.lastToken = token
	f.lastWABA = wabaID
	return f.subscribeErr
}

func configureEmbeddedSignup(t *testing.T, fx whatsappFixture) *fakeEmbeddedSignupClient {
	t.Helper()
	t.Setenv("META_APP_SECRET", "meta-app-secret")
	t.Setenv("META_WEBHOOK_VERIFY_TOKEN", "meta-webhook-token")
	expiresAt := fx.service.now().Add(24 * time.Hour)
	client := &fakeEmbeddedSignupClient{
		grant:  MetaAccessGrant{AccessToken: "embedded-access-secret", ExpiresAt: &expiresAt},
		assets: MetaBusinessAssets{BusinessName: "Bing Chun", DisplayPhoneNumber: "+234 800 000 0000"},
	}
	fx.service.ConfigureEmbeddedSignup(EmbeddedSignupConfig{
		AppID: "123456789", AppSecret: "meta-app-secret", ConfigurationID: "987654321",
		GraphAPIVersion: "v24.0", WebhookVerifyToken: "meta-webhook-token",
	}, client)
	return client
}

func completeEmbeddedSignup(t *testing.T, fx whatsappFixture, attempt EmbeddedSignupInitiation, wabaID, phoneID string) EmbeddedSignupCompletion {
	t.Helper()
	result, err := fx.service.CompleteEmbeddedSignup(context.Background(), fx.actor, fx.connection.ID, EmbeddedSignupCompletionInput{
		AttemptToken: attempt.AttemptToken, AuthorizationCode: "short-lived-authorization-code",
		WhatsAppBusinessAccountID: wabaID, PhoneNumberID: phoneID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEmbeddedSignupCompletionIsTenantScopedEncryptedAndIdempotent(t *testing.T) {
	fx := newWhatsAppFixture(t)
	client := configureEmbeddedSignup(t, fx)
	attempt, err := fx.service.InitiateEmbeddedSignup(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.AppID != "123456789" || attempt.ConfigurationID != "987654321" || attempt.GraphAPIVersion != "v24.0" || len(attempt.AttemptToken) < 32 {
		t.Fatalf("unexpected safe signup initiation: %+v", attempt)
	}

	result := completeEmbeddedSignup(t, fx, attempt, "1000000001", "2000000001")
	if result.AuthorizationStatus != AuthorizationAuthorized || result.ConnectionStatus != channelplatform.StatusConnected || result.DisplayPhoneNumber != "+234 800 000 0000" {
		t.Fatalf("unexpected completion result: %+v", result)
	}
	if client.exchangeCalls != 1 || client.validateCalls != 1 || client.subscribeCalls != 1 || client.lastCode != "short-lived-authorization-code" || client.lastToken != "embedded-access-secret" {
		t.Fatalf("unexpected Meta calls: %+v", client)
	}

	second := completeEmbeddedSignup(t, fx, attempt, "1000000001", "2000000001")
	if second.AuthorizationStatus != AuthorizationAuthorized || client.exchangeCalls != 1 || client.subscribeCalls != 1 {
		t.Fatal("repeated completion must return the existing result without reusing the Meta code")
	}
	var configurationCount, attemptCount int64
	if err := fx.db.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).Count(&configurationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Model(&MetaSignupAttempt{}).Where("organization_id = ? AND channel_connection_id = ? AND status = ?", fx.actor.OrganizationID, fx.connection.ID, SignupAttemptCompleted).Count(&attemptCount).Error; err != nil {
		t.Fatal(err)
	}
	if configurationCount != 1 || attemptCount != 1 {
		t.Fatalf("expected one configuration and one completed attempt, got %d and %d", configurationCount, attemptCount)
	}

	access, err := fx.service.ResolveCredential(context.Background(), fx.actor.OrganizationID, fx.connection.ID, CredentialAccessToken)
	if err != nil || access != "embedded-access-secret" {
		t.Fatal("encrypted access token was not resolvable by the adapter")
	}
	appSecret, err := fx.service.ResolveCredential(context.Background(), fx.actor.OrganizationID, fx.connection.ID, CredentialAppSecret)
	if err != nil || appSecret != "meta-app-secret" {
		t.Fatal("Meta app secret environment reference was not resolvable")
	}
	view, err := fx.service.GetConfiguration(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	health, err := fx.service.EvaluateHealth(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || health.Status != channelplatform.HealthDegraded || !containsString(health.Issues, "signed_webhook_not_observed") {
		t.Fatalf("Embedded Signup must not claim healthy before a signed webhook is observed: %+v err=%v", health, err)
	}
	if err := fx.service.MarkSignatureVerified(context.Background(), view.Configuration); err != nil {
		t.Fatal(err)
	}
	health, err = fx.service.EvaluateHealth(context.Background(), fx.actor, fx.connection.ID)
	if err != nil || health.Status != channelplatform.HealthHealthy {
		t.Fatalf("signed webhook evidence should clear the Embedded Signup health warning: %+v err=%v", health, err)
	}
	body, _ := json.Marshal(struct {
		Completion EmbeddedSignupCompletion `json:"completion"`
		Config     ConfigurationView        `json:"configuration"`
	}{result, view})
	for _, forbidden := range []string{"embedded-access-secret", "meta-app-secret", "meta-webhook-token", "short-lived-authorization-code", "dbenc://", "env://"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("merchant API response exposed secret material %q", forbidden)
		}
	}
	var auditCount int64
	if err := fx.db.Table("audit_logs").Where("organization_id = ? AND action = ?", fx.actor.OrganizationID, "whatsapp_embedded_signup_completed").Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("expected one safe completion audit, count=%d err=%v", auditCount, err)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestEmbeddedSignupRejectsCrossTenantAttemptsAndDuplicateAssets(t *testing.T) {
	fx := newWhatsAppFixture(t)
	client := configureEmbeddedSignup(t, fx)
	attempt, err := fx.service.InitiateEmbeddedSignup(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CompleteEmbeddedSignup(context.Background(), fx.other, fx.connection.ID, EmbeddedSignupCompletionInput{
		AttemptToken: attempt.AttemptToken, AuthorizationCode: "code", WhatsAppBusinessAccountID: "1000000001", PhoneNumberID: "2000000001",
	}); err == nil {
		t.Fatal("another organization must not complete this connection's signup attempt")
	}
	if client.exchangeCalls != 0 {
		t.Fatal("cross-tenant attempt must be rejected before calling Meta")
	}
	completeEmbeddedSignup(t, fx, attempt, "1000000001", "2000000001")

	otherConnection, err := fx.platform.CreateConnection(context.Background(), fx.other, channelplatform.CreateConnectionInput{Provider: Provider, DisplayName: "Other WhatsApp", OwnershipModel: channelplatform.OwnershipMerchantManaged, Environment: channelplatform.EnvironmentSandbox})
	if err != nil {
		t.Fatal(err)
	}
	otherAttempt, err := fx.service.InitiateEmbeddedSignup(context.Background(), fx.other, otherConnection.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fx.service.CompleteEmbeddedSignup(context.Background(), fx.other, otherConnection.ID, EmbeddedSignupCompletionInput{
		AttemptToken: otherAttempt.AttemptToken, AuthorizationCode: "other-code",
		WhatsAppBusinessAccountID: "1000000001", PhoneNumberID: "2000000001",
	})
	if err == nil {
		t.Fatal("the same authorized WABA or phone must not be associated with another tenant")
	}
	if client.exchangeCalls != 1 {
		t.Fatal("duplicate assets must be rejected before another authorization exchange")
	}
}

func TestEmbeddedSignupPermissionAndProviderFailures(t *testing.T) {
	fx := newWhatsAppFixture(t)
	client := configureEmbeddedSignup(t, fx)
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	if _, err := fx.service.InitiateEmbeddedSignup(context.Background(), viewer, fx.connection.ID); err == nil {
		t.Fatal("viewer must not initiate Meta authorization")
	}
	attempt, err := fx.service.InitiateEmbeddedSignup(context.Background(), fx.actor, fx.connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	client.validationErr = errors.New("provider detail with embedded-access-secret")
	_, err = fx.service.CompleteEmbeddedSignup(context.Background(), fx.actor, fx.connection.ID, EmbeddedSignupCompletionInput{
		AttemptToken: attempt.AttemptToken, AuthorizationCode: "code",
		WhatsAppBusinessAccountID: "1000000001", PhoneNumberID: "2000000001",
	})
	if err == nil || strings.Contains(err.Error(), "embedded-access-secret") {
		t.Fatal("provider failures must be sanitized and must not expose token material")
	}
	var configuration Configuration
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, fx.connection.ID).First(&configuration).Error; err != nil {
		t.Fatal(err)
	}
	if configuration.AuthorizationStatus != AuthorizationFailed || configuration.LastAuthorizationError != "asset_validation_failed" {
		t.Fatalf("expected safe authorization failure state, got %+v", configuration)
	}
}

func TestAssistedSetupIsSeparateFromAuthorizationAndOperatorControlled(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureEmbeddedSignup(t, fx)
	view, err := fx.service.UpdateAssistedSetup(context.Background(), fx.actor, fx.connection.ID, AssistedSetupInput{Status: AssistedRequested})
	if err != nil {
		t.Fatal(err)
	}
	if view.AssistedSetupStatus != AssistedRequested || view.Connection.OwnershipModel != channelplatform.OwnershipZidiManaged || view.AuthorizationStatus == AuthorizationAuthorized {
		t.Fatalf("assisted request must change management mode without pretending Meta is authorized: %+v", view)
	}
	if _, err := fx.service.UpdateAssistedSetup(context.Background(), fx.actor, fx.connection.ID, AssistedSetupInput{Status: AssistedInProgress}); err == nil {
		t.Fatal("merchant admin must not advance operator-owned assisted setup")
	}
	operator := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.PlatformAdmin}
	view, err = fx.service.UpdateAssistedSetup(context.Background(), operator, fx.connection.ID, AssistedSetupInput{Status: AssistedInProgress, Note: "Preparing the Meta requirements."})
	if err != nil {
		t.Fatal(err)
	}
	view, err = fx.service.UpdateAssistedSetup(context.Background(), operator, fx.connection.ID, AssistedSetupInput{Status: AssistedAwaitingMerchantAction, Note: "Merchant must complete Meta authorization."})
	if err != nil {
		t.Fatal(err)
	}
	if view.AssistedSetupStatus != AssistedAwaitingMerchantAction || view.AssistedSetupNote == "" {
		t.Fatalf("expected explicit merchant action state, got %+v", view)
	}
	if _, err := fx.service.UpdateAssistedSetup(context.Background(), operator, fx.connection.ID, AssistedSetupInput{Status: AssistedConnected}); err == nil {
		t.Fatal("assisted setup must not be marked connected before Meta authorization")
	}
}

func TestSharedWebhookVerificationUsesServerConfiguration(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureEmbeddedSignup(t, fx)
	if !fx.service.VerifyPlatformWebhookToken("meta-webhook-token") || fx.service.VerifyPlatformWebhookToken("wrong") {
		t.Fatal("shared webhook verification token must match exactly")
	}
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	NewHandler(fx.service, fx.platform, fx.adapter, &recordingProcessor{}).RegisterPublic(app)
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/runtime/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=meta-webhook-token&hub.challenge=shared-challenge", nil))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("shared webhook verification failed: status=%d err=%v", response.StatusCode, err)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != "shared-challenge" {
		t.Fatalf("unexpected shared webhook challenge %q", body)
	}
	wrong, _ := app.Test(httptest.NewRequest(http.MethodGet, "/runtime/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=x", nil))
	if wrong.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong shared webhook token must fail closed, got %d", wrong.StatusCode)
	}
}

func TestMetaGraphClientExchangesValidatesAndSubscribesWithoutURLTokenLeak(t *testing.T) {
	httpClient := &fakeHTTPClient{responses: []*http.Response{
		jsonResponse(http.StatusOK, `{"access_token":"graph-access-token","expires_in":3600}`, nil),
		jsonResponse(http.StatusOK, `{"id":"1000000001","name":"Bing Chun"}`, nil),
		jsonResponse(http.StatusOK, `{"data":[{"id":"2000000001","display_phone_number":"+234 800 000 0000","verified_name":"Bing Chun"}]}`, nil),
		jsonResponse(http.StatusOK, `{"success":true}`, nil),
	}}
	client := NewMetaGraphClient("https://graph.example.test", "123456789", "meta-secret", "v24.0", httpClient)
	grant, err := client.ExchangeCode(context.Background(), "short-code")
	if err != nil || grant.AccessToken != "graph-access-token" || grant.ExpiresAt == nil {
		t.Fatalf("unexpected token exchange: grant=%+v err=%v", grant, err)
	}
	assets, err := client.ValidateAssets(context.Background(), grant.AccessToken, "1000000001", "2000000001")
	if err != nil || assets.BusinessName != "Bing Chun" || assets.DisplayPhoneNumber == "" {
		t.Fatalf("unexpected asset validation: assets=%+v err=%v", assets, err)
	}
	if err := client.SubscribeApp(context.Background(), grant.AccessToken, "1000000001"); err != nil {
		t.Fatal(err)
	}
	if len(httpClient.requests) != 4 {
		t.Fatalf("expected four Meta API calls, got %d", len(httpClient.requests))
	}
	for index, request := range httpClient.requests {
		if strings.Contains(request.URL.String(), "graph-access-token") || strings.Contains(request.URL.String(), "meta-secret") || strings.Contains(request.URL.String(), "short-code") {
			t.Fatalf("request %d leaked credential material in its URL", index)
		}
	}
	if got := httpClient.requests[3].Header.Get("Authorization"); got != "Bearer graph-access-token" {
		t.Fatal("WABA subscription must use the exchanged token in the authorization header")
	}
}

func TestMetaGraphClientSanitizesProviderErrorBody(t *testing.T) {
	httpClient := &fakeHTTPClient{responses: []*http.Response{
		jsonResponse(http.StatusBadRequest, `{"error":{"message":"token graph-access-token was rejected","type":"OAuthException","code":190}}`, nil),
	}}
	client := NewMetaGraphClient("https://graph.example.test", "123456789", "meta-secret", "v24.0", httpClient)
	_, err := client.ExchangeCode(context.Background(), "short-code")
	if err == nil || strings.Contains(err.Error(), "graph-access-token") || !strings.Contains(err.Error(), "code 190") {
		t.Fatalf("expected sanitized provider error, got %v", err)
	}
}
