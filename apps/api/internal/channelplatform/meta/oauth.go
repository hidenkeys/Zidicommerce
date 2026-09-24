package meta

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

const (
	ProviderInstagram = "instagram"
	ProviderFacebook  = "facebook"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type OAuthConfig struct {
	Provider                 string
	AppID                    string
	AppSecret                string
	GraphVersion             string
	FacebookOAuthBaseURL     string
	FacebookGraphBaseURL     string
	InstagramOAuthBaseURL    string
	InstagramAPIOAuthBaseURL string
	InstagramGraphBaseURL    string
	AdvancedAccess           bool
}

type OAuthClient struct {
	config OAuthConfig
	client HTTPClient
	now    func() time.Time
}

func NewOAuthClient(config OAuthConfig, client HTTPClient) *OAuthClient {
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	config.AppID = strings.TrimSpace(config.AppID)
	config.AppSecret = strings.TrimSpace(config.AppSecret)
	config.GraphVersion = strings.Trim(strings.TrimSpace(config.GraphVersion), "/")
	if config.GraphVersion == "" {
		config.GraphVersion = "v24.0"
	}
	config.FacebookOAuthBaseURL = defaultBaseURL(config.FacebookOAuthBaseURL, "https://www.facebook.com")
	config.FacebookGraphBaseURL = defaultBaseURL(config.FacebookGraphBaseURL, "https://graph.facebook.com")
	config.InstagramOAuthBaseURL = defaultBaseURL(config.InstagramOAuthBaseURL, "https://www.instagram.com")
	config.InstagramAPIOAuthBaseURL = defaultBaseURL(config.InstagramAPIOAuthBaseURL, "https://api.instagram.com")
	config.InstagramGraphBaseURL = defaultBaseURL(config.InstagramGraphBaseURL, "https://graph.instagram.com")
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &OAuthClient{config: config, client: client, now: func() time.Time { return time.Now().UTC() }}
}

func (c *OAuthClient) Enabled() bool {
	return (c.config.Provider == ProviderInstagram || c.config.Provider == ProviderFacebook) && c.config.AppID != "" && c.config.AppSecret != ""
}

func (c *OAuthClient) Provider() string { return c.config.Provider }

func (c *OAuthClient) RequestedScopes() []string {
	if c.config.Provider == ProviderInstagram {
		return []string{"instagram_business_basic", "instagram_business_manage_messages"}
	}
	return []string{"pages_manage_metadata", "pages_messaging", "pages_read_engagement", "pages_show_list"}
}

func (c *OAuthClient) AuthorizationURL(input channelplatform.OAuthAuthorizationRequest) (string, error) {
	if !c.Enabled() {
		return "", errors.New("Meta OAuth client is not configured")
	}
	values := url.Values{
		"client_id":     []string{c.config.AppID},
		"redirect_uri":  []string{input.RedirectURI},
		"response_type": []string{"code"},
		"scope":         []string{strings.Join(input.Scopes, ",")},
		"state":         []string{input.State},
	}
	if c.config.Provider == ProviderInstagram {
		values.Set("enable_fb_login", "0")
		values.Set("force_authentication", "1")
		return c.config.InstagramOAuthBaseURL + "/oauth/authorize?" + values.Encode(), nil
	}
	values.Set("auth_type", "rerequest")
	return c.config.FacebookOAuthBaseURL + "/" + c.config.GraphVersion + "/dialog/oauth?" + values.Encode(), nil
}

func (c *OAuthClient) ExchangeCode(ctx context.Context, input channelplatform.OAuthCodeExchangeRequest) (channelplatform.OAuthTokenGrant, error) {
	if c.config.Provider == ProviderInstagram {
		return c.exchangeInstagramCode(ctx, input)
	}
	return c.exchangeFacebookCode(ctx, input)
}

func (c *OAuthClient) DiscoverAssets(ctx context.Context, accessToken string) ([]channelplatform.OAuthAsset, error) {
	if c.config.Provider == ProviderInstagram {
		return c.discoverInstagramAccount(ctx, accessToken)
	}
	return c.discoverFacebookPages(ctx, accessToken)
}

func (c *OAuthClient) ConnectAsset(ctx context.Context, accessToken string, asset channelplatform.OAuthAsset) (channelplatform.OAuthTokenGrant, error) {
	if c.config.Provider == ProviderInstagram {
		if err := c.subscribeInstagram(ctx, accessToken, asset.ProviderAssetID); err != nil {
			return channelplatform.OAuthTokenGrant{}, err
		}
		return channelplatform.OAuthTokenGrant{}, nil
	}
	pageToken, err := c.facebookPageToken(ctx, accessToken, asset.ProviderAssetID)
	if err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	if err := c.subscribeFacebookPage(ctx, pageToken, asset.ProviderAssetID); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	return channelplatform.OAuthTokenGrant{AccessToken: pageToken, RefreshToken: accessToken, GrantedScopes: c.RequestedScopes(), ReviewApproved: c.config.AdvancedAccess}, nil
}

func (c *OAuthClient) Refresh(ctx context.Context, input channelplatform.OAuthRefreshRequest) (channelplatform.OAuthTokenGrant, error) {
	if c.config.Provider == ProviderInstagram {
		return c.refreshInstagram(ctx, input.RefreshToken)
	}
	if input.Asset == nil || strings.TrimSpace(input.Asset.ProviderAssetID) == "" {
		return channelplatform.OAuthTokenGrant{}, errors.New("selected Facebook Page is required")
	}
	userGrant, err := c.extendFacebookToken(ctx, input.RefreshToken)
	if err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	pageToken, err := c.facebookPageToken(ctx, userGrant.AccessToken, input.Asset.ProviderAssetID)
	if err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	if err := c.subscribeFacebookPage(ctx, pageToken, input.Asset.ProviderAssetID); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	return channelplatform.OAuthTokenGrant{AccessToken: pageToken, RefreshToken: userGrant.AccessToken, GrantedScopes: userGrant.GrantedScopes, AccessExpiresAt: userGrant.AccessExpiresAt, RefreshExpiresAt: userGrant.RefreshExpiresAt, ReviewApproved: c.config.AdvancedAccess}, nil
}

func (c *OAuthClient) Revoke(ctx context.Context, input channelplatform.OAuthRevokeRequest) error {
	token := input.AccessToken
	baseURL := c.config.InstagramGraphBaseURL
	if c.config.Provider == ProviderFacebook {
		baseURL = c.facebookGraphURL()
		if strings.TrimSpace(input.RefreshToken) != "" {
			token = input.RefreshToken
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/me/permissions", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	return c.doJSON(request, nil)
}

func (c *OAuthClient) exchangeInstagramCode(ctx context.Context, input channelplatform.OAuthCodeExchangeRequest) (channelplatform.OAuthTokenGrant, error) {
	form := url.Values{
		"client_id":     []string{c.config.AppID},
		"client_secret": []string{c.config.AppSecret},
		"code":          []string{input.Code},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{input.RedirectURI},
	}
	var short instagramTokenResponse
	if err := c.postForm(ctx, c.config.InstagramAPIOAuthBaseURL+"/oauth/access_token", form, &short); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	shortToken := short.AccessToken
	if shortToken == "" && len(short.Data) > 0 {
		shortToken = short.Data[0].AccessToken
	}
	if shortToken == "" {
		return channelplatform.OAuthTokenGrant{}, errors.New("Instagram authorization returned no access token")
	}
	values := url.Values{"grant_type": []string{"ig_exchange_token"}, "client_secret": []string{c.config.AppSecret}, "access_token": []string{shortToken}}
	var long metaTokenResponse
	if err := c.getJSON(ctx, c.config.InstagramGraphBaseURL+"/access_token?"+values.Encode(), "", &long); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	return c.instagramGrant(long.AccessToken, long.ExpiresIn), nil
}

func (c *OAuthClient) exchangeFacebookCode(ctx context.Context, input channelplatform.OAuthCodeExchangeRequest) (channelplatform.OAuthTokenGrant, error) {
	form := url.Values{
		"client_id":     []string{c.config.AppID},
		"client_secret": []string{c.config.AppSecret},
		"code":          []string{input.Code},
		"redirect_uri":  []string{input.RedirectURI},
	}
	var short metaTokenResponse
	if err := c.postForm(ctx, c.facebookGraphURL()+"/oauth/access_token", form, &short); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	if short.AccessToken == "" {
		return channelplatform.OAuthTokenGrant{}, errors.New("Facebook authorization returned no access token")
	}
	return c.extendFacebookToken(ctx, short.AccessToken)
}

func (c *OAuthClient) extendFacebookToken(ctx context.Context, token string) (channelplatform.OAuthTokenGrant, error) {
	form := url.Values{
		"client_id":         []string{c.config.AppID},
		"client_secret":     []string{c.config.AppSecret},
		"fb_exchange_token": []string{token},
		"grant_type":        []string{"fb_exchange_token"},
	}
	var response metaTokenResponse
	if err := c.postForm(ctx, c.facebookGraphURL()+"/oauth/access_token", form, &response); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	if response.AccessToken == "" {
		return channelplatform.OAuthTokenGrant{}, errors.New("Facebook token exchange returned no access token")
	}
	scopes, err := c.facebookGrantedScopes(ctx, response.AccessToken)
	if err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	grant := channelplatform.OAuthTokenGrant{AccessToken: response.AccessToken, RefreshToken: response.AccessToken, GrantedScopes: scopes, ReviewApproved: c.config.AdvancedAccess}
	if response.ExpiresIn > 0 {
		expiresAt := c.now().Add(time.Duration(response.ExpiresIn) * time.Second)
		grant.AccessExpiresAt = &expiresAt
		grant.RefreshExpiresAt = &expiresAt
	}
	return grant, nil
}

func (c *OAuthClient) refreshInstagram(ctx context.Context, token string) (channelplatform.OAuthTokenGrant, error) {
	values := url.Values{"grant_type": []string{"ig_refresh_token"}, "access_token": []string{token}}
	var response metaTokenResponse
	if err := c.getJSON(ctx, c.config.InstagramGraphBaseURL+"/refresh_access_token?"+values.Encode(), "", &response); err != nil {
		return channelplatform.OAuthTokenGrant{}, err
	}
	return c.instagramGrant(response.AccessToken, response.ExpiresIn), nil
}

func (c *OAuthClient) instagramGrant(token string, expiresIn int64) channelplatform.OAuthTokenGrant {
	grant := channelplatform.OAuthTokenGrant{AccessToken: token, RefreshToken: token, GrantedScopes: c.RequestedScopes(), ReviewApproved: c.config.AdvancedAccess}
	if expiresIn > 0 {
		expiresAt := c.now().Add(time.Duration(expiresIn) * time.Second)
		grant.AccessExpiresAt = &expiresAt
		grant.RefreshExpiresAt = &expiresAt
	}
	return grant
}

func (c *OAuthClient) discoverInstagramAccount(ctx context.Context, token string) ([]channelplatform.OAuthAsset, error) {
	var profile struct {
		ID                string `json:"id"`
		UserID            string `json:"user_id"`
		Username          string `json:"username"`
		Name              string `json:"name"`
		AccountType       string `json:"account_type"`
		ProfilePictureURL string `json:"profile_picture_url"`
	}
	endpoint := c.config.InstagramGraphBaseURL + "/me?fields=user_id,username,name,account_type,profile_picture_url"
	if err := c.getJSON(ctx, endpoint, token, &profile); err != nil {
		return nil, err
	}
	id := first(profile.UserID, profile.ID)
	eligible := strings.EqualFold(profile.AccountType, "BUSINESS") || strings.EqualFold(profile.AccountType, "MEDIA_CREATOR") || strings.EqualFold(profile.AccountType, "CREATOR")
	reason := ""
	if !eligible {
		reason = "Instagram account must be a professional Business or Creator account"
	}
	return []channelplatform.OAuthAsset{{ProviderAssetID: id, AssetType: "instagram_business_account", DisplayName: first(profile.Name, profile.Username), ExternalHandle: profile.Username, Eligible: eligible, IneligibleReason: reason, Metadata: map[string]any{"account_type": profile.AccountType, "profile_picture_url": profile.ProfilePictureURL}}}, nil
}

func (c *OAuthClient) discoverFacebookPages(ctx context.Context, token string) ([]channelplatform.OAuthAsset, error) {
	var response struct {
		Data []struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Category string   `json:"category"`
			Tasks    []string `json:"tasks"`
		} `json:"data"`
	}
	endpoint := c.facebookGraphURL() + "/me/accounts?fields=id,name,category,tasks&limit=100"
	if err := c.getJSON(ctx, endpoint, token, &response); err != nil {
		return nil, err
	}
	assets := make([]channelplatform.OAuthAsset, 0, len(response.Data))
	for _, page := range response.Data {
		eligible := containsFold(page.Tasks, "MESSAGING") || containsFold(page.Tasks, "MODERATE")
		reason := ""
		if !eligible {
			reason = "The authorized user does not have a messaging task on this Page"
		}
		assets = append(assets, channelplatform.OAuthAsset{ProviderAssetID: page.ID, AssetType: "page", DisplayName: page.Name, Eligible: eligible, IneligibleReason: reason, Metadata: map[string]any{"category": page.Category, "tasks": page.Tasks}})
	}
	return assets, nil
}

func (c *OAuthClient) subscribeInstagram(ctx context.Context, token, accountID string) error {
	form := url.Values{"subscribed_fields": []string{"messages,messaging_postbacks,messaging_seen,message_reactions"}}
	return c.postFormBearer(ctx, c.config.InstagramGraphBaseURL+"/"+url.PathEscape(accountID)+"/subscribed_apps", token, form, nil)
}

func (c *OAuthClient) facebookPageToken(ctx context.Context, userToken, pageID string) (string, error) {
	var response struct {
		ID          string `json:"id"`
		AccessToken string `json:"access_token"`
	}
	endpoint := c.facebookGraphURL() + "/" + url.PathEscape(pageID) + "?fields=id,access_token"
	if err := c.getJSON(ctx, endpoint, userToken, &response); err != nil {
		return "", err
	}
	if response.ID != pageID || response.AccessToken == "" {
		return "", errors.New("Facebook did not return a token for the selected Page")
	}
	return response.AccessToken, nil
}

func (c *OAuthClient) subscribeFacebookPage(ctx context.Context, pageToken, pageID string) error {
	form := url.Values{"subscribed_fields": []string{"messages,messaging_postbacks,message_deliveries,message_reads,message_reactions"}}
	return c.postFormBearer(ctx, c.facebookGraphURL()+"/"+url.PathEscape(pageID)+"/subscribed_apps", pageToken, form, nil)
}

func (c *OAuthClient) facebookGrantedScopes(ctx context.Context, token string) ([]string, error) {
	var response struct {
		Data []struct {
			Permission string `json:"permission"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, c.facebookGraphURL()+"/me/permissions", token, &response); err != nil {
		return nil, err
	}
	scopes := make([]string, 0, len(response.Data))
	for _, permission := range response.Data {
		if strings.EqualFold(permission.Status, "granted") {
			scopes = append(scopes, permission.Permission)
		}
	}
	return scopes, nil
}

func (c *OAuthClient) getJSON(ctx context.Context, endpoint, token string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return c.doJSON(request, target)
}

func (c *OAuthClient) postForm(ctx context.Context, endpoint string, form url.Values, target any) error {
	return c.postFormBearer(ctx, endpoint, "", form, target)
}

func (c *OAuthClient) postFormBearer(ctx context.Context, endpoint, token string, form url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return c.doJSON(request, target)
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
		var providerError struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &providerError)
		return fmt.Errorf("Meta API request failed with status %d and code %d", response.StatusCode, providerError.Error.Code)
	}
	if target == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("Meta API returned an invalid response")
	}
	return nil
}

func (c *OAuthClient) facebookGraphURL() string {
	return c.config.FacebookGraphBaseURL + "/" + c.config.GraphVersion
}

type metaTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type instagramTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      any    `json:"user_id"`
	Data        []struct {
		AccessToken string `json:"access_token"`
		UserID      any    `json:"user_id"`
	} `json:"data"`
}

func defaultBaseURL(value, fallback string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return fallback
	}
	return value
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), expected) {
			return true
		}
	}
	return false
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
