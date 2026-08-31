package whatsapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const signupAttemptTTL = 15 * time.Minute

type EmbeddedSignupConfig struct {
	AppID              string
	AppSecret          string
	ConfigurationID    string
	GraphAPIVersion    string
	WebhookVerifyToken string
}

type MetaAccessGrant struct {
	AccessToken string
	ExpiresAt   *time.Time
}

type MetaBusinessAssets struct {
	BusinessName       string
	DisplayPhoneNumber string
}

type EmbeddedSignupClient interface {
	ExchangeCode(context.Context, string) (MetaAccessGrant, error)
	ValidateAssets(context.Context, string, string, string) (MetaBusinessAssets, error)
	SubscribeApp(context.Context, string, string) error
}

type metaHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type MetaGraphClient struct {
	baseURL    string
	appID      string
	appSecret  string
	version    string
	httpClient metaHTTPClient
}

func NewMetaGraphClient(baseURL, appID, appSecret, version string, client metaHTTPClient) *MetaGraphClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &MetaGraphClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), appID: strings.TrimSpace(appID),
		appSecret: strings.TrimSpace(appSecret), version: strings.Trim(strings.TrimSpace(version), "/"), httpClient: client,
	}
}

func (c *MetaGraphClient) ExchangeCode(ctx context.Context, code string) (MetaAccessGrant, error) {
	form := url.Values{}
	form.Set("client_id", c.appID)
	form.Set("client_secret", c.appSecret)
	form.Set("code", strings.TrimSpace(code))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("oauth/access_token"), strings.NewReader(form.Encode()))
	if err != nil {
		return MetaAccessGrant{}, errors.New("could not prepare Meta authorization exchange")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.do(request, &response); err != nil {
		return MetaAccessGrant{}, err
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return MetaAccessGrant{}, errors.New("Meta authorization did not return an access token")
	}
	grant := MetaAccessGrant{AccessToken: response.AccessToken}
	if response.ExpiresIn > 0 {
		expiresAt := time.Now().UTC().Add(time.Duration(response.ExpiresIn) * time.Second)
		grant.ExpiresAt = &expiresAt
	}
	return grant, nil
}

func (c *MetaGraphClient) ValidateAssets(ctx context.Context, accessToken, wabaID, phoneNumberID string) (MetaBusinessAssets, error) {
	fields := url.Values{}
	fields.Set("fields", "id,name")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(url.PathEscape(wabaID))+"?"+fields.Encode(), nil)
	if err != nil {
		return MetaBusinessAssets{}, errors.New("could not prepare Meta business validation")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	var account struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(request, &account); err != nil {
		return MetaBusinessAssets{}, err
	}
	if account.ID != wabaID {
		return MetaBusinessAssets{}, errors.New("Meta returned a different WhatsApp Business Account")
	}

	phoneQuery := url.Values{}
	phoneQuery.Set("fields", "id,display_phone_number,verified_name")
	phoneQuery.Set("limit", "100")
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(url.PathEscape(wabaID)+"/phone_numbers")+"?"+phoneQuery.Encode(), nil)
	if err != nil {
		return MetaBusinessAssets{}, errors.New("could not prepare Meta phone validation")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	var phones struct {
		Data []struct {
			ID                 string `json:"id"`
			DisplayPhoneNumber string `json:"display_phone_number"`
			VerifiedName       string `json:"verified_name"`
		} `json:"data"`
	}
	if err := c.do(request, &phones); err != nil {
		return MetaBusinessAssets{}, err
	}
	for _, phone := range phones.Data {
		if phone.ID == phoneNumberID {
			businessName := strings.TrimSpace(account.Name)
			if businessName == "" {
				businessName = strings.TrimSpace(phone.VerifiedName)
			}
			return MetaBusinessAssets{BusinessName: businessName, DisplayPhoneNumber: strings.TrimSpace(phone.DisplayPhoneNumber)}, nil
		}
	}
	return MetaBusinessAssets{}, errors.New("The selected phone number does not belong to the authorized WhatsApp Business Account")
}

func (c *MetaGraphClient) SubscribeApp(ctx context.Context, accessToken, wabaID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(url.PathEscape(wabaID)+"/subscribed_apps"), strings.NewReader(""))
	if err != nil {
		return errors.New("could not prepare Meta webhook subscription")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		Success bool `json:"success"`
	}
	if err := c.do(request, &response); err != nil {
		return err
	}
	if !response.Success {
		return errors.New("Meta did not confirm the WhatsApp webhook subscription")
	}
	return nil
}

func (c *MetaGraphClient) endpoint(path string) string {
	return c.baseURL + "/" + c.version + "/" + strings.TrimLeft(path, "/")
}

func (c *MetaGraphClient) do(request *http.Request, target any) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return errors.New("Meta is temporarily unavailable")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return errors.New("Could not read Meta's response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var providerError struct {
			Error struct {
				Code int    `json:"code"`
				Type string `json:"type"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &providerError)
		if providerError.Error.Code != 0 {
			return fmt.Errorf("Meta rejected the request (code %d)", providerError.Error.Code)
		}
		return fmt.Errorf("Meta rejected the request (HTTP %d)", response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("Meta returned an invalid response")
	}
	return nil
}

func (s *Service) ConfigureEmbeddedSignup(config EmbeddedSignupConfig, client EmbeddedSignupClient) {
	config.AppID = strings.TrimSpace(config.AppID)
	config.AppSecret = strings.TrimSpace(config.AppSecret)
	config.ConfigurationID = strings.TrimSpace(config.ConfigurationID)
	config.GraphAPIVersion = strings.Trim(strings.TrimSpace(config.GraphAPIVersion), "/")
	config.WebhookVerifyToken = strings.TrimSpace(config.WebhookVerifyToken)
	s.embeddedConfig = config
	s.embeddedClient = client
}

func (s *Service) embeddedSignupAvailable() bool {
	config := s.embeddedConfig
	return s.encrypted != nil && s.embeddedClient != nil && validMetaID(config.AppID) && config.AppSecret != "" && validMetaID(config.ConfigurationID) && validGraphVersion(config.GraphAPIVersion) && config.WebhookVerifyToken != ""
}

func (s *Service) InitiateEmbeddedSignup(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (EmbeddedSignupInitiation, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return EmbeddedSignupInitiation{}, httperror.Forbidden("You do not have permission to connect WhatsApp")
	}
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return EmbeddedSignupInitiation{}, err
	}
	if detail.Connection.Provider != Provider || detail.Connection.Status == channelplatform.StatusArchived {
		return EmbeddedSignupInitiation{}, httperror.BadRequest("An active WhatsApp setup record is required")
	}
	if !s.embeddedSignupAvailable() {
		return EmbeddedSignupInitiation{}, httperror.BadRequest("Connect with Meta is not configured for this Zidi environment")
	}
	token, err := randomSignupToken()
	if err != nil {
		return EmbeddedSignupInitiation{}, err
	}
	now := s.now()
	attempt := MetaSignupAttempt{
		ID: uuid.New(), OrganizationID: actor.OrganizationID, ConnectionID: connectionID,
		RequestedByUserID: &actor.ID, TokenHash: signupTokenHash(token), Status: SignupAttemptInitiated,
		ExpiresAt: now.Add(signupAttemptTTL), CreatedAt: now, UpdatedAt: now,
	}
	configuration, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return EmbeddedSignupInitiation{}, err
	}
	if configuration.ID == uuid.Nil {
		configuration.ID = uuid.New()
		configuration.CreatedAt = now
	}
	configuration.ConnectionMethod = ConnectionMethodEmbeddedSignup
	configuration.AuthorizationStatus = AuthorizationPending
	configuration.LastAuthorizationError = ""
	configuration.GraphAPIVersion = s.embeddedConfig.GraphAPIVersion
	configuration.UpdatedAt = now
	if configuration.WebhookStatus == "" {
		configuration.WebhookStatus = WebhookNotConfigured
	}
	if configuration.SetupState == "" || configuration.SetupState == SetupNotConnected {
		configuration.SetupState = SetupStarted
	}
	if configuration.AssistedSetupStatus == "" {
		configuration.AssistedSetupStatus = AssistedNotRequested
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"connection_method", "authorization_status", "last_authorization_error", "graph_api_version", "setup_state", "updated_at"}),
		}).Create(&configuration).Error; err != nil {
			return err
		}
		if detail.Connection.Status != channelplatform.StatusConnected && detail.Connection.Status != channelplatform.StatusHealthy {
			if err := tx.Model(&channelplatform.ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": channelplatform.StatusConnecting, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_embedded_signup_initiated", map[string]any{"ownership_model": detail.Connection.OwnershipModel, "expires_at": attempt.ExpiresAt})
	}); err != nil {
		return EmbeddedSignupInitiation{}, err
	}
	return EmbeddedSignupInitiation{AppID: s.embeddedConfig.AppID, ConfigurationID: s.embeddedConfig.ConfigurationID, GraphAPIVersion: s.embeddedConfig.GraphAPIVersion, AttemptToken: token, ExpiresAt: attempt.ExpiresAt}, nil
}

func (s *Service) CompleteEmbeddedSignup(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input EmbeddedSignupCompletionInput) (EmbeddedSignupCompletion, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return EmbeddedSignupCompletion{}, httperror.Forbidden("You do not have permission to connect WhatsApp")
	}
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return EmbeddedSignupCompletion{}, err
	}
	if detail.Connection.Provider != Provider || detail.Connection.Status == channelplatform.StatusArchived {
		return EmbeddedSignupCompletion{}, httperror.BadRequest("An active WhatsApp setup record is required")
	}
	if !s.embeddedSignupAvailable() {
		return EmbeddedSignupCompletion{}, httperror.BadRequest("Connect with Meta is not configured for this Zidi environment")
	}
	input.AttemptToken = strings.TrimSpace(input.AttemptToken)
	input.AuthorizationCode = strings.TrimSpace(input.AuthorizationCode)
	input.WhatsAppBusinessAccountID = strings.TrimSpace(input.WhatsAppBusinessAccountID)
	input.PhoneNumberID = strings.TrimSpace(input.PhoneNumberID)
	if len(input.AttemptToken) < 32 || input.AuthorizationCode == "" || len(input.AuthorizationCode) > 4096 || !validMetaID(input.WhatsAppBusinessAccountID) || !validMetaID(input.PhoneNumberID) {
		return EmbeddedSignupCompletion{}, httperror.BadRequest("Meta Embedded Signup did not return complete authorization information")
	}
	var attempt MetaSignupAttempt
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND token_hash = ?", actor.OrganizationID, connectionID, signupTokenHash(input.AttemptToken)).Order("created_at DESC").First(&attempt).Error; err != nil {
		return EmbeddedSignupCompletion{}, mapNotFound(err, "Meta signup attempt was not found or does not belong to this connection")
	}
	if attempt.Status == SignupAttemptCompleted {
		return s.embeddedCompletionView(ctx, actor, connectionID)
	}
	if !attempt.ExpiresAt.After(s.now()) {
		_ = s.db.WithContext(ctx).Model(&attempt).Updates(map[string]any{"status": SignupAttemptExpired, "failure_code": "attempt_expired", "updated_at": s.now()}).Error
		return EmbeddedSignupCompletion{}, httperror.BadRequest("Meta signup attempt expired; start Connect with Meta again")
	}
	if attempt.Status == SignupAttemptProcessing {
		return EmbeddedSignupCompletion{}, httperror.Conflict("Meta signup completion is already being processed")
	}
	processing := s.db.WithContext(ctx).Model(&MetaSignupAttempt{}).Where("id = ? AND organization_id = ? AND channel_connection_id = ? AND status IN ?", attempt.ID, actor.OrganizationID, connectionID, []string{SignupAttemptInitiated, SignupAttemptFailed}).Updates(map[string]any{"status": SignupAttemptProcessing, "failure_code": "", "updated_at": s.now()})
	if processing.Error != nil {
		return EmbeddedSignupCompletion{}, processing.Error
	}
	if processing.RowsAffected != 1 {
		return EmbeddedSignupCompletion{}, httperror.Conflict("Meta signup completion could not be claimed; refresh and try again")
	}
	fail := func(code string, cause error) (EmbeddedSignupCompletion, error) {
		_ = s.db.WithContext(ctx).Model(&MetaSignupAttempt{}).Where("id = ? AND organization_id = ? AND channel_connection_id = ?", attempt.ID, actor.OrganizationID, connectionID).Updates(map[string]any{"status": SignupAttemptFailed, "failure_code": code, "updated_at": s.now()}).Error
		_ = s.db.WithContext(ctx).Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"authorization_status": AuthorizationFailed, "last_authorization_error": code, "updated_at": s.now()}).Error
		_ = audit(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_embedded_signup_failed", map[string]any{"failure_code": code})
		return EmbeddedSignupCompletion{}, cause
	}

	var duplicate Configuration
	duplicateErr := s.db.WithContext(ctx).Where("channel_connection_id <> ? AND (phone_number_id = ? OR (whatsapp_business_account_id = ? AND authorization_status = ?))", connectionID, input.PhoneNumberID, input.WhatsAppBusinessAccountID, AuthorizationAuthorized).First(&duplicate).Error
	if duplicateErr == nil {
		return fail("asset_already_connected", httperror.Conflict("This WhatsApp account or phone number is already connected to another Zidi organization"))
	}
	if !errors.Is(duplicateErr, gorm.ErrRecordNotFound) {
		return fail("duplicate_check_failed", duplicateErr)
	}
	grant, err := s.embeddedClient.ExchangeCode(ctx, input.AuthorizationCode)
	if err != nil {
		return fail("authorization_exchange_failed", httperror.BadRequest("Meta authorization could not be completed; reconnect and try again"))
	}
	assets, err := s.embeddedClient.ValidateAssets(ctx, grant.AccessToken, input.WhatsAppBusinessAccountID, input.PhoneNumberID)
	if err != nil {
		return fail("asset_validation_failed", httperror.BadRequest("Meta could not validate the selected WhatsApp business and phone number"))
	}
	if err := s.embeddedClient.SubscribeApp(ctx, grant.AccessToken, input.WhatsAppBusinessAccountID); err != nil {
		return fail("webhook_subscription_failed", httperror.BadRequest("Meta authorization succeeded, but Zidi could not subscribe to WhatsApp events; reconnect and try again"))
	}
	accessReference, err := s.encrypted.Save(ctx, actor.OrganizationID, connectionID, CredentialAccessToken, grant.AccessToken)
	if err != nil {
		return fail("credential_storage_failed", errors.New("WhatsApp authorization could not be stored securely"))
	}
	now := s.now()
	validatedAt := now
	credentialInputs := []channelplatform.CredentialReferenceInput{
		{CredentialType: CredentialAccessToken, SecretRef: accessReference, Status: channelplatform.CredentialPresent, ExpiresAt: grant.ExpiresAt, LastValidatedAt: &validatedAt},
		{CredentialType: CredentialAppSecret, SecretRef: "env://META_APP_SECRET", Status: channelplatform.CredentialPresent, LastValidatedAt: &validatedAt},
		{CredentialType: CredentialVerifyToken, SecretRef: "env://META_WEBHOOK_VERIFY_TOKEN", Status: channelplatform.CredentialPresent, LastValidatedAt: &validatedAt},
	}
	for _, credential := range credentialInputs {
		if _, err := s.platform.UpsertCredentialReference(ctx, actor, connectionID, credential); err != nil {
			return fail("credential_reference_failed", errors.New("WhatsApp authorization could not be stored securely"))
		}
	}
	configuration, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return fail("configuration_load_failed", err)
	}
	if configuration.ID == uuid.Nil {
		configuration.ID = uuid.New()
		configuration.CreatedAt = now
	}
	configuration.PhoneNumberID = input.PhoneNumberID
	configuration.WhatsAppBusinessAccountID = input.WhatsAppBusinessAccountID
	configuration.DisplayPhoneNumber = assets.DisplayPhoneNumber
	configuration.GraphAPIVersion = s.embeddedConfig.GraphAPIVersion
	configuration.ConnectionMethod = ConnectionMethodEmbeddedSignup
	configuration.AuthorizationStatus = AuthorizationAuthorized
	configuration.AuthorizedAt = &now
	configuration.AuthorizationExpiresAt = grant.ExpiresAt
	configuration.LastAuthorizationError = ""
	configuration.WebhookStatus = WebhookVerified
	configuration.LastWebhookVerifiedAt = &now
	configuration.LastWebhookAttemptAt = &now
	configuration.SetupState = SetupTestMessageReady
	configuration.UpdatedAt = now
	if configuration.AssistedSetupStatus == AssistedAwaitingMerchantAction || configuration.AssistedSetupStatus == AssistedInProgress || configuration.AssistedSetupStatus == AssistedRequested {
		configuration.AssistedSetupStatus = AssistedConnected
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"phone_number_id", "whatsapp_business_account_id", "display_phone_number", "graph_api_version", "connection_method", "authorization_status", "authorized_at", "authorization_expires_at", "last_authorization_error", "assisted_setup_status", "webhook_status", "last_webhook_verified_at", "last_webhook_verification_attempt_at", "setup_state", "updated_at"}),
		}).Create(&configuration).Error; err != nil {
			return err
		}
		if err := syncProviderRecords(tx, configuration, detail.Connection.OwnershipModel, now); err != nil {
			return err
		}
		if err := tx.Model(&channelplatform.ProviderAccount{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"business_name": assets.BusinessName, "display_name": firstNonEmpty(assets.BusinessName, "WhatsApp Business Account"), "status": channelplatform.StatusConnected, "updated_at": now}).Error; err != nil {
			return err
		}
		if _, err := promoteReadyRecords(tx, configuration, now); err != nil {
			return err
		}
		if err := tx.Model(&MetaSignupAttempt{}).Where("id = ? AND organization_id = ? AND channel_connection_id = ? AND status = ?", attempt.ID, actor.OrganizationID, connectionID, SignupAttemptProcessing).Updates(map[string]any{"status": SignupAttemptCompleted, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_embedded_signup_completed", map[string]any{"authorization_status": AuthorizationAuthorized, "phone_identity_validated": true, "waba_validated": true, "webhook_subscribed": true})
	}); err != nil {
		return fail("connection_persistence_failed", mapEmbeddedPersistenceError(err))
	}
	return s.embeddedCompletionView(ctx, actor, connectionID)
}

func (s *Service) UpdateAssistedSetup(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input AssistedSetupInput) (ConfigurationView, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return ConfigurationView{}, httperror.Forbidden("You do not have permission to manage WhatsApp setup")
	}
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	if detail.Connection.Provider != Provider || detail.Connection.Status == channelplatform.StatusArchived {
		return ConfigurationView{}, httperror.BadRequest("An active WhatsApp setup record is required")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = AssistedRequested
	}
	if !validAssistedStatus(status) {
		return ConfigurationView{}, httperror.BadRequest("Assisted setup status is not valid")
	}
	if actor.Role != authz.PlatformAdmin && status != AssistedRequested && status != AssistedNotRequested {
		return ConfigurationView{}, httperror.Forbidden("Only a Zidi operator can advance assisted setup")
	}
	note := strings.TrimSpace(input.Note)
	if len(note) > 1000 {
		return ConfigurationView{}, httperror.BadRequest("Assisted setup note must be 1000 characters or fewer")
	}
	configuration, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	current := configuration.AssistedSetupStatus
	if current == "" {
		current = AssistedNotRequested
	}
	if !canTransitionAssisted(current, status) {
		return ConfigurationView{}, httperror.BadRequest("Assisted setup cannot move from " + current + " to " + status)
	}
	if (status == AssistedConnected || status == AssistedCompleted) && configuration.AuthorizationStatus != AuthorizationAuthorized {
		return ConfigurationView{}, httperror.BadRequest("Meta authorization must be completed before assisted setup can be marked connected")
	}
	if status == AssistedRequested && detail.Connection.OwnershipModel != channelplatform.OwnershipZidiManaged {
		ownership := channelplatform.OwnershipZidiManaged
		if _, err := s.platform.UpdateConnection(ctx, actor, connectionID, channelplatform.UpdateConnectionInput{OwnershipModel: &ownership}); err != nil {
			return ConfigurationView{}, err
		}
	}
	now := s.now()
	if configuration.ID == uuid.Nil {
		configuration.ID = uuid.New()
		configuration.CreatedAt = now
	}
	configuration.AssistedSetupStatus = status
	configuration.AssistedSetupNote = note
	if configuration.AuthorizationStatus == "" {
		configuration.AuthorizationStatus = AuthorizationNotStarted
	}
	if configuration.ConnectionMethod == "" || configuration.ConnectionMethod == ConnectionMethodManual {
		configuration.ConnectionMethod = ConnectionMethodAssisted
	}
	if configuration.WebhookStatus == "" {
		configuration.WebhookStatus = WebhookNotConfigured
	}
	if configuration.SetupState == "" || configuration.SetupState == SetupNotConnected {
		configuration.SetupState = SetupStarted
	}
	if configuration.GraphAPIVersion == "" {
		configuration.GraphAPIVersion = "v20.0"
	}
	configuration.UpdatedAt = now
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"connection_method", "authorization_status", "assisted_setup_status", "assisted_setup_note", "setup_state", "updated_at"}),
		}).Create(&configuration).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_assisted_setup_updated", map[string]any{"from": current, "to": status, "merchant_action_required": status == AssistedAwaitingMerchantAction})
	}); err != nil {
		return ConfigurationView{}, err
	}
	return s.GetConfiguration(ctx, actor, connectionID)
}

func (s *Service) VerifyPlatformWebhookToken(supplied string) bool {
	return constantTimeEqual(s.embeddedConfig.WebhookVerifyToken, supplied)
}

func (s *Service) configurationWebhookCallbackURL(configuration Configuration) string {
	if configuration.ConnectionMethod == ConnectionMethodEmbeddedSignup {
		return s.publicURL + webhookPath
	}
	return s.WebhookCallbackURL(configuration.ConnectionID)
}

func (s *Service) embeddedCompletionView(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (EmbeddedSignupCompletion, error) {
	view, err := s.GetConfiguration(ctx, actor, connectionID)
	if err != nil {
		return EmbeddedSignupCompletion{}, err
	}
	var account channelplatform.ProviderAccount
	_ = s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).First(&account).Error
	return EmbeddedSignupCompletion{
		ConnectionID: connectionID, ConnectionStatus: view.Connection.Status, AuthorizationStatus: view.AuthorizationStatus,
		BusinessName: account.BusinessName, DisplayPhoneNumber: view.DisplayPhoneNumber,
		OwnershipModel: view.Connection.OwnershipModel, AuthorizedAt: view.AuthorizedAt, NextAction: view.NextAction,
	}, nil
}

func randomSignupToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func signupTokenHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func validMetaID(value string) bool {
	return regexp.MustCompile(`^[0-9]{5,40}$`).MatchString(strings.TrimSpace(value))
}

func validGraphVersion(value string) bool {
	return regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]{1,3}$`).MatchString(strings.TrimSpace(value))
}

func validAssistedStatus(value string) bool {
	switch value {
	case AssistedNotRequested, AssistedRequested, AssistedInProgress, AssistedAwaitingMerchantAction, AssistedConnected, AssistedBlocked, AssistedCompleted:
		return true
	default:
		return false
	}
}

func canTransitionAssisted(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string][]string{
		AssistedNotRequested:           {AssistedRequested},
		AssistedRequested:              {AssistedNotRequested, AssistedInProgress, AssistedAwaitingMerchantAction, AssistedBlocked},
		AssistedInProgress:             {AssistedAwaitingMerchantAction, AssistedConnected, AssistedBlocked},
		AssistedAwaitingMerchantAction: {AssistedInProgress, AssistedConnected, AssistedBlocked},
		AssistedBlocked:                {AssistedInProgress, AssistedAwaitingMerchantAction},
		AssistedConnected:              {AssistedCompleted, AssistedBlocked},
		AssistedCompleted:              {},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func mapEmbeddedPersistenceError(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate") {
		return httperror.Conflict("This WhatsApp account or phone number is already connected")
	}
	return err
}
