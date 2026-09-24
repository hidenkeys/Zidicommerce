package tiktok

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

func TestTikTokLoginKitLifecycleAndProfileDiscovery(t *testing.T) {
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/oauth/token/":
			tokenCalls++
			_ = r.ParseForm()
			if r.Form.Get("client_secret") != "client-secret" {
				t.Error("TikTok token exchange did not send the client secret in the request body")
			}
			if tokenCalls == 1 {
				if r.Form.Get("code_verifier") != "pkce-verifier" || r.Form.Get("code") != "auth-code" {
					t.Errorf("unexpected authorization-code form: %v", r.Form)
				}
				_, _ = w.Write([]byte(`{"access_token":"access-1","expires_in":86400,"open_id":"open-1","refresh_token":"refresh-1","refresh_expires_in":31536000,"scope":"user.info.basic","token_type":"Bearer"}`))
				return
			}
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-1" {
				t.Errorf("unexpected refresh form: %v", r.Form)
			}
			_, _ = w.Write([]byte(`{"access_token":"access-2","expires_in":86400,"open_id":"open-1","refresh_token":"refresh-2","refresh_expires_in":31536000,"scope":"user.info.basic","token_type":"Bearer"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/user/info/":
			if r.Header.Get("Authorization") != "Bearer access-1" {
				t.Errorf("unexpected profile token: %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"data":{"user":{"open_id":"open-1","union_id":"union-1","avatar_url":"https://cdn.example/avatar.jpg","display_name":"Zidi Creator","username":"zidi_creator"}},"error":{"code":"ok","message":"","log_id":"log-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/oauth/revoke/":
			_ = r.ParseForm()
			if r.Form.Get("token") != "access-2" {
				t.Errorf("unexpected revoke token")
			}
			_, _ = w.Write([]byte(`{"error":{"code":"ok","message":"","log_id":"log-2"}}`))
		default:
			http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewOAuthClient(OAuthConfig{ClientKey: "client-key", ClientSecret: "client-secret", AuthorizeURL: server.URL + "/authorize/", APIBaseURL: server.URL, VerifiedAccess: false}, server.Client())
	client.now = func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) }
	authorizationURL, err := client.AuthorizationURL(channelplatform.OAuthAuthorizationRequest{State: "state", CodeChallenge: "challenge", RedirectURI: "https://app.example/callback", Scopes: client.RequestedScopes()})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizationURL)
	if parsed.Query().Get("client_key") != "client-key" || parsed.Query().Get("code_challenge") != "challenge" || parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("state") != "state" {
		t.Fatalf("unexpected TikTok authorization URL: %s", authorizationURL)
	}
	grant, err := client.ExchangeCode(context.Background(), channelplatform.OAuthCodeExchangeRequest{Code: "auth-code", CodeVerifier: "pkce-verifier", RedirectURI: "https://app.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccessToken != "access-1" || grant.RefreshToken != "refresh-1" || len(grant.GrantedScopes) != 1 || grant.ReviewApproved || grant.AccessExpiresAt == nil || grant.RefreshExpiresAt == nil {
		t.Fatalf("unexpected TikTok grant: %+v", grant)
	}
	assets, err := client.DiscoverAssets(context.Background(), grant.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].ProviderAssetID != "open-1" || assets[0].AssetType != "tiktok_account" || assets[0].ExternalHandle != "zidi_creator" {
		t.Fatalf("unexpected TikTok profile asset: %+v", assets)
	}
	if _, err := client.ConnectAsset(context.Background(), grant.AccessToken, assets[0]); err != nil {
		t.Fatal(err)
	}
	refreshed, err := client.Refresh(context.Background(), channelplatform.OAuthRefreshRequest{AccessToken: grant.AccessToken, RefreshToken: grant.RefreshToken, Asset: &assets[0]})
	if err != nil || refreshed.AccessToken != "access-2" || refreshed.RefreshToken != "refresh-2" {
		t.Fatalf("TikTok refresh failed: grant=%+v err=%v", refreshed, err)
	}
	if err := client.Revoke(context.Background(), channelplatform.OAuthRevokeRequest{AccessToken: refreshed.AccessToken, RefreshToken: refreshed.RefreshToken, Asset: &assets[0]}); err != nil {
		t.Fatal(err)
	}
}

func TestTikTokMessagingIsNotRequestedOrRegistered(t *testing.T) {
	client := NewOAuthClient(OAuthConfig{ClientKey: "key", ClientSecret: "secret"}, nil)
	if strings.Join(client.RequestedScopes(), ",") != "user.info.basic" {
		t.Fatalf("TikTok Login Kit requested unsupported scopes: %v", client.RequestedScopes())
	}
	for _, provider := range channelplatform.ProviderCatalog() {
		if provider.Provider != Provider {
			continue
		}
		for _, capability := range provider.Capabilities {
			if strings.Contains(capability.Capability, "text") || strings.Contains(capability.Capability, "media") || capability.Capability == "commerce_actions" || capability.Capability == "human_handoff" {
				if capability.Support != channelplatform.ProviderSupportUnavailable {
					t.Fatalf("TikTok messaging capability was presented as supported: %+v", capability)
				}
			}
		}
		return
	}
	t.Fatal("TikTok provider catalog entry is missing")
}
