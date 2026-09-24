package channelplatform

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
)

type fakeOAuthClient struct {
	authorization OAuthAuthorizationRequest
	exchange      OAuthCodeExchangeRequest
	grant         OAuthTokenGrant
	refreshGrant  OAuthTokenGrant
	assets        []OAuthAsset
	connected     OAuthAsset
	revokedToken  string
	refreshToken  string
	exchangeErr   error
}

func (f *fakeOAuthClient) Provider() string { return "instagram" }

func (f *fakeOAuthClient) RequestedScopes() []string {
	return []string{"instagram_business_basic", "instagram_business_manage_messages"}
}

func (f *fakeOAuthClient) AuthorizationURL(input OAuthAuthorizationRequest) (string, error) {
	f.authorization = input
	values := url.Values{"state": []string{input.State}, "code_challenge": []string{input.CodeChallenge}}
	return "https://provider.example/authorize?" + values.Encode(), nil
}

func (f *fakeOAuthClient) ExchangeCode(_ context.Context, input OAuthCodeExchangeRequest) (OAuthTokenGrant, error) {
	f.exchange = input
	return f.grant, f.exchangeErr
}

func (f *fakeOAuthClient) DiscoverAssets(context.Context, string) ([]OAuthAsset, error) {
	return f.assets, nil
}

func (f *fakeOAuthClient) ConnectAsset(_ context.Context, _ string, asset OAuthAsset) error {
	f.connected = asset
	return nil
}

func (f *fakeOAuthClient) Refresh(_ context.Context, token string) (OAuthTokenGrant, error) {
	f.refreshToken = token
	return f.refreshGrant, nil
}

func (f *fakeOAuthClient) Revoke(_ context.Context, token string) error {
	f.revokedToken = token
	return nil
}

type oauthFixture struct {
	channelFixture
	oauth  *OAuthService
	client *fakeOAuthClient
}

func newOAuthFixture(t *testing.T) oauthFixture {
	t.Helper()
	base := newChannelFixture(t)
	if err := base.db.AutoMigrate(&ProviderSecret{}, &OAuthSession{}, &OAuthAssetRecord{}); err != nil {
		t.Fatal(err)
	}
	store, err := NewEncryptedSecretStore(base.db, []byte("0123456789abcdef0123456789abcdef"), "test")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeOAuthClient{
		grant: OAuthTokenGrant{
			AccessToken:    "provider-access-token",
			RefreshToken:   "provider-refresh-token",
			GrantedScopes:  []string{"instagram_business_basic"},
			ReviewApproved: false,
		},
		refreshGrant: OAuthTokenGrant{
			AccessToken:   "refreshed-access-token",
			GrantedScopes: []string{"instagram_business_basic"},
		},
		assets: []OAuthAsset{{
			ProviderAssetID: "ig-asset-1",
			AssetType:       "instagram_business_account",
			DisplayName:     "Merchant Instagram",
			ExternalHandle:  "@merchant",
			Eligible:        true,
			Metadata: map[string]any{
				"account_type": "BUSINESS",
				"access_token": "must-not-leak",
				"nested":       map[string]any{"secret": "must-not-leak", "category": "Retail"},
			},
		}},
	}
	oauthService := NewOAuthService(base.db, base.service, store, NewDatabaseSecretResolver(base.db, store))
	oauthService.now = base.service.now
	oauthService.RegisterClient(client)
	return oauthFixture{channelFixture: base, oauth: oauthService, client: client}
}

func startOAuthSession(t *testing.T, fx oauthFixture, actor auth.CurrentUser, connectionID uuid.UUID) (OAuthStartView, string) {
	t.Helper()
	started, err := fx.oauth.Start(context.Background(), actor, connectionID, OAuthStartInput{RedirectURI: "https://app.example/channels/oauth/callback"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("authorization URL did not contain OAuth state")
	}
	return started, state
}

func TestOAuthLifecycleSecuresStateTokensScopesAndAssets(t *testing.T) {
	fx := newOAuthFixture(t)
	connection := createConnection(t, fx.channelFixture, fx.actor, "instagram", OwnershipMerchantManaged)
	started, state := startOAuthSession(t, fx, fx.actor, connection.ID)

	var session OAuthSession
	if err := fx.db.First(&session, "id = ?", started.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if session.StateHash == state || session.StateHash != oauthHash(state) {
		t.Fatal("OAuth state must only be stored as a hash")
	}
	if session.PKCEVerifierRef == "" || strings.Contains(session.PKCEVerifierRef, fx.client.authorization.CodeChallenge) {
		t.Fatal("PKCE verifier must be stored through the encrypted secret store")
	}

	callback, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: state, Code: "authorization-code"})
	if err != nil {
		t.Fatal(err)
	}
	if fx.client.exchange.CodeVerifier == "" || pkceChallenge(fx.client.exchange.CodeVerifier) != fx.client.authorization.CodeChallenge {
		t.Fatal("callback did not recover the verifier matching the PKCE challenge")
	}
	if callback.Session.Status != OAuthSessionAwaitingAsset || len(callback.Session.DeniedScopes) != 1 || callback.Session.DeniedScopes[0] != "instagram_business_manage_messages" {
		t.Fatalf("unexpected callback scope result: %+v", callback.Session)
	}
	serialized, err := json.Marshal(callback)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"provider-access-token", "provider-refresh-token", "must-not-leak"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("OAuth response exposed secret %q", secret)
		}
	}
	if len(callback.Assets) != 1 || callback.Assets[0].Metadata["access_token"] != nil {
		t.Fatalf("unsafe provider metadata was not redacted: %+v", callback.Assets)
	}
	nested, _ := callback.Assets[0].Metadata["nested"].(map[string]any)
	if nested["secret"] != nil || nested["category"] != "Retail" {
		t.Fatalf("nested provider metadata was not safely filtered: %+v", nested)
	}

	var secrets []ProviderSecret
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, connection.ID).Find(&secrets).Error; err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 2 {
		t.Fatalf("expected encrypted access and refresh tokens, got %d records", len(secrets))
	}
	for _, secret := range secrets {
		if strings.Contains(secret.Ciphertext, "provider-") {
			t.Fatal("provider token was stored as plaintext")
		}
	}

	detail, err := fx.oauth.SelectAsset(context.Background(), fx.actor, connection.ID, callback.Assets[0].ID, OAuthAssetSelectionInput{SessionID: callback.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Connection.Status != StatusConnected || fx.client.connected.ProviderAssetID != "ig-asset-1" {
		t.Fatalf("provider asset was not connected: %+v", detail.Connection)
	}
	for _, capability := range detail.Connection.CapabilityStates {
		if capability.Capability == "outbound_text" && capability.Status != CapabilityAwaitingPermissionReview {
			t.Fatalf("standard Meta access must remain review gated, got %+v", capability)
		}
	}
}

func TestOAuthCallbackIsBoundToTenantUserExpiryAndOneTimeState(t *testing.T) {
	fx := newOAuthFixture(t)
	connection := createConnection(t, fx.channelFixture, fx.actor, "instagram", OwnershipMerchantManaged)
	_, state := startOAuthSession(t, fx, fx.actor, connection.ID)

	sameTenantOtherUser := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.MerchantAdmin}
	if _, err := fx.oauth.CompleteCallback(context.Background(), sameTenantOtherUser, connection.ID, OAuthCallbackInput{State: state, Code: "code"}); err == nil {
		t.Fatal("OAuth callback must be bound to the user that started it")
	}
	if _, err := fx.oauth.CompleteCallback(context.Background(), fx.other, connection.ID, OAuthCallbackInput{State: state, Code: "code"}); err == nil {
		t.Fatal("OAuth callback must not cross tenant boundaries")
	}
	if _, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: state, Code: "code"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: state, Code: "code"}); err == nil {
		t.Fatal("OAuth callback state must be single use")
	}

	second, secondState := startOAuthSession(t, fx, fx.actor, connection.ID)
	expired := fx.oauth.now().Add(-time.Minute)
	if err := fx.db.Model(&OAuthSession{}).Where("id = ?", second.SessionID).Update("expires_at", expired).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: secondState, Code: "code"}); err == nil {
		t.Fatal("expired OAuth state must be rejected")
	}
	var expiredSession OAuthSession
	if err := fx.db.First(&expiredSession, "id = ?", second.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if expiredSession.Status != OAuthSessionExpired {
		t.Fatalf("expected expired session status, got %s", expiredSession.Status)
	}
}

func TestOAuthCancelRefreshAndDisconnectLifecycle(t *testing.T) {
	fx := newOAuthFixture(t)
	connection := createConnection(t, fx.channelFixture, fx.actor, "instagram", OwnershipMerchantManaged)
	cancelledStart, _ := startOAuthSession(t, fx, fx.actor, connection.ID)
	cancelled, err := fx.oauth.Cancel(context.Background(), fx.actor, connection.ID, OAuthCancelInput{SessionID: cancelledStart.SessionID})
	if err != nil || cancelled.Status != OAuthSessionCancelled {
		t.Fatalf("cancel OAuth: status=%s err=%v", cancelled.Status, err)
	}

	_, state := startOAuthSession(t, fx, fx.actor, connection.ID)
	callback, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: state, Code: "code"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.oauth.SelectAsset(context.Background(), fx.actor, connection.ID, callback.Assets[0].ID, OAuthAssetSelectionInput{SessionID: callback.Session.ID}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := fx.oauth.Refresh(context.Background(), fx.actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != CredentialPresent || fx.client.refreshToken != "provider-refresh-token" {
		t.Fatalf("refresh did not use the stored refresh token: %+v", refreshed)
	}
	accessToken, err := fx.oauth.resolveOAuthCredential(context.Background(), fx.actor.OrganizationID, connection.ID, OAuthCredentialAccessToken)
	if err != nil || accessToken != "refreshed-access-token" {
		t.Fatalf("refreshed access token was not stored: token=%q err=%v", accessToken, err)
	}

	detail, err := fx.oauth.Disconnect(context.Background(), fx.actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Connection.Status != StatusDisconnected || fx.client.revokedToken != "refreshed-access-token" {
		t.Fatalf("disconnect did not revoke and transition the connection: %+v", detail.Connection)
	}
	var activeSecrets int64
	if err := fx.db.Model(&ProviderSecret{}).Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, connection.ID).Count(&activeSecrets).Error; err != nil {
		t.Fatal(err)
	}
	if activeSecrets != 0 {
		t.Fatalf("disconnect left %d encrypted OAuth secrets", activeSecrets)
	}
	var credential CredentialReference
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ?", fx.actor.OrganizationID, connection.ID, OAuthCredentialAccessToken).First(&credential).Error; err != nil {
		t.Fatal(err)
	}
	if credential.Status != CredentialRevoked || credential.SecretRef != "" {
		t.Fatalf("credential reference was not revoked: %+v", credential)
	}
}

func TestOAuthExchangeFailureCannotBeRetriedWithSameState(t *testing.T) {
	fx := newOAuthFixture(t)
	connection := createConnection(t, fx.channelFixture, fx.actor, "instagram", OwnershipMerchantManaged)
	_, state := startOAuthSession(t, fx, fx.actor, connection.ID)
	fx.client.exchangeErr = errors.New("provider rejected code")
	if _, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: state, Code: "bad-code"}); err == nil {
		t.Fatal("provider exchange failure should be returned")
	}
	fx.client.exchangeErr = nil
	if _, err := fx.oauth.CompleteCallback(context.Background(), fx.actor, connection.ID, OAuthCallbackInput{State: state, Code: "good-code"}); err == nil {
		t.Fatal("failed state must not be replayed; a new OAuth session is required")
	}
}
