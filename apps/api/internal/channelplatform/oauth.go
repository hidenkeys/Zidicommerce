package channelplatform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OAuthSessionInitiated       = "initiated"
	OAuthSessionProcessing      = "processing"
	OAuthSessionAwaitingAsset   = "awaiting_asset_selection"
	OAuthSessionCompleted       = "completed"
	OAuthSessionCancelled       = "cancelled"
	OAuthSessionFailed          = "failed"
	OAuthSessionExpired         = "expired"
	OAuthCredentialAccessToken  = "oauth_access_token"
	OAuthCredentialRefreshToken = "oauth_refresh_token"
	oauthSessionTTL             = 10 * time.Minute
)

type OAuthSession struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	ConnectionID      uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index" json:"channel_connection_id"`
	RequestedByUserID *uuid.UUID `gorm:"type:uuid;index" json:"requested_by_user_id,omitempty"`
	Provider          string     `json:"provider"`
	StateHash         string     `gorm:"uniqueIndex" json:"-"`
	PKCEVerifierRef   string     `json:"-"`
	RedirectURI       string     `json:"redirect_uri"`
	RequestedScopes   string     `json:"-"`
	GrantedScopes     string     `json:"-"`
	DeniedScopes      string     `json:"-"`
	ReviewApproved    bool       `json:"review_approved"`
	Status            string     `json:"status"`
	FailureCode       string     `json:"failure_code,omitempty"`
	ExpiresAt         time.Time  `json:"expires_at"`
	CallbackClaimedAt *time.Time `json:"callback_claimed_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (OAuthSession) TableName() string { return "channel_oauth_sessions" }

type OAuthAssetRecord struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_channel_oauth_asset" json:"organization_id"`
	ConnectionID     uuid.UUID  `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_channel_oauth_asset" json:"channel_connection_id"`
	OAuthSessionID   uuid.UUID  `gorm:"column:oauth_session_id;type:uuid;index" json:"oauth_session_id"`
	Provider         string     `json:"provider"`
	ProviderAssetID  string     `gorm:"uniqueIndex:idx_channel_oauth_asset" json:"provider_asset_id"`
	AssetType        string     `json:"asset_type"`
	DisplayName      string     `json:"display_name"`
	ExternalHandle   string     `json:"external_handle,omitempty"`
	Eligible         bool       `json:"eligible"`
	IneligibleReason string     `json:"ineligible_reason,omitempty"`
	Selected         bool       `json:"selected"`
	Metadata         string     `json:"-"`
	SelectedAt       *time.Time `json:"selected_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (OAuthAssetRecord) TableName() string { return "channel_oauth_assets" }

type OAuthAuthorizationRequest struct {
	State         string
	CodeChallenge string
	RedirectURI   string
	Scopes        []string
}

type OAuthCodeExchangeRequest struct {
	Code         string
	CodeVerifier string
	RedirectURI  string
}

type OAuthRefreshRequest struct {
	AccessToken  string
	RefreshToken string
	Asset        *OAuthAsset
}

type OAuthRevokeRequest struct {
	AccessToken  string
	RefreshToken string
	Asset        *OAuthAsset
}

type OAuthTokenGrant struct {
	AccessToken      string
	RefreshToken     string
	GrantedScopes    []string
	AccessExpiresAt  *time.Time
	RefreshExpiresAt *time.Time
	ReviewApproved   bool
}

type OAuthAsset struct {
	ProviderAssetID  string
	AssetType        string
	DisplayName      string
	ExternalHandle   string
	Eligible         bool
	IneligibleReason string
	Metadata         map[string]any
}

type OAuthProviderClient interface {
	Provider() string
	RequestedScopes() []string
	AuthorizationURL(OAuthAuthorizationRequest) (string, error)
	ExchangeCode(context.Context, OAuthCodeExchangeRequest) (OAuthTokenGrant, error)
	DiscoverAssets(context.Context, string) ([]OAuthAsset, error)
	ConnectAsset(context.Context, string, OAuthAsset) (OAuthTokenGrant, error)
	Refresh(context.Context, OAuthRefreshRequest) (OAuthTokenGrant, error)
	Revoke(context.Context, OAuthRevokeRequest) error
}

type OAuthService struct {
	db        *gorm.DB
	platform  *Service
	encrypted *EncryptedSecretStore
	resolver  SecretResolver
	clients   map[string]OAuthProviderClient
	now       func() time.Time
}

func NewOAuthService(db *gorm.DB, platform *Service, encrypted *EncryptedSecretStore, resolver SecretResolver) *OAuthService {
	return &OAuthService{
		db:        db,
		platform:  platform,
		encrypted: encrypted,
		resolver:  resolver,
		clients:   map[string]OAuthProviderClient{},
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *OAuthService) RegisterClient(client OAuthProviderClient) {
	if client != nil && normalizeKey(client.Provider()) != "" {
		s.clients[normalizeKey(client.Provider())] = client
	}
}

type OAuthStartInput struct {
	RedirectURI string `json:"redirect_uri"`
}

type OAuthStartView struct {
	SessionID        uuid.UUID `json:"session_id"`
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type OAuthCallbackInput struct {
	State string `json:"state"`
	Code  string `json:"code"`
}

type OAuthSessionView struct {
	ID              uuid.UUID `json:"id"`
	ConnectionID    uuid.UUID `json:"channel_connection_id"`
	Provider        string    `json:"provider"`
	Status          string    `json:"status"`
	RequestedScopes []string  `json:"requested_scopes"`
	GrantedScopes   []string  `json:"granted_scopes"`
	DeniedScopes    []string  `json:"denied_scopes"`
	FailureCode     string    `json:"failure_code,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type OAuthAssetView struct {
	ID               uuid.UUID      `json:"id"`
	SessionID        uuid.UUID      `json:"oauth_session_id"`
	ProviderAssetID  string         `json:"provider_asset_id"`
	AssetType        string         `json:"asset_type"`
	DisplayName      string         `json:"display_name"`
	ExternalHandle   string         `json:"external_handle,omitempty"`
	Eligible         bool           `json:"eligible"`
	IneligibleReason string         `json:"ineligible_reason,omitempty"`
	Selected         bool           `json:"selected"`
	Metadata         map[string]any `json:"metadata"`
}

type OAuthCallbackView struct {
	Session OAuthSessionView `json:"session"`
	Assets  []OAuthAssetView `json:"assets"`
}

type OAuthAssetSelectionInput struct {
	SessionID uuid.UUID `json:"session_id"`
}

type OAuthCancelInput struct {
	SessionID uuid.UUID `json:"session_id"`
}

type OAuthRefreshView struct {
	ConnectionID uuid.UUID  `json:"channel_connection_id"`
	Status       string     `json:"status"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func (s *OAuthService) Start(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input OAuthStartInput) (OAuthStartView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return OAuthStartView{}, err
	}
	connection, err := s.platform.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return OAuthStartView{}, err
	}
	client := s.clients[connection.Provider]
	if client == nil || s.encrypted == nil || s.resolver == nil {
		return OAuthStartView{}, httperror.BadRequest("OAuth onboarding is not configured for this provider")
	}
	redirectURI, err := validOAuthRedirect(input.RedirectURI)
	if err != nil {
		return OAuthStartView{}, err
	}
	state, err := randomOAuthValue(32)
	if err != nil {
		return OAuthStartView{}, err
	}
	verifier, err := randomOAuthValue(64)
	if err != nil {
		return OAuthStartView{}, err
	}
	now := s.now()
	session := OAuthSession{
		ID:                uuid.New(),
		OrganizationID:    actor.OrganizationID,
		ConnectionID:      connectionID,
		RequestedByUserID: &actor.ID,
		Provider:          connection.Provider,
		StateHash:         oauthHash(state),
		RedirectURI:       redirectURI,
		RequestedScopes:   jsonValue(normalizeScopeList(client.RequestedScopes())),
		GrantedScopes:     "[]",
		DeniedScopes:      "[]",
		Status:            OAuthSessionInitiated,
		ExpiresAt:         now.Add(oauthSessionTTL),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	credentialType := oauthPKCECredentialType(session.ID)
	verifierRef, err := s.encrypted.Save(ctx, actor.OrganizationID, connectionID, credentialType, verifier)
	if err != nil {
		return OAuthStartView{}, errors.New("OAuth session could not be secured")
	}
	session.PKCEVerifierRef = verifierRef
	authorizationURL, err := client.AuthorizationURL(OAuthAuthorizationRequest{
		State:         state,
		CodeChallenge: pkceChallenge(verifier),
		RedirectURI:   redirectURI,
		Scopes:        stringList(session.RequestedScopes),
	})
	if err != nil || !safeAuthorizationURL(authorizationURL) {
		_ = s.encrypted.Delete(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: credentialType, Reference: verifierRef})
		return OAuthStartView{}, httperror.BadRequest("Provider authorization could not be started")
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if connection.Status == StatusSetupRequired || connection.Status == StatusDisconnected {
			if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusConnecting, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_oauth_started", map[string]any{"provider": connection.Provider, "session_id": session.ID.String(), "expires_at": session.ExpiresAt})
	}); err != nil {
		_ = s.encrypted.Delete(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: credentialType, Reference: verifierRef})
		return OAuthStartView{}, err
	}
	return OAuthStartView{SessionID: session.ID, AuthorizationURL: authorizationURL, ExpiresAt: session.ExpiresAt}, nil
}

func (s *OAuthService) CompleteCallback(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input OAuthCallbackInput) (OAuthCallbackView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return OAuthCallbackView{}, err
	}
	state, code := strings.TrimSpace(input.State), strings.TrimSpace(input.Code)
	if len(state) < 32 || code == "" || len(code) > 4096 {
		return OAuthCallbackView{}, httperror.BadRequest("Provider authorization response is incomplete")
	}
	var session OAuthSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND requested_by_user_id = ? AND state_hash = ?", actor.OrganizationID, connectionID, actor.ID, oauthHash(state)).First(&session).Error; err != nil {
		return OAuthCallbackView{}, oauthNotFound(err, "OAuth session was not found")
	}
	if !session.ExpiresAt.After(s.now()) {
		_ = s.failSession(ctx, session, OAuthSessionExpired, "session_expired")
		if session.PKCEVerifierRef != "" && s.encrypted != nil {
			_ = s.encrypted.Delete(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: oauthPKCECredentialType(session.ID), Reference: session.PKCEVerifierRef})
		}
		return OAuthCallbackView{}, httperror.BadRequest("OAuth session expired; start again")
	}
	if session.Status != OAuthSessionInitiated {
		return OAuthCallbackView{}, httperror.Conflict("OAuth callback was already used")
	}
	client := s.clients[session.Provider]
	if client == nil {
		return OAuthCallbackView{}, httperror.BadRequest("OAuth onboarding is not configured for this provider")
	}
	claimedAt := s.now()
	claim := s.db.WithContext(ctx).Model(&OAuthSession{}).Where("id = ? AND organization_id = ? AND status = ?", session.ID, actor.OrganizationID, OAuthSessionInitiated).Updates(map[string]any{"status": OAuthSessionProcessing, "callback_claimed_at": claimedAt, "updated_at": claimedAt})
	if claim.Error != nil {
		return OAuthCallbackView{}, claim.Error
	}
	if claim.RowsAffected != 1 {
		return OAuthCallbackView{}, httperror.Conflict("OAuth callback was already used")
	}
	fail := func(code string, cause error) (OAuthCallbackView, error) {
		_ = s.failSession(ctx, session, OAuthSessionFailed, code)
		return OAuthCallbackView{}, cause
	}
	credentialType := oauthPKCECredentialType(session.ID)
	verifier, err := s.resolver.Resolve(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: credentialType, Reference: session.PKCEVerifierRef})
	if err != nil {
		return fail("pkce_verifier_unavailable", errors.New("OAuth session could not be verified"))
	}
	grant, err := client.ExchangeCode(ctx, OAuthCodeExchangeRequest{Code: code, CodeVerifier: verifier, RedirectURI: session.RedirectURI})
	_ = s.encrypted.Delete(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: credentialType, Reference: session.PKCEVerifierRef})
	if err != nil || strings.TrimSpace(grant.AccessToken) == "" {
		return fail("code_exchange_failed", httperror.BadRequest("Provider authorization could not be completed"))
	}
	assets, err := client.DiscoverAssets(ctx, grant.AccessToken)
	if err != nil {
		return fail("asset_discovery_failed", httperror.BadRequest("Authorized provider assets could not be loaded"))
	}
	if len(assets) == 0 {
		return fail("no_provider_assets", httperror.BadRequest("No provider assets are available for this account"))
	}
	now := s.now()
	connection, err := s.platform.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return fail("connection_unavailable", err)
	}
	if err := s.storeOAuthGrant(ctx, actor, connection, grant, now); err != nil {
		return fail("token_storage_failed", errors.New("Provider authorization could not be stored securely"))
	}
	granted := normalizeScopeList(grant.GrantedScopes)
	requested := stringList(session.RequestedScopes)
	denied := missingValues(requested, normalizedSet(granted))
	assetRecords, err := s.persistOAuthAssets(ctx, session, assets)
	if err != nil {
		return fail("asset_storage_failed", err)
	}
	if err := s.platform.ResolveConnectionCapabilities(ctx, actor.OrganizationID, connectionID, CapabilityResolution{GrantedScopes: granted, ReviewApproved: grant.ReviewApproved, AssetReady: false, VerifiedAt: now}); err != nil {
		return fail("capability_resolution_failed", err)
	}
	if err := s.db.WithContext(ctx).Model(&OAuthSession{}).Where("id = ? AND organization_id = ? AND status = ?", session.ID, actor.OrganizationID, OAuthSessionProcessing).Updates(map[string]any{
		"status":            OAuthSessionAwaitingAsset,
		"granted_scopes":    jsonValue(granted),
		"denied_scopes":     jsonValue(denied),
		"review_approved":   grant.ReviewApproved,
		"pkce_verifier_ref": "",
		"updated_at":        now,
	}).Error; err != nil {
		return fail("session_persistence_failed", err)
	}
	_ = auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_oauth_callback_completed", map[string]any{"provider": session.Provider, "session_id": session.ID.String(), "granted_scope_count": len(granted), "denied_scope_count": len(denied), "asset_count": len(assets)})
	session.Status = OAuthSessionAwaitingAsset
	session.GrantedScopes = jsonValue(granted)
	session.DeniedScopes = jsonValue(denied)
	session.ReviewApproved = grant.ReviewApproved
	return OAuthCallbackView{Session: oauthSessionView(session), Assets: oauthAssetViews(assetRecords)}, nil
}

func (s *OAuthService) ListAssets(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) ([]OAuthAssetView, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return nil, err
	}
	if _, err := s.platform.findConnection(ctx, actor.OrganizationID, connectionID); err != nil {
		return nil, err
	}
	var rows []OAuthAssetRecord
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Order("created_at DESC, display_name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return oauthAssetViews(rows), nil
}

func (s *OAuthService) SelectAsset(ctx context.Context, actor auth.CurrentUser, connectionID, assetID uuid.UUID, input OAuthAssetSelectionInput) (ConnectionDetail, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return ConnectionDetail{}, err
	}
	if input.SessionID == uuid.Nil {
		return ConnectionDetail{}, httperror.BadRequest("OAuth session ID is required")
	}
	connection, err := s.platform.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionDetail{}, err
	}
	var session OAuthSession
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND channel_connection_id = ? AND requested_by_user_id = ? AND status = ?", input.SessionID, actor.OrganizationID, connectionID, actor.ID, OAuthSessionAwaitingAsset).First(&session).Error; err != nil {
		return ConnectionDetail{}, oauthNotFound(err, "OAuth asset-selection session was not found")
	}
	var asset OAuthAssetRecord
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND channel_connection_id = ? AND oauth_session_id = ?", assetID, actor.OrganizationID, connectionID, session.ID).First(&asset).Error; err != nil {
		return ConnectionDetail{}, oauthNotFound(err, "Provider asset was not found")
	}
	if !asset.Eligible {
		return ConnectionDetail{}, httperror.BadRequest("This provider asset is not eligible: " + asset.IneligibleReason)
	}
	accessToken, err := s.resolveOAuthCredential(ctx, actor.OrganizationID, connectionID, OAuthCredentialAccessToken)
	if err != nil {
		return ConnectionDetail{}, httperror.BadRequest("Provider authorization requires reconnection")
	}
	client := s.clients[connection.Provider]
	if client == nil {
		return ConnectionDetail{}, httperror.BadRequest("OAuth onboarding is not configured for this provider")
	}
	providerAsset := OAuthAsset{ProviderAssetID: asset.ProviderAssetID, AssetType: asset.AssetType, DisplayName: asset.DisplayName, ExternalHandle: asset.ExternalHandle, Eligible: asset.Eligible, IneligibleReason: asset.IneligibleReason}
	assetGrant, err := client.ConnectAsset(ctx, accessToken, providerAsset)
	if err != nil {
		return ConnectionDetail{}, httperror.BadRequest("Provider asset could not be connected")
	}
	now := s.now()
	if strings.TrimSpace(assetGrant.AccessToken) != "" {
		if len(assetGrant.GrantedScopes) == 0 {
			assetGrant.GrantedScopes = stringList(session.GrantedScopes)
		}
		assetGrant.ReviewApproved = session.ReviewApproved
		if err := s.storeOAuthGrant(ctx, actor, connection, assetGrant, now); err != nil {
			return ConnectionDetail{}, errors.New("Provider asset authorization could not be stored securely")
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&OAuthAssetRecord{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"selected": false, "selected_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&OAuthAssetRecord{}).Where("id = ? AND organization_id = ?", asset.ID, actor.OrganizationID).Updates(map[string]any{"selected": true, "selected_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		account := ProviderAccount{ID: uuid.New(), OrganizationID: actor.OrganizationID, ConnectionID: connectionID, Provider: connection.Provider, ProviderAccountID: asset.ProviderAssetID, BusinessName: asset.DisplayName, DisplayName: asset.DisplayName, OwnershipModel: connection.OwnershipModel, Status: StatusConnected, Metadata: "{}", CreatedAt: now, UpdatedAt: now}
		if err := upsertOAuthProviderAccount(tx, account); err != nil {
			return err
		}
		identity := ChannelIdentity{ID: uuid.New(), OrganizationID: actor.OrganizationID, ConnectionID: connectionID, Provider: connection.Provider, IdentityType: asset.AssetType, DisplayName: asset.DisplayName, ProviderIdentityID: asset.ProviderAssetID, ExternalHandle: asset.ExternalHandle, Status: StatusConnected, Metadata: "{}", CreatedAt: now, UpdatedAt: now}
		if err := upsertOAuthIdentity(tx, identity); err != nil {
			return err
		}
		if err := tx.Model(&OAuthSession{}).Where("id = ? AND organization_id = ? AND status = ?", session.ID, actor.OrganizationID, OAuthSessionAwaitingAsset).Updates(map[string]any{"status": OAuthSessionCompleted, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusConnected, "last_connected_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_oauth_asset_connected", map[string]any{"provider": connection.Provider, "asset_type": asset.AssetType, "session_id": session.ID.String()})
	}); err != nil {
		return ConnectionDetail{}, err
	}
	if err := s.platform.ResolveConnectionCapabilities(ctx, actor.OrganizationID, connectionID, CapabilityResolution{GrantedScopes: stringList(session.GrantedScopes), ReviewApproved: session.ReviewApproved, AssetReady: true, VerifiedAt: now}); err != nil {
		return ConnectionDetail{}, err
	}
	return s.platform.GetConnection(ctx, actor, connectionID)
}

func upsertOAuthProviderAccount(tx *gorm.DB, account ProviderAccount) error {
	var existing ProviderAccount
	err := tx.Where("organization_id = ? AND provider = ? AND provider_account_id = ?", account.OrganizationID, account.Provider, account.ProviderAccountID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&account).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&ProviderAccount{}).Where("id = ? AND organization_id = ?", existing.ID, account.OrganizationID).Updates(map[string]any{
		"channel_connection_id": account.ConnectionID,
		"business_name":         account.BusinessName,
		"display_name":          account.DisplayName,
		"ownership_model":       account.OwnershipModel,
		"status":                account.Status,
		"updated_at":            account.UpdatedAt,
	}).Error
}

func upsertOAuthIdentity(tx *gorm.DB, identity ChannelIdentity) error {
	var existing ChannelIdentity
	err := tx.Where("organization_id = ? AND provider = ? AND provider_identity_id = ?", identity.OrganizationID, identity.Provider, identity.ProviderIdentityID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&identity).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&ChannelIdentity{}).Where("id = ? AND organization_id = ?", existing.ID, identity.OrganizationID).Updates(map[string]any{
		"channel_connection_id": identity.ConnectionID,
		"identity_type":         identity.IdentityType,
		"display_name":          identity.DisplayName,
		"external_handle":       identity.ExternalHandle,
		"status":                identity.Status,
		"updated_at":            identity.UpdatedAt,
	}).Error
}

func (s *OAuthService) Cancel(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input OAuthCancelInput) (OAuthSessionView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return OAuthSessionView{}, err
	}
	var session OAuthSession
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND channel_connection_id = ? AND requested_by_user_id = ?", input.SessionID, actor.OrganizationID, connectionID, actor.ID).First(&session).Error; err != nil {
		return OAuthSessionView{}, oauthNotFound(err, "OAuth session was not found")
	}
	if session.Status == OAuthSessionCompleted || session.Status == OAuthSessionCancelled {
		return OAuthSessionView{}, httperror.Conflict("OAuth session can no longer be cancelled")
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&OAuthSession{}).Where("id = ? AND organization_id = ?", session.ID, actor.OrganizationID).Updates(map[string]any{"status": OAuthSessionCancelled, "pkce_verifier_ref": "", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ? AND status = ?", actor.OrganizationID, connectionID, StatusConnecting).Updates(map[string]any{"status": StatusSetupRequired, "updated_at": now}).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_oauth_cancelled", map[string]any{"provider": session.Provider, "session_id": session.ID.String()})
	}); err != nil {
		return OAuthSessionView{}, err
	}
	if session.PKCEVerifierRef != "" && s.encrypted != nil {
		_ = s.encrypted.Delete(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: oauthPKCECredentialType(session.ID), Reference: session.PKCEVerifierRef})
	}
	session.Status = OAuthSessionCancelled
	return oauthSessionView(session), nil
}

func (s *OAuthService) Refresh(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (OAuthRefreshView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return OAuthRefreshView{}, err
	}
	connection, err := s.platform.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return OAuthRefreshView{}, err
	}
	client := s.clients[connection.Provider]
	if client == nil || s.encrypted == nil || s.resolver == nil {
		return OAuthRefreshView{}, httperror.BadRequest("OAuth refresh is not configured for this provider")
	}
	accessToken, err := s.resolveOAuthCredential(ctx, actor.OrganizationID, connectionID, OAuthCredentialAccessToken)
	if err != nil {
		return OAuthRefreshView{}, httperror.BadRequest("Provider authorization requires reconnection")
	}
	refreshToken, err := s.resolveOAuthCredential(ctx, actor.OrganizationID, connectionID, OAuthCredentialRefreshToken)
	if err != nil {
		return OAuthRefreshView{}, httperror.BadRequest("Provider authorization requires reconnection")
	}
	asset, err := s.selectedOAuthAsset(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return OAuthRefreshView{}, httperror.BadRequest("Provider asset selection is incomplete")
	}
	grant, err := client.Refresh(ctx, OAuthRefreshRequest{AccessToken: accessToken, RefreshToken: refreshToken, Asset: &asset})
	if err != nil || strings.TrimSpace(grant.AccessToken) == "" {
		_ = s.markOAuthCredentials(ctx, actor, connectionID, CredentialRequiresReauthorization, "channel_oauth_refresh_failed")
		return OAuthRefreshView{}, httperror.BadRequest("Provider authorization could not be refreshed; reconnect the channel")
	}
	now := s.now()
	if err := s.storeOAuthGrant(ctx, actor, connection, grant, now); err != nil {
		return OAuthRefreshView{}, errors.New("Refreshed provider authorization could not be stored securely")
	}
	_ = auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_oauth_refreshed", map[string]any{"provider": connection.Provider})
	return OAuthRefreshView{ConnectionID: connectionID, Status: CredentialPresent, ExpiresAt: grant.AccessExpiresAt}, nil
}

func (s *OAuthService) Disconnect(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (ConnectionDetail, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return ConnectionDetail{}, err
	}
	connection, err := s.platform.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionDetail{}, err
	}
	client := s.clients[connection.Provider]
	if client == nil || s.encrypted == nil || s.resolver == nil {
		return ConnectionDetail{}, httperror.BadRequest("OAuth disconnect is not configured for this provider")
	}
	accessToken, _ := s.resolveOAuthCredential(ctx, actor.OrganizationID, connectionID, OAuthCredentialAccessToken)
	refreshToken, _ := s.resolveOAuthCredential(ctx, actor.OrganizationID, connectionID, OAuthCredentialRefreshToken)
	asset, _ := s.selectedOAuthAsset(ctx, actor.OrganizationID, connectionID)
	if accessToken != "" || refreshToken != "" {
		if err := client.Revoke(ctx, OAuthRevokeRequest{AccessToken: accessToken, RefreshToken: refreshToken, Asset: &asset}); err != nil {
			return ConnectionDetail{}, httperror.BadRequest("Provider authorization could not be revoked")
		}
	}
	var credentials []CredentialReference
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type IN ?", actor.OrganizationID, connectionID, []string{OAuthCredentialAccessToken, OAuthCredentialRefreshToken}).Find(&credentials).Error; err != nil {
		return ConnectionDetail{}, err
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CredentialReference{}).Where("organization_id = ? AND channel_connection_id = ? AND credential_type IN ?", actor.OrganizationID, connectionID, []string{OAuthCredentialAccessToken, OAuthCredentialRefreshToken}).Updates(map[string]any{"secret_ref": "", "status": CredentialRevoked, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ProviderAccount{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusDisconnected, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelIdentity{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusDisconnected, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusDisconnected, "updated_at": now}).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_oauth_disconnected", map[string]any{"provider": connection.Provider})
	}); err != nil {
		return ConnectionDetail{}, err
	}
	for _, credential := range credentials {
		_ = s.encrypted.Delete(ctx, SecretScope{OrganizationID: actor.OrganizationID, ConnectionID: connectionID, CredentialType: credential.CredentialType, Reference: credential.SecretRef})
	}
	if err := s.platform.ResolveConnectionCapabilities(ctx, actor.OrganizationID, connectionID, CapabilityResolution{VerifiedAt: now}); err != nil {
		return ConnectionDetail{}, err
	}
	return s.platform.GetConnection(ctx, actor, connectionID)
}

func (s *OAuthService) storeOAuthGrant(ctx context.Context, actor auth.CurrentUser, connection ChannelConnection, grant OAuthTokenGrant, validatedAt time.Time) error {
	accessRef, err := s.encrypted.Save(ctx, actor.OrganizationID, connection.ID, OAuthCredentialAccessToken, grant.AccessToken)
	if err != nil {
		return err
	}
	if _, err := s.platform.UpsertCredentialReference(ctx, actor, connection.ID, CredentialReferenceInput{CredentialType: OAuthCredentialAccessToken, SecretRef: accessRef, Status: CredentialPresent, ExpiresAt: grant.AccessExpiresAt, LastValidatedAt: &validatedAt}); err != nil {
		return err
	}
	if strings.TrimSpace(grant.RefreshToken) == "" {
		return nil
	}
	refreshRef, err := s.encrypted.Save(ctx, actor.OrganizationID, connection.ID, OAuthCredentialRefreshToken, grant.RefreshToken)
	if err != nil {
		return err
	}
	_, err = s.platform.UpsertCredentialReference(ctx, actor, connection.ID, CredentialReferenceInput{CredentialType: OAuthCredentialRefreshToken, SecretRef: refreshRef, Status: CredentialPresent, ExpiresAt: grant.RefreshExpiresAt, LastValidatedAt: &validatedAt})
	return err
}

func (s *OAuthService) markOAuthCredentials(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, status, action string) error {
	now := s.now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CredentialReference{}).Where("organization_id = ? AND channel_connection_id = ? AND credential_type IN ?", actor.OrganizationID, connectionID, []string{OAuthCredentialAccessToken, OAuthCredentialRefreshToken}).Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusRequiresAttention, "updated_at": now}).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, action, map[string]any{"credential_status": status})
	})
}

func (s *OAuthService) persistOAuthAssets(ctx context.Context, session OAuthSession, assets []OAuthAsset) ([]OAuthAssetRecord, error) {
	rows := assetsToRecords(session, assets, s.now())
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND channel_connection_id = ? AND oauth_session_id <> ?", session.OrganizationID, session.ConnectionID, session.ID).Delete(&OAuthAssetRecord{}).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "provider_asset_id"}}, DoUpdates: clause.AssignmentColumns([]string{"oauth_session_id", "asset_type", "display_name", "external_handle", "eligible", "ineligible_reason", "metadata", "selected", "selected_at", "updated_at"})}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var stored []OAuthAssetRecord
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND oauth_session_id = ?", session.OrganizationID, session.ConnectionID, session.ID).Order("display_name ASC").Find(&stored).Error; err != nil {
		return nil, err
	}
	return stored, nil
}

func (s *OAuthService) resolveOAuthCredential(ctx context.Context, organizationID, connectionID uuid.UUID, credentialType string) (string, error) {
	var credential CredentialReference
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ?", organizationID, connectionID, credentialType).First(&credential).Error; err != nil {
		return "", err
	}
	return s.resolver.Resolve(ctx, SecretScope{OrganizationID: organizationID, ConnectionID: connectionID, CredentialType: credentialType, Reference: credential.SecretRef})
}

func (s *OAuthService) selectedOAuthAsset(ctx context.Context, organizationID, connectionID uuid.UUID) (OAuthAsset, error) {
	var record OAuthAssetRecord
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND selected = ?", organizationID, connectionID, true).First(&record).Error; err != nil {
		return OAuthAsset{}, err
	}
	metadata := map[string]any{}
	_ = jsonUnmarshalObject(record.Metadata, &metadata)
	return OAuthAsset{ProviderAssetID: record.ProviderAssetID, AssetType: record.AssetType, DisplayName: record.DisplayName, ExternalHandle: record.ExternalHandle, Eligible: record.Eligible, IneligibleReason: record.IneligibleReason, Metadata: metadata}, nil
}

func (s *OAuthService) failSession(ctx context.Context, session OAuthSession, status, code string) error {
	return s.db.WithContext(ctx).Model(&OAuthSession{}).Where("id = ? AND organization_id = ?", session.ID, session.OrganizationID).Updates(map[string]any{"status": status, "failure_code": code, "updated_at": s.now()}).Error
}

func assetsToRecords(session OAuthSession, assets []OAuthAsset, now time.Time) []OAuthAssetRecord {
	rows := make([]OAuthAssetRecord, 0, len(assets))
	for _, asset := range assets {
		rows = append(rows, OAuthAssetRecord{
			ID:               uuid.New(),
			OrganizationID:   session.OrganizationID,
			ConnectionID:     session.ConnectionID,
			OAuthSessionID:   session.ID,
			Provider:         session.Provider,
			ProviderAssetID:  strings.TrimSpace(asset.ProviderAssetID),
			AssetType:        normalizeKey(asset.AssetType),
			DisplayName:      strings.TrimSpace(asset.DisplayName),
			ExternalHandle:   strings.TrimSpace(asset.ExternalHandle),
			Eligible:         asset.Eligible,
			IneligibleReason: strings.TrimSpace(asset.IneligibleReason),
			Metadata:         jsonValue(sanitizeOAuthMetadata(asset.Metadata)),
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	return rows
}

func oauthAssetViews(rows []OAuthAssetRecord) []OAuthAssetView {
	views := make([]OAuthAssetView, 0, len(rows))
	for _, row := range rows {
		metadata := map[string]any{}
		_ = jsonUnmarshalObject(row.Metadata, &metadata)
		views = append(views, OAuthAssetView{ID: row.ID, SessionID: row.OAuthSessionID, ProviderAssetID: row.ProviderAssetID, AssetType: row.AssetType, DisplayName: row.DisplayName, ExternalHandle: row.ExternalHandle, Eligible: row.Eligible, IneligibleReason: row.IneligibleReason, Selected: row.Selected, Metadata: metadata})
	}
	return views
}

func oauthSessionView(session OAuthSession) OAuthSessionView {
	return OAuthSessionView{ID: session.ID, ConnectionID: session.ConnectionID, Provider: session.Provider, Status: session.Status, RequestedScopes: stringList(session.RequestedScopes), GrantedScopes: stringList(session.GrantedScopes), DeniedScopes: stringList(session.DeniedScopes), FailureCode: session.FailureCode, ExpiresAt: session.ExpiresAt}
}

func sanitizeOAuthMetadata(metadata map[string]any) map[string]any {
	clean := make(map[string]any, len(metadata))
	for key, value := range metadata {
		normalized := normalizeKey(key)
		if strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "verifier") || normalized == "code" || normalized == "authorization" {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			clean[key] = sanitizeOAuthMetadata(nested)
			continue
		}
		clean[key] = value
	}
	return clean
}

func oauthPKCECredentialType(id uuid.UUID) string {
	return "oauth_pkce_" + strings.ReplaceAll(id.String(), "-", "")
}

func oauthHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func pkceChallenge(verifier string) string { return oauthHash(verifier) }

func randomOAuthValue(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func normalizeScopeList(values []string) []string {
	return sortedKeys(normalizedSet(values))
}

func jsonUnmarshalObject(value string, target *map[string]any) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return json.Unmarshal([]byte(value), target)
}

func validOAuthRedirect(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.Fragment != "" {
		return "", httperror.BadRequest("OAuth redirect URI must be an absolute URL without a fragment")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1")) {
		return "", httperror.BadRequest("OAuth redirect URI must use HTTPS")
	}
	return parsed.String(), nil
}

func safeAuthorizationURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func oauthNotFound(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httperror.NotFound(message)
	}
	return err
}
