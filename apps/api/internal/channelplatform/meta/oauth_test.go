package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

func TestInstagramOAuthDiscoverySubscriptionRefreshAndRevoke(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/oauth/access_token" && strings.Contains(r.URL.RawQuery, "client_secret") {
			t.Error("client secret must not be placed in a request URL")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/access_token":
			_ = r.ParseForm()
			if r.Form.Get("client_secret") != "app-secret" || r.Form.Get("code") != "auth-code" {
				t.Errorf("unexpected code exchange form: %v", r.Form)
			}
			_, _ = w.Write([]byte(`{"access_token":"short-token","user_id":"1789"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/access_token":
			_, _ = w.Write([]byte(`{"access_token":"long-token","token_type":"bearer","expires_in":5184000}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			assertBearer(t, r, "long-token")
			_, _ = w.Write([]byte(`{"user_id":"1789","username":"zidi_shop","name":"Zidi Shop","account_type":"BUSINESS","profile_picture_url":"https://cdn.example/profile.jpg"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/1789/subscribed_apps":
			assertBearer(t, r, "long-token")
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/refresh_access_token":
			_, _ = w.Write([]byte(`{"access_token":"refreshed-token","token_type":"bearer","expires_in":5184000}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/me/permissions":
			assertBearer(t, r, "refreshed-token")
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.Error(w, `{"error":{"code":100}}`, http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewOAuthClient(OAuthConfig{Provider: ProviderInstagram, AppID: "app-id", AppSecret: "app-secret", InstagramOAuthBaseURL: server.URL, InstagramAPIOAuthBaseURL: server.URL, InstagramGraphBaseURL: server.URL, AdvancedAccess: false}, server.Client())
	client.now = func() time.Time { return time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC) }
	authorizationURL, err := client.AuthorizationURL(channelplatform.OAuthAuthorizationRequest{State: "state", RedirectURI: "https://app.example/callback", Scopes: client.RequestedScopes()})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizationURL)
	if parsed.Path != "/oauth/authorize" || parsed.Query().Get("state") != "state" || !strings.Contains(parsed.Query().Get("scope"), "instagram_business_manage_messages") {
		t.Fatalf("unexpected Instagram authorization URL: %s", authorizationURL)
	}

	grant, err := client.ExchangeCode(context.Background(), channelplatform.OAuthCodeExchangeRequest{Code: "auth-code", RedirectURI: "https://app.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccessToken != "long-token" || grant.RefreshToken != "long-token" || grant.ReviewApproved || grant.AccessExpiresAt == nil {
		t.Fatalf("unexpected Instagram grant: %+v", grant)
	}
	assets, err := client.DiscoverAssets(context.Background(), grant.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || !assets[0].Eligible || assets[0].ProviderAssetID != "1789" || assets[0].ExternalHandle != "zidi_shop" {
		t.Fatalf("unexpected Instagram assets: %+v", assets)
	}
	if _, err := client.ConnectAsset(context.Background(), grant.AccessToken, assets[0]); err != nil {
		t.Fatal(err)
	}
	refreshed, err := client.Refresh(context.Background(), channelplatform.OAuthRefreshRequest{RefreshToken: grant.RefreshToken, Asset: &assets[0]})
	if err != nil || refreshed.AccessToken != "refreshed-token" {
		t.Fatalf("refresh failed: grant=%+v err=%v", refreshed, err)
	}
	if err := client.Revoke(context.Background(), channelplatform.OAuthRevokeRequest{AccessToken: refreshed.AccessToken}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 6 {
		t.Fatalf("unexpected request sequence: %v", requests)
	}
}

func TestFacebookOAuthDiscoversEligiblePagesAndUsesPageToken(t *testing.T) {
	longTokenCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v24.0/oauth/access_token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") == "fb_exchange_token" {
				longTokenCount++
				_, _ = w.Write([]byte(`{"access_token":"user-long-` + string(rune('0'+longTokenCount)) + `","expires_in":5184000}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"user-short","expires_in":3600}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v24.0/me/permissions":
			_, _ = w.Write([]byte(`{"data":[{"permission":"pages_show_list","status":"granted"},{"permission":"pages_manage_metadata","status":"granted"},{"permission":"pages_messaging","status":"declined"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v24.0/me/accounts":
			assertBearer(t, r, "user-long-1")
			_, _ = w.Write([]byte(`{"data":[{"id":"page-1","name":"Zidi Page","category":"Retail","tasks":["MESSAGING"]},{"id":"page-2","name":"Read Only","category":"Community","tasks":["ANALYZE"]}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v24.0/page-1":
			_, _ = w.Write([]byte(`{"id":"page-1","access_token":"page-token"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v24.0/page-1/subscribed_apps":
			assertBearer(t, r, "page-token")
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v24.0/me/permissions":
			assertBearer(t, r, "user-long-1")
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.Error(w, `{"error":{"code":100}}`, http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewOAuthClient(OAuthConfig{Provider: ProviderFacebook, AppID: "app-id", AppSecret: "app-secret", GraphVersion: "v24.0", FacebookOAuthBaseURL: server.URL, FacebookGraphBaseURL: server.URL, AdvancedAccess: true}, server.Client())
	grant, err := client.ExchangeCode(context.Background(), channelplatform.OAuthCodeExchangeRequest{Code: "auth-code", RedirectURI: "https://app.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccessToken != "user-long-1" || len(grant.GrantedScopes) != 2 || !grant.ReviewApproved {
		t.Fatalf("unexpected Facebook grant: %+v", grant)
	}
	assets, err := client.DiscoverAssets(context.Background(), grant.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 || !assets[0].Eligible || assets[1].Eligible || !strings.Contains(assets[1].IneligibleReason, "messaging task") {
		t.Fatalf("unexpected Facebook assets: %+v", assets)
	}
	assetGrant, err := client.ConnectAsset(context.Background(), grant.AccessToken, assets[0])
	if err != nil {
		t.Fatal(err)
	}
	if assetGrant.AccessToken != "page-token" || assetGrant.RefreshToken != "user-long-1" {
		t.Fatalf("selected Page credential was not returned: %+v", assetGrant)
	}
	if err := client.Revoke(context.Background(), channelplatform.OAuthRevokeRequest{AccessToken: "page-token", RefreshToken: "user-long-1", Asset: &assets[0]}); err != nil {
		t.Fatal(err)
	}
}

func TestMetaErrorsDoNotExposeProviderMessagesOrTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 190, "message": "Invalid token secret-token"}})
	}))
	defer server.Close()
	client := NewOAuthClient(OAuthConfig{Provider: ProviderFacebook, AppID: "app-id", AppSecret: "app-secret", FacebookGraphBaseURL: server.URL}, server.Client())
	_, err := client.DiscoverAssets(context.Background(), "secret-token")
	if err == nil || strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "Invalid token") {
		t.Fatalf("provider error was not safely redacted: %v", err)
	}
}

func assertBearer(t *testing.T, request *http.Request, expected string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+expected {
		t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
	}
}
