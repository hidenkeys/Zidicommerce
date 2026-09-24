package meta

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const replyWindow = 24 * time.Hour

type Service struct {
	db       *gorm.DB
	platform *channelplatform.Service
	resolver channelplatform.SecretResolver
	now      func() time.Time
}

type ResolvedConnection struct {
	Connection channelplatform.ChannelConnection
	Identity   channelplatform.ChannelIdentity
	Token      string
}

func NewService(db *gorm.DB, platform *channelplatform.Service, resolver channelplatform.SecretResolver) *Service {
	return &Service{db: db, platform: platform, resolver: resolver, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ResolveByProviderIdentity(ctx context.Context, provider, providerIdentityID string) (ResolvedConnection, error) {
	provider = normalize(provider)
	providerIdentityID = strings.TrimSpace(providerIdentityID)
	var identity channelplatform.ChannelIdentity
	if err := s.db.WithContext(ctx).Where("provider = ? AND provider_identity_id = ? AND status IN ?", provider, providerIdentityID, []string{channelplatform.StatusConnected, channelplatform.StatusHealthy, channelplatform.StatusDegraded, channelplatform.StatusRequiresAttention}).First(&identity).Error; err != nil {
		return ResolvedConnection{}, err
	}
	return s.resolve(ctx, identity.OrganizationID, identity.ConnectionID, provider, false)
}

func (s *Service) ResolveForSend(ctx context.Context, organizationID, connectionID uuid.UUID, provider string) (ResolvedConnection, error) {
	return s.resolve(ctx, organizationID, connectionID, normalize(provider), true)
}

func (s *Service) ResolveConnection(ctx context.Context, organizationID, connectionID uuid.UUID, provider string) (ResolvedConnection, error) {
	return s.resolve(ctx, organizationID, connectionID, normalize(provider), false)
}

func (s *Service) resolve(ctx context.Context, organizationID, connectionID uuid.UUID, provider string, withToken bool) (ResolvedConnection, error) {
	var connection channelplatform.ChannelConnection
	query := s.db.WithContext(ctx).Where("id = ? AND provider = ?", connectionID, provider)
	if organizationID != uuid.Nil {
		query = query.Where("organization_id = ?", organizationID)
	}
	if err := query.First(&connection).Error; err != nil {
		return ResolvedConnection{}, err
	}
	organizationID = connection.OrganizationID
	var identity channelplatform.ChannelIdentity
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND provider = ? AND status IN ?", organizationID, connectionID, provider, []string{channelplatform.StatusConnected, channelplatform.StatusHealthy, channelplatform.StatusDegraded, channelplatform.StatusRequiresAttention}).First(&identity).Error; err != nil {
		return ResolvedConnection{}, err
	}
	resolved := ResolvedConnection{Connection: connection, Identity: identity}
	if !withToken {
		return resolved, nil
	}
	if s.resolver == nil {
		return ResolvedConnection{}, errors.New("channel credential resolver is unavailable")
	}
	var credential channelplatform.CredentialReference
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ? AND status = ?", organizationID, connectionID, channelplatform.OAuthCredentialAccessToken, channelplatform.CredentialPresent).First(&credential).Error; err != nil {
		return ResolvedConnection{}, err
	}
	token, err := s.resolver.Resolve(ctx, channelplatform.SecretScope{OrganizationID: organizationID, ConnectionID: connectionID, CredentialType: credential.CredentialType, Reference: credential.SecretRef})
	if err != nil {
		return ResolvedConnection{}, err
	}
	resolved.Token = token
	return resolved, nil
}

func (s *Service) RecordInboundContact(ctx context.Context, resolved ResolvedConnection, customerID string, observedAt time.Time) error {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return errors.New("Meta customer identity is required")
	}
	if observedAt.IsZero() {
		observedAt = s.now()
	}
	state := ContactState{ID: uuid.New(), OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: resolved.Connection.Provider, ExternalCustomerID: customerID, LastInboundAt: observedAt, ConversationWindowEnds: observedAt.Add(replyWindow), CreatedAt: s.now(), UpdatedAt: s.now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "external_customer_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_inbound_at", "conversation_window_ends_at", "updated_at"})}).Create(&state).Error; err != nil {
		return err
	}
	return s.updateConnectionState(ctx, resolved, map[string]any{"webhook_status": WebhookActive, "last_inbound_at": observedAt, "last_signature_verified_at": s.now(), "last_provider_error": "", "last_provider_error_at": nil})
}

func (s *Service) RequireReplyWindow(ctx context.Context, resolved ResolvedConnection, customerID string) error {
	var state ContactState
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND external_customer_id = ?", resolved.Connection.OrganizationID, resolved.Connection.ID, strings.TrimSpace(customerID)).First(&state).Error; err != nil {
		return errors.New("Meta messaging requires a customer-initiated conversation")
	}
	if !state.ConversationWindowEnds.After(s.now()) {
		return errors.New("Meta customer reply window has expired")
	}
	return nil
}

func (s *Service) MarkSignatureRejected(ctx context.Context, resolved ResolvedConnection) error {
	now := s.now()
	state := ConnectionState{ID: uuid.New(), OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: resolved.Connection.Provider, WebhookStatus: WebhookPending, LastSignatureRejectedAt: &now, SignatureRejectionCount: 1, CreatedAt: now, UpdatedAt: now}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}}, DoUpdates: clause.Assignments(map[string]any{"last_signature_rejected_at": now, "signature_rejection_count": gorm.Expr("signature_rejection_count + 1"), "updated_at": now})}).Create(&state).Error
}

func (s *Service) MarkSignatureVerified(ctx context.Context, resolved ResolvedConnection) error {
	now := s.now()
	return s.updateConnectionState(ctx, resolved, map[string]any{"webhook_status": gorm.Expr("CASE WHEN webhook_status = ? THEN ? ELSE ? END", WebhookActive, WebhookActive, WebhookVerified), "last_signature_verified_at": now, "updated_at": now})
}

func (s *Service) MarkOutbound(ctx context.Context, resolved ResolvedConnection, providerError string) error {
	now := s.now()
	updates := map[string]any{"updated_at": now}
	if providerError == "" {
		updates["last_outbound_at"] = now
		updates["last_provider_error"] = ""
		updates["last_provider_error_at"] = nil
	} else {
		updates["last_provider_error"] = providerError
		updates["last_provider_error_at"] = now
	}
	return s.updateConnectionState(ctx, resolved, updates)
}

func (s *Service) updateConnectionState(ctx context.Context, resolved ResolvedConnection, updates map[string]any) error {
	now := s.now()
	state := ConnectionState{ID: uuid.New(), OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID, Provider: resolved.Connection.Provider, WebhookStatus: WebhookPending, CreatedAt: now, UpdatedAt: now}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}}, DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		return tx.Model(&ConnectionState{}).Where("organization_id = ? AND channel_connection_id = ?", resolved.Connection.OrganizationID, resolved.Connection.ID).Updates(updates).Error
	})
}

func (s *Service) MarkProviderEventResult(ctx context.Context, eventID, organizationID uuid.UUID, status, processingError string) error {
	result := s.db.WithContext(ctx).Model(&channelplatform.ProviderEvent{}).Where("id = ? AND organization_id = ?", eventID, organizationID).Updates(map[string]any{"normalized_status": normalize(status), "processing_error": strings.TrimSpace(processingError), "updated_at": s.now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) ClaimProviderEventRetry(ctx context.Context, eventID, organizationID uuid.UUID) (bool, error) {
	result := s.db.WithContext(ctx).Model(&channelplatform.ProviderEvent{}).Where("id = ? AND organization_id = ? AND normalized_status = ?", eventID, organizationID, "failed").Updates(map[string]any{"normalized_status": "processing", "processing_error": "", "updated_at": s.now()})
	return result.RowsAffected == 1, result.Error
}

func (s *Service) ApplyDeliveryUpdate(ctx context.Context, resolved ResolvedConnection, update channelplatform.DeliveryUpdate) (bool, error) {
	providerMessageID := strings.TrimSpace(update.ProviderMessageID)
	status := normalize(update.Status)
	if providerMessageID == "" || (status != "delivered" && status != "read" && status != "failed" && status != "sent") {
		return false, nil
	}
	updates := map[string]any{"updated_at": s.now()}
	switch status {
	case "sent":
		updates["status"], updates["sent_at"] = "sent", update.OccurredAt
	case "delivered":
		updates["status"], updates["delivered_at"] = "delivered", update.OccurredAt
	case "read":
		updates["status"], updates["read_at"] = "read", update.OccurredAt
	case "failed":
		updates["status"], updates["error_message"] = "failed_permanently", "Meta delivery failed"
	}
	result := s.db.WithContext(ctx).Table("channel_outbound_messages").Where("organization_id = ? AND channel_id = ? AND provider_message_id = ?", resolved.Connection.OrganizationID, resolved.Connection.ID, providerMessageID).Updates(updates)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	metric := channelplatform.MetricInput{OrganizationID: resolved.Connection.OrganizationID, ConnectionID: resolved.Connection.ID}
	if status == "delivered" {
		metric.DeliveredCount = 1
	} else if status == "read" {
		metric.ReadCount = 1
	} else if status == "failed" {
		metric.ProviderErrorCount = 1
	}
	if metric.DeliveredCount+metric.ReadCount+metric.ProviderErrorCount > 0 {
		_, _ = s.platform.IncrementMetric(ctx, metric)
	}
	return true, s.audit(ctx, resolved, "meta_delivery_status_updated", map[string]any{"provider_message_id": providerMessageID, "status": status, "error_code": update.ErrorCode})
}

func (s *Service) audit(ctx context.Context, resolved ResolvedConnection, action string, metadata map[string]any) error {
	body, _ := json.Marshal(metadata)
	organizationID, targetID := resolved.Connection.OrganizationID, resolved.Connection.ID
	return s.db.WithContext(ctx).Create(&organization.AuditLog{ID: uuid.New(), OrganizationID: &organizationID, TargetType: "channel_connection", TargetID: &targetID, Action: action, Metadata: string(body), CreatedAt: s.now()}).Error
}

func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
