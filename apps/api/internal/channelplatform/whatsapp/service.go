package whatsapp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db             *gorm.DB
	platform       *channelplatform.Service
	resolver       channelplatform.SecretResolver
	encrypted      *channelplatform.EncryptedSecretStore
	embeddedConfig EmbeddedSignupConfig
	embeddedClient EmbeddedSignupClient
	publicURL      string
	now            func() time.Time
}

var inboundReadyStatuses = []string{"active", channelplatform.StatusConnected, channelplatform.StatusHealthy, channelplatform.StatusDegraded, channelplatform.StatusRequiresAttention}

func NewService(db *gorm.DB, platform *channelplatform.Service, resolver channelplatform.SecretResolver, encrypted *channelplatform.EncryptedSecretStore) *Service {
	return &Service{db: db, platform: platform, resolver: resolver, encrypted: encrypted, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) GetConfiguration(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (ConfigurationView, error) {
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	if detail.Connection.Provider != Provider {
		return ConfigurationView{}, httperror.BadRequest("This connection is not a WhatsApp channel")
	}
	configuration, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	return s.configurationView(ctx, detail.Connection, configuration)
}

func (s *Service) UpdateConfiguration(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input ConfigurationInput) (ConfigurationView, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return ConfigurationView{}, httperror.Forbidden("You do not have permission to configure WhatsApp")
	}
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	if detail.Connection.Provider != Provider || detail.Connection.Status == channelplatform.StatusArchived {
		return ConfigurationView{}, httperror.BadRequest("An active WhatsApp setup record is required")
	}
	configuration, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConfigurationView{}, err
	}
	if err := applyConfigurationInput(&configuration, input); err != nil {
		return ConfigurationView{}, err
	}
	if err := s.saveCredentialInput(ctx, actor, connectionID, CredentialAccessToken, input.AccessTokenReference, input.AccessTokenValue); err != nil {
		return ConfigurationView{}, err
	}
	if err := s.saveCredentialInput(ctx, actor, connectionID, CredentialAppSecret, input.AppSecretReference, input.AppSecretValue); err != nil {
		return ConfigurationView{}, err
	}
	if err := s.saveCredentialInput(ctx, actor, connectionID, CredentialVerifyToken, input.VerifyTokenReference, input.VerifyTokenValue); err != nil {
		return ConfigurationView{}, err
	}
	now := s.now()
	configuration.UpdatedAt = now
	if configuration.ID == uuid.Nil {
		configuration.ID = uuid.New()
		configuration.OrganizationID = actor.OrganizationID
		configuration.ConnectionID = connectionID
		configuration.CreatedAt = now
	}
	if configuration.GraphAPIVersion == "" {
		configuration.GraphAPIVersion = "v20.0"
	}
	if configuration.WebhookStatus == "" {
		configuration.WebhookStatus = WebhookNotConfigured
	}
	if configuration.SetupState == "" || configuration.SetupState == SetupNotConnected {
		configuration.SetupState = SetupStarted
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"phone_number_id", "whatsapp_business_account_id", "meta_business_account_id", "display_phone_number", "graph_api_version", "webhook_status", "metadata", "updated_at"}),
		}).Create(&configuration).Error; err != nil {
			return err
		}
		if err := syncProviderRecords(tx, configuration, detail.Connection.OwnershipModel, now); err != nil {
			return err
		}
		if detail.Connection.Status == channelplatform.StatusSetupRequired && configuration.PhoneNumberID != "" {
			if err := tx.Model(&channelplatform.ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": channelplatform.StatusConnecting, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_configuration_updated", map[string]any{"phone_identity_configured": configuration.PhoneNumberID != "", "business_account_configured": configuration.WhatsAppBusinessAccountID != ""})
	}); err != nil {
		return ConfigurationView{}, err
	}
	if configuration.WebhookStatus == WebhookVerified && s.connectionReady(ctx, configuration) {
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			promoted, err := promoteReadyRecords(tx, configuration, now)
			if err != nil || !promoted {
				return err
			}
			return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_connection_ready", map[string]any{"trigger": "configuration_updated"})
		}); err != nil {
			return ConfigurationView{}, err
		}
	}
	if err := s.refreshSetupState(ctx, actor, connectionID); err != nil {
		return ConfigurationView{}, err
	}
	return s.GetConfiguration(ctx, actor, connectionID)
}

func (s *Service) MigrateLegacyCredentials(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (LegacyMigrationResult, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionChannelsManage) {
		return LegacyMigrationResult{}, httperror.Forbidden("You do not have permission to migrate WhatsApp credentials")
	}
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return LegacyMigrationResult{}, err
	}
	if detail.Connection.Provider != Provider {
		return LegacyMigrationResult{}, httperror.BadRequest("This connection is not a WhatsApp channel")
	}
	legacy, err := s.legacyChannel(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return LegacyMigrationResult{}, err
	}
	legacyConfig := jsonObject(legacy.Config)
	legacySecrets := jsonObject(legacy.SecretConfig)
	configuration, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return LegacyMigrationResult{}, err
	}
	if configuration.PhoneNumberID == "" {
		configuration.PhoneNumberID = strings.TrimSpace(legacy.PhoneNumberID)
	}
	if configuration.DisplayPhoneNumber == "" {
		configuration.DisplayPhoneNumber = strings.TrimSpace(legacy.DisplayNumber)
	}
	if configuration.WhatsAppBusinessAccountID == "" {
		configuration.WhatsAppBusinessAccountID = strings.TrimSpace(valueString(legacyConfig["whatsapp_business_account_id"]))
	}
	if configuration.MetaBusinessAccountID == "" {
		configuration.MetaBusinessAccountID = strings.TrimSpace(valueString(legacyConfig["meta_business_account_id"]))
	}
	if version := strings.TrimSpace(valueString(legacyConfig["graph_version"])); configuration.GraphAPIVersion == "" && version != "" {
		configuration.GraphAPIVersion = version
	}
	result := LegacyMigrationResult{ConnectionID: connectionID, ConfigurationFound: configuration.PhoneNumberID != "" || configuration.DisplayPhoneNumber != "", Encrypted: s.encrypted != nil, LegacyRetained: true}
	credentials := map[string]string{
		CredentialAccessToken: strings.TrimSpace(valueString(legacySecrets[CredentialAccessToken])),
		CredentialAppSecret:   strings.TrimSpace(valueString(legacySecrets[CredentialAppSecret])),
		CredentialVerifyToken: firstNonEmpty(valueString(legacyConfig[CredentialVerifyToken]), valueString(legacySecrets[CredentialVerifyToken])),
	}
	for _, credentialType := range requiredCredentialTypes {
		value := credentials[credentialType]
		if value == "" {
			continue
		}
		result.CredentialsFound++
		reference := "legacy://channels/" + connectionID.String() + "/" + credentialType
		if s.encrypted != nil {
			reference, err = s.encrypted.Save(ctx, actor.OrganizationID, connectionID, credentialType, value)
			if err != nil {
				return LegacyMigrationResult{}, err
			}
		}
		if _, err := s.platform.UpsertCredentialReference(ctx, actor, connectionID, channelplatform.CredentialReferenceInput{CredentialType: credentialType, SecretRef: reference, Status: channelplatform.CredentialPresent}); err != nil {
			return LegacyMigrationResult{}, err
		}
		result.CredentialsMigrated++
	}
	configurationInput := ConfigurationInput{PhoneNumberID: &configuration.PhoneNumberID, WhatsAppBusinessAccountID: &configuration.WhatsAppBusinessAccountID, MetaBusinessAccountID: &configuration.MetaBusinessAccountID, DisplayPhoneNumber: &configuration.DisplayPhoneNumber, GraphAPIVersion: &configuration.GraphAPIVersion}
	if _, err := s.UpdateConfiguration(ctx, actor, connectionID, configurationInput); err != nil {
		return LegacyMigrationResult{}, err
	}
	if err := audit(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_legacy_credentials_migrated", map[string]any{"credentials_found": result.CredentialsFound, "credentials_migrated": result.CredentialsMigrated, "encrypted": result.Encrypted, "legacy_retained": true}); err != nil {
		return LegacyMigrationResult{}, err
	}
	return result, nil
}

func (s *Service) EvaluateHealth(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (HealthResult, error) {
	view, err := s.GetConfiguration(ctx, actor, connectionID)
	if err != nil {
		return HealthResult{}, err
	}
	issues := []string{}
	if !view.Checklist.PhoneIdentityConfigured {
		issues = append(issues, "phone_identity_missing")
	}
	if !view.Checklist.AccessTokenConfigured {
		issues = append(issues, "access_token_missing")
	}
	if !view.Checklist.AppSecretConfigured {
		issues = append(issues, "app_secret_missing")
	}
	if !view.Checklist.VerifyTokenConfigured {
		issues = append(issues, "verify_token_missing")
	}
	if !view.Checklist.WebhookVerified {
		issues = append(issues, "webhook_not_verified")
	}
	if view.ConnectionMethod == ConnectionMethodEmbeddedSignup && view.LastSignatureVerifiedAt == nil {
		issues = append(issues, "signed_webhook_not_observed")
	}
	if view.RateLimitedUntil != nil && view.RateLimitedUntil.After(s.now()) {
		issues = append(issues, "provider_rate_limited")
	}
	if view.LastSignatureRejectedAt != nil && (view.LastSignatureVerifiedAt == nil || view.LastSignatureRejectedAt.After(*view.LastSignatureVerifiedAt)) {
		issues = append(issues, "signature_verification_failed")
	}
	status := channelplatform.HealthHealthy
	if len(issues) > 0 {
		status = channelplatform.HealthDegraded
	}
	if !view.Checklist.PhoneIdentityConfigured || (!view.Checklist.AccessTokenConfigured && !view.Checklist.AppSecretConfigured) {
		status = channelplatform.HealthFailed
	}
	_, err = s.platform.RecordHealthCheck(ctx, actor.OrganizationID, connectionID, channelplatform.HealthCheckInput{Status: status, ErrorCode: firstIssue(issues), ErrorMessage: healthMessage(issues), Metadata: jsonValue(map[string]any{"check_type": "whatsapp_configuration", "issue_count": len(issues)})})
	if err != nil {
		return HealthResult{}, err
	}
	setupState := view.SetupState
	if status == channelplatform.HealthFailed {
		setupState = SetupRequiresAttention
		if credentialStateExpired(view.Credentials) {
			setupState = SetupCredentialExpired
		}
		now := s.now()
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&channelplatform.ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": channelplatform.StatusRequiresAttention, "updated_at": now}).Error; err != nil {
				return err
			}
			return tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"setup_state": setupState, "updated_at": now}).Error
		}); err != nil {
			return HealthResult{}, err
		}
	} else if status == channelplatform.HealthHealthy && view.SetupState == SetupConnected {
		setupState = SetupHealthy
		if err := s.db.WithContext(ctx).Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"setup_state": setupState, "updated_at": s.now()}).Error; err != nil {
			return HealthResult{}, err
		}
	}
	return HealthResult{Status: status, SetupState: setupState, Issues: issues}, nil
}

func credentialStateExpired(credentials []CredentialStatus) bool {
	for _, credential := range credentials {
		if credential.Status == channelplatform.CredentialExpired || credential.Status == channelplatform.CredentialRevoked || credential.Status == channelplatform.CredentialRequiresReauthorization {
			return true
		}
	}
	return false
}

func (s *Service) ResolveByPhoneNumberID(ctx context.Context, phoneNumberID string) (Configuration, channelplatform.ChannelConnection, error) {
	phoneNumberID = strings.TrimSpace(phoneNumberID)
	if phoneNumberID == "" {
		return Configuration{}, channelplatform.ChannelConnection{}, httperror.NotFound("WhatsApp channel identity not found")
	}
	var configuration Configuration
	if err := s.db.WithContext(ctx).Where("phone_number_id = ?", phoneNumberID).First(&configuration).Error; err != nil {
		return Configuration{}, channelplatform.ChannelConnection{}, mapNotFound(err, "WhatsApp channel identity not found")
	}
	var connection channelplatform.ChannelConnection
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND provider = ? AND status IN ?", configuration.ConnectionID, configuration.OrganizationID, Provider, inboundReadyStatuses).First(&connection).Error; err != nil {
		return Configuration{}, channelplatform.ChannelConnection{}, mapNotFound(err, "WhatsApp channel connection not found")
	}
	return configuration, connection, nil
}

func (s *Service) ResolveVerifyToken(ctx context.Context, supplied string) (Configuration, channelplatform.ChannelConnection, bool) {
	supplied = strings.TrimSpace(supplied)
	if supplied == "" {
		return Configuration{}, channelplatform.ChannelConnection{}, false
	}
	var configurations []Configuration
	if err := s.db.WithContext(ctx).Order("updated_at DESC").Find(&configurations).Error; err != nil {
		return Configuration{}, channelplatform.ChannelConnection{}, false
	}
	for _, configuration := range configurations {
		var connection channelplatform.ChannelConnection
		if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND provider = ? AND status <> ?", configuration.ConnectionID, configuration.OrganizationID, Provider, channelplatform.StatusArchived).First(&connection).Error; err != nil {
			continue
		}
		secret, err := s.resolveCredential(ctx, configuration.OrganizationID, configuration.ConnectionID, CredentialVerifyToken)
		if err != nil || len(secret) != len(supplied) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(secret), []byte(supplied)) == 1 {
			return configuration, connection, true
		}
	}
	return Configuration{}, channelplatform.ChannelConnection{}, false
}

func (s *Service) ResolveCredential(ctx context.Context, organizationID, connectionID uuid.UUID, credentialType string) (string, error) {
	return s.resolveCredential(ctx, organizationID, connectionID, credentialType)
}

func (s *Service) MarkWebhookVerified(ctx context.Context, configuration Configuration) error {
	now := s.now()
	connectionReady := s.connectionReady(ctx, configuration)
	setupState := SetupWebhookVerified
	if strings.TrimSpace(configuration.PhoneNumberID) != "" {
		setupState = SetupPhoneVerified
	}
	if connectionReady {
		setupState = SetupTestMessageReady
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).Updates(map[string]any{"webhook_status": WebhookVerified, "setup_state": setupState, "last_webhook_verified_at": now, "last_webhook_verification_attempt_at": now, "last_webhook_verification_error": "", "updated_at": now}).Error; err != nil {
			return err
		}
		if connectionReady {
			if _, err := promoteReadyRecords(tx, configuration, now); err != nil {
				return err
			}
		}
		return audit(tx, &configuration.OrganizationID, nil, configuration.ConnectionID, "whatsapp_webhook_verification_succeeded", map[string]any{"webhook_status": WebhookVerified, "connection_ready": connectionReady})
	})
	if err != nil {
		return err
	}
	_, _, _ = s.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{
		OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider,
		EventType: "webhook_verification", ProviderEventID: "verification-succeeded:" + uuid.NewString(),
		IdempotencyKey: "whatsapp:verification-succeeded:" + uuid.NewString(), NormalizedStatus: "verified", ReceivedAt: now,
	})
	return nil
}

func promoteReadyRecords(tx *gorm.DB, configuration Configuration, now time.Time) (bool, error) {
	var connection channelplatform.ChannelConnection
	if err := tx.Where("organization_id = ? AND id = ?", configuration.OrganizationID, configuration.ConnectionID).First(&connection).Error; err != nil {
		return false, err
	}
	promoted := connection.Status == channelplatform.StatusConnecting || connection.Status == channelplatform.StatusSetupRequired || connection.Status == channelplatform.StatusDisconnected || connection.Status == channelplatform.StatusRequiresAttention
	if promoted {
		if err := tx.Model(&connection).Updates(map[string]any{"status": channelplatform.StatusConnected, "last_connected_at": now, "updated_at": now}).Error; err != nil {
			return false, err
		}
	}
	if err := tx.Model(&channelplatform.ProviderAccount{}).Where("organization_id = ? AND channel_connection_id = ? AND status <> ?", configuration.OrganizationID, configuration.ConnectionID, channelplatform.StatusConnected).Updates(map[string]any{"status": channelplatform.StatusConnected, "updated_at": now}).Error; err != nil {
		return false, err
	}
	if err := tx.Model(&channelplatform.ChannelIdentity{}).Where("organization_id = ? AND channel_connection_id = ? AND status <> ?", configuration.OrganizationID, configuration.ConnectionID, channelplatform.StatusConnected).Updates(map[string]any{"status": channelplatform.StatusConnected, "updated_at": now}).Error; err != nil {
		return false, err
	}
	return promoted, nil
}

func (s *Service) connectionReady(ctx context.Context, configuration Configuration) bool {
	if strings.TrimSpace(configuration.PhoneNumberID) == "" {
		return false
	}
	for _, credentialType := range requiredCredentialTypes {
		if secret, err := s.resolveCredential(ctx, configuration.OrganizationID, configuration.ConnectionID, credentialType); err != nil || strings.TrimSpace(secret) == "" {
			return false
		}
	}
	return true
}

func (s *Service) MarkInbound(ctx context.Context, configuration Configuration) error {
	now := s.now()
	return s.db.WithContext(ctx).Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).Updates(map[string]any{"last_inbound_at": now, "updated_at": now}).Error
}

func (s *Service) MarkOutbound(ctx context.Context, configuration Configuration, failure bool, rateLimitedUntil *time.Time) error {
	now := s.now()
	updates := map[string]any{"updated_at": now}
	if failure {
		updates["last_provider_failure_at"] = now
	} else {
		updates["last_outbound_at"] = now
	}
	if rateLimitedUntil != nil {
		updates["rate_limited_until"] = rateLimitedUntil
	} else if !failure {
		updates["rate_limited_until"] = nil
	}
	return s.db.WithContext(ctx).Model(&Configuration{}).Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).Updates(updates).Error
}

func (s *Service) configurationView(ctx context.Context, connection channelplatform.ConnectionView, configuration Configuration) (ConfigurationView, error) {
	credentials := make([]CredentialStatus, 0, len(requiredCredentialTypes))
	configured := map[string]bool{}
	for _, credentialType := range requiredCredentialTypes {
		status := CredentialStatus{CredentialType: credentialType, Status: channelplatform.CredentialMissing}
		var credential channelplatform.CredentialReference
		err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ?", connection.OrganizationID, connection.ID, credentialType).First(&credential).Error
		if err == nil {
			status.Status = credential.Status
			if credential.ExpiresAt != nil && !credential.ExpiresAt.After(s.now()) {
				status.Status = channelplatform.CredentialExpired
			}
			status.Configured = credential.SecretRef != "" && status.Status == channelplatform.CredentialPresent
			status.ReferenceType = referenceType(credential.SecretRef)
			status.ExpiresAt = credential.ExpiresAt
			status.LastValidatedAt = credential.LastValidatedAt
			updatedAt := credential.UpdatedAt
			status.UpdatedAt = &updatedAt
			if status.Configured {
				if s.resolver == nil {
					status.ResolutionProblem = "Secret resolution is not configured"
				} else if _, resolveErr := s.resolver.Resolve(ctx, channelplatform.SecretScope{OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, CredentialType: credentialType, Reference: credential.SecretRef}); resolveErr == nil {
					status.Resolvable = true
				} else {
					status.ResolutionProblem = publicResolutionProblem(resolveErr)
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ConfigurationView{}, err
		}
		configured[credentialType] = status.Configured && status.Resolvable
		credentials = append(credentials, status)
	}
	legacy, err := s.legacyCredentialsFound(ctx, connection.OrganizationID, connection.ID)
	if err != nil {
		return ConfigurationView{}, err
	}
	checklist := SetupChecklist{
		PhoneIdentityConfigured: configuration.PhoneNumberID != "",
		BusinessAccountKnown:    configuration.WhatsAppBusinessAccountID != "",
		AccessTokenConfigured:   configured[CredentialAccessToken],
		AppSecretConfigured:     configured[CredentialAppSecret],
		VerifyTokenConfigured:   configured[CredentialVerifyToken],
		WebhookVerified:         configuration.WebhookStatus == WebhookVerified,
		SignatureVerified:       configuration.LastSignatureVerifiedAt != nil && (configuration.LastSignatureRejectedAt == nil || !configuration.LastSignatureRejectedAt.After(*configuration.LastSignatureVerifiedAt)),
		TestMessageSent:         configuration.LastTestMessageAt != nil,
		InboundTestReceived:     configuration.LastInboundTestAt != nil,
	}
	checklist.ReadyForInbound = checklist.PhoneIdentityConfigured && checklist.AppSecretConfigured && checklist.VerifyTokenConfigured && checklist.WebhookVerified
	checklist.ReadyForOutbound = checklist.PhoneIdentityConfigured && checklist.AccessTokenConfigured
	checklist.TestMessageReady = checklist.ReadyForInbound && checklist.ReadyForOutbound
	checklist.ReadyToComplete = checklist.TestMessageReady && checklist.TestMessageSent && checklist.InboundTestReceived && checklist.SignatureVerified
	configuration.SetupState = deriveSetupState(configuration, connection, checklist, credentials)
	signatureStatus := "not_observed"
	if checklist.SignatureVerified {
		signatureStatus = "verified"
	} else if configuration.LastSignatureRejectedAt != nil {
		signatureStatus = "rejected"
	}
	events, err := s.operationalEvents(ctx, connection.OrganizationID, connection.ID)
	if err != nil {
		return ConfigurationView{}, err
	}
	return ConfigurationView{
		Configuration: configuration, Connection: connection, Credentials: credentials, Checklist: checklist,
		WebhookCallbackURL: s.configurationWebhookCallbackURL(configuration), SignatureStatus: signatureStatus,
		NextAction: nextSetupAction(configuration.SetupState), OperationalEvents: events,
		LegacyCredentialsFound: legacy, EncryptedStorageEnabled: s.encrypted != nil,
		EmbeddedSignupAvailable: s.embeddedSignupAvailable(),
	}, nil
}

func (s *Service) saveCredentialInput(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, credentialType string, reference, value *string) error {
	if reference != nil && value != nil && strings.TrimSpace(*reference) != "" && strings.TrimSpace(*value) != "" {
		return httperror.BadRequest("Provide either a secret reference or a secret value, not both")
	}
	if reference == nil && value == nil {
		return nil
	}
	secretReference := ""
	if value != nil && strings.TrimSpace(*value) != "" {
		if s.encrypted == nil {
			return httperror.BadRequest("Encrypted channel secret storage is not configured; provide a secure secret reference instead")
		}
		stored, err := s.encrypted.Save(ctx, actor.OrganizationID, connectionID, credentialType, *value)
		if err != nil {
			return err
		}
		secretReference = stored
	} else if reference != nil {
		secretReference = strings.TrimSpace(*reference)
	}
	status := channelplatform.CredentialPresent
	if secretReference == "" {
		status = channelplatform.CredentialMissing
	}
	_, err := s.platform.UpsertCredentialReference(ctx, actor, connectionID, channelplatform.CredentialReferenceInput{CredentialType: credentialType, SecretRef: secretReference, Status: status})
	return err
}

func (s *Service) resolveCredential(ctx context.Context, organizationID, connectionID uuid.UUID, credentialType string) (string, error) {
	if s.resolver == nil {
		return "", errors.New("channel secret resolution is not configured")
	}
	var credential channelplatform.CredentialReference
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ? AND status = ?", organizationID, connectionID, credentialType, channelplatform.CredentialPresent).First(&credential).Error; err != nil {
		return "", err
	}
	return s.resolver.Resolve(ctx, channelplatform.SecretScope{OrganizationID: organizationID, ConnectionID: connectionID, CredentialType: credentialType, Reference: credential.SecretRef})
}

func (s *Service) findOrDefaultConfiguration(ctx context.Context, organizationID, connectionID uuid.UUID) (Configuration, error) {
	var configuration Configuration
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", organizationID, connectionID).First(&configuration).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Configuration{OrganizationID: organizationID, ConnectionID: connectionID, GraphAPIVersion: "v20.0", ConnectionMethod: ConnectionMethodManual, AuthorizationStatus: AuthorizationNotStarted, AssistedSetupStatus: AssistedNotRequested, WebhookStatus: WebhookNotConfigured, SetupState: SetupNotConnected, Metadata: "{}"}, nil
	}
	return configuration, err
}

type legacyChannelRow struct {
	PhoneNumberID string
	DisplayNumber string
	Config        string
	SecretConfig  string
}

func (s *Service) legacyChannel(ctx context.Context, organizationID, connectionID uuid.UUID) (legacyChannelRow, error) {
	var row legacyChannelRow
	err := s.db.WithContext(ctx).Table("channels").Select("phone_number_id, display_number, config, secret_config").Where("organization_id = ? AND id = ? AND provider = ?", organizationID, connectionID, Provider).First(&row).Error
	return row, mapNotFound(err, "WhatsApp channel not found")
}

func (s *Service) legacyCredentialsFound(ctx context.Context, organizationID, connectionID uuid.UUID) (bool, error) {
	legacy, err := s.legacyChannel(ctx, organizationID, connectionID)
	if err != nil {
		return false, err
	}
	config := jsonObject(legacy.Config)
	secrets := jsonObject(legacy.SecretConfig)
	return strings.TrimSpace(valueString(config[CredentialVerifyToken])) != "" || strings.TrimSpace(valueString(secrets[CredentialVerifyToken])) != "" || strings.TrimSpace(valueString(secrets[CredentialAccessToken])) != "" || strings.TrimSpace(valueString(secrets[CredentialAppSecret])) != "", nil
}

func applyConfigurationInput(configuration *Configuration, input ConfigurationInput) error {
	assign := func(target *string, source *string) {
		if source != nil {
			*target = strings.TrimSpace(*source)
		}
	}
	assign(&configuration.PhoneNumberID, input.PhoneNumberID)
	assign(&configuration.WhatsAppBusinessAccountID, input.WhatsAppBusinessAccountID)
	assign(&configuration.MetaBusinessAccountID, input.MetaBusinessAccountID)
	assign(&configuration.DisplayPhoneNumber, input.DisplayPhoneNumber)
	assign(&configuration.GraphAPIVersion, input.GraphAPIVersion)
	if configuration.PhoneNumberID != "" && !regexp.MustCompile(`^[0-9]{5,40}$`).MatchString(configuration.PhoneNumberID) {
		return httperror.BadRequest("WhatsApp phone number ID must contain only digits")
	}
	if configuration.GraphAPIVersion != "" && !regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]{1,3}$`).MatchString(configuration.GraphAPIVersion) {
		return httperror.BadRequest("Meta Graph API version must look like v20.0")
	}
	return nil
}

func syncProviderRecords(tx *gorm.DB, configuration Configuration, ownership string, now time.Time) error {
	if configuration.WhatsAppBusinessAccountID != "" {
		var account channelplatform.ProviderAccount
		err := tx.Where("organization_id = ? AND channel_connection_id = ?", configuration.OrganizationID, configuration.ConnectionID).First(&account).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			account = channelplatform.ProviderAccount{ID: uuid.New(), OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider, Status: channelplatform.StatusConnecting, CreatedAt: now}
		} else if err != nil {
			return err
		}
		account.ProviderAccountID = configuration.WhatsAppBusinessAccountID
		account.DisplayName = "WhatsApp Business Account"
		account.OwnershipModel = ownership
		if account.Status == "" {
			account.Status = channelplatform.StatusConnecting
		}
		account.Metadata = "{}"
		account.UpdatedAt = now
		if err := tx.Save(&account).Error; err != nil {
			return err
		}
	}
	if configuration.PhoneNumberID != "" {
		var identity channelplatform.ChannelIdentity
		err := tx.Where("organization_id = ? AND channel_connection_id = ? AND identity_type = ?", configuration.OrganizationID, configuration.ConnectionID, "phone_number").First(&identity).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			identity = channelplatform.ChannelIdentity{ID: uuid.New(), OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider, IdentityType: "phone_number", Status: channelplatform.StatusConnecting, CreatedAt: now}
		} else if err != nil {
			return err
		}
		identity.DisplayName = firstNonEmpty(configuration.DisplayPhoneNumber, "WhatsApp number")
		identity.ProviderIdentityID = configuration.PhoneNumberID
		identity.ExternalHandle = configuration.DisplayPhoneNumber
		if identity.Status == "" {
			identity.Status = channelplatform.StatusConnecting
		}
		identity.Metadata = "{}"
		identity.UpdatedAt = now
		if err := tx.Save(&identity).Error; err != nil {
			return err
		}
	}
	return nil
}

func audit(tx *gorm.DB, organizationID, actorID *uuid.UUID, connectionID uuid.UUID, action string, metadata map[string]any) error {
	entry := organization.AuditLog{ID: uuid.New(), OrganizationID: organizationID, ActorUserID: actorID, TargetType: "channel_connection", TargetID: &connectionID, Action: action, Metadata: jsonValue(metadata), CreatedAt: time.Now().UTC()}
	return tx.Create(&entry).Error
}

func jsonObject(value string) map[string]any {
	result := map[string]any{}
	_ = json.Unmarshal([]byte(value), &result)
	return result
}

func jsonValue(value any) string {
	body, _ := json.Marshal(value)
	if len(body) == 0 || string(body) == "null" {
		return "{}"
	}
	return string(body)
}

func valueString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func referenceType(reference string) string {
	typeName, _, found := strings.Cut(strings.TrimSpace(reference), "://")
	if !found {
		return ""
	}
	return strings.ToLower(typeName)
}

func publicResolutionProblem(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "Secret reference was not found"
	}
	return "Secret reference cannot be resolved"
}

func firstIssue(issues []string) string {
	if len(issues) == 0 {
		return ""
	}
	return issues[0]
}

func healthMessage(issues []string) string {
	if len(issues) == 0 {
		return "WhatsApp configuration is ready"
	}
	copyIssues := append([]string(nil), issues...)
	sort.Strings(copyIssues)
	return "WhatsApp configuration requires attention: " + strings.Join(copyIssues, ", ")
}

func mapNotFound(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httperror.NotFound(message)
	}
	return err
}
