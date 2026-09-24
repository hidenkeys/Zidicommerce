package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

const Provider = "tiktok"

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type OAuthConfig struct {
	ClientKey      string
	ClientSecret   string
	AuthorizeURL   string
	APIBaseURL     string
	VerifiedAccess bool
}

type OAuthClient struct {
	config OAuthConfig
	client HTTPClient
	now    func() time.Time
}

func NewOAuthClient(config OAuthConfig, client HTTPClient) *OAuthClient {
	config.ClientKey = strings.TrimSpace(config.ClientKey)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.AuthorizeURL = defaultURL(config.AuthorizeURL, "https://www.tiktok.com/v2/auth/authorize/")
	config.APIBaseURL = strings.TrimRight(defaultURL(config.APIBaseURL, "https://open.tiktokapis.com"), "/")
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &OAuthClient{config: config, client: client, now: func() time.Time { return time.Now().UTC() }}
}

func (c *OAuthClient) Enabled() bool {
	return c.config.ClientKey != "" && c.config.ClientSecret != ""
}

func (c *OAuthClient) Provider() string { return Provider }

func (c *OAuthClient) RequestedScopes() []string { return []string{"user.info.basic"} }

func (c *OAuthClient) AuthorizationURL(input channelplatform.OAuthAuthorizationRequest) (string, error) {
	if !c.Enabled() {
		return "", errors.New("TikTok OAuth client is not configured")
	}
	values := url.Values{
		"client_key":            []string{c.config.ClientKey},
		"response_type":         []string{"code"},
		"scope":                 []string{strings.Join(input.Scopes, ",")},
		"redirect_uri":          []string{input.RedirectURI},
		"state":                 []string{input.State},
		"code_challenge":        []string{input.CodeChallenge},
		"code_challenge_method": []string{"S256"},
	}
	return c.config.AuthorizeURL + "?" + values.Encode(), nil
}

func (c *OAuthClient) ExchangeCode(ctx context.Context, input channelplatform.OAuthCodeExchangeRequest) (channelplatform.OAuthTokenGrant, error) {
	form := url.Values{
		"client_key":    []string{c.config.ClientKey},
		"client_secret": []string{c.config.ClientSecret},
		"code":          []string{input.Code},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{input.RedirectURI},
		"code_verifier": []string{input.CodeVerifier},
	}
	return c.tokenRequest(ctx, form)
}

func (c *OAuthClient) DiscoverAssets(ctx context.Context, accessToken string) ([]channelplatform.OAuthAsset, error) {
	var response struct {
		Data struct {
			User struct {
				OpenID      string `json:"open_id"`
				UnionID     string `json:"union_id"`
				AvatarURL   string `json:"avatar_url"`
				DisplayName string `json:"display_name"`
				Username    string `json:"username"`
			} `json:"user"`
		} `json:"data"`
		Error tikTokError `json:"error"`
	}
	endpoint := c.config.APIBaseURL + "/v2/user/info/?fields=open_id,union_id,avatar_url,display_name,username"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if err := c.doJSON(request, &response); err != nil {
		return nil, err
	}
	if response.Error.Code != "" && response.Error.Code != "ok" {
		return nil, fmt.Errorf("TikTok API request failed with code %s", response.Error.Code)
	}
	user := response.Data.User
	if user.OpenID == "" {
		return nil, errors.New("TikTok profile response did not include an account identity")
	}
	return []channelplatform.OAuthAsset{{ProviderAssetID: user.OpenID, AssetType: "tiktok_account", DisplayName: first(user.DisplayName, user.Username, "TikTok account"), ExternalHandle: user.Username, Eligible: true, Metadata: map[string]any{"union_id": user.UnionID, "avatar_url": user.AvatarURL}}}, nil
}

func (c *OAuthClient) ConnectAsset(context.Context, string, channelplatform.OAuthAsset) (channelplatform.OAuthTokenGrant, error) {
	return channelplatform.OAuthTokenGrant{}, nil
}

func (c *OAuthClient) Refresh(ctx context.Context, input channelplatform.OAuthRefreshRequest) (channelplatform.OAuthTokenGrant, error) {
	form := url.Values{
		"client_key":    []string{c.config.ClientKey},
		"client_secret": []string{c.config.ClientSecret},
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{input.RefreshToken},
	}
	return c.tokenRequest(ctx, form)
}

func (c *OAuthClient) Revoke(ctx context.Context, input channelplatform.OAuthRevokeRequest) error {
	token := input.AccessToken
	if token == "" {
		token = input.RefreshToken
	}
	form := url.Values{"client_key": []string{c.config.ClientKey}, "client_secret": []string{c.config.ClientSecret}, "token": []string{token}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.APIBaseURL+"/v2/oauth/revoke/", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		Error tikTokError `json:"error"`
	}
	if err := c.doJSON(request, &response); err != nil {
		return err
	}
	if response.Error.Code != "" && response.Error.Code != "ok" {
		return fmt.Errorf("TikTok revoke failed with code %s", response.Error.Code)
	}
	return nil
}

func (c *OAuthClient) tokenRequest(ctx context.Context, form url.Values) (channelplatform.OAuthTokenGrant, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.APIBaseURL+"/v2/oauth/token/", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		AccessToken      string `json:"access_token"`
		ExpiresIn        int64  `json:"expires_in"`
		OpenID           string `json:"open_id"`
		RefreshToken     string `json:"refresh_token"`
		RefreshExpiresIn int64  `json:"refresh_expires_in"`
		Scope            string `json:"scope"`
		TokenType        string `json:"token_type"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := c.doJSON(request, &response); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	if response.Error != "" || response.AccessToken == "" || response.RefreshToken == "" {
		return channelplatform.OAuthTokenGrant{}, errors.New("TikTok authorization did not return usable credentials")
	}
	grant := channelplatform.OAuthTokenGrant{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, GrantedScopes: splitScopes(response.Scope), ReviewApproved: c.config.VerifiedAccess}
	if response.ExpiresIn > 0 {
		expiresAt := c.now().Add(time.Duration(response.ExpiresIn) * time.Second)
		grant.AccessExpiresAt = &expiresAt
	}
	if response.RefreshExpiresIn > 0 {
		expiresAt := c.now().Add(time.Duration(response.RefreshExpiresIn) * time.Second)
		grant.RefreshExpiresAt = &expiresAt
	}
	return grant, nil
}

func (c *OAuthClient) doJSON(request *http.Request, target any) error {
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("TikTok API request failed with status %d", response.StatusCode)
	}
	if target == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if json.Unmarshal(body, target) != nil {
		return errors.New("TikTok API returned an invalid response")
	}
	return nil
}

type tikTokError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	LogID   string `json:"log_id"`
}

func splitScopes(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !seen[part] {
			seen[part] = true
			result = append(result, part)
		}
	}
	return result
}

func defaultURL(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
