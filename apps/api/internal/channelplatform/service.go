package channelplatform

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ListConnections(ctx context.Context, actor auth.CurrentUser) ([]ConnectionView, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return nil, err
	}
	var connections []ChannelConnection
	if err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Find(&connections).Error; err != nil {
		return nil, err
	}
	views := make([]ConnectionView, 0, len(connections))
	for _, connection := range connections {
		view, err := s.connectionView(ctx, connection)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) GetConnection(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (ConnectionDetail, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return ConnectionDetail{}, err
	}
	connection, err := s.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionDetail{}, err
	}
	view, err := s.connectionView(ctx, connection)
	if err != nil {
		return ConnectionDetail{}, err
	}
	var accounts []ProviderAccount
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Order("created_at DESC").Find(&accounts).Error; err != nil {
		return ConnectionDetail{}, err
	}
	var identities []ChannelIdentity
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Order("created_at DESC").Find(&identities).Error; err != nil {
		return ConnectionDetail{}, err
	}
	credentials, err := s.listCredentialViews(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionDetail{}, err
	}
	health, err := s.healthSummary(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionDetail{}, err
	}
	metrics, err := s.metricsSummary(ctx, actor.OrganizationID, connectionID, s.now().AddDate(0, 0, -29), s.now())
	if err != nil {
		return ConnectionDetail{}, err
	}
	return ConnectionDetail{Connection: view, Accounts: accounts, Identities: identities, Credentials: credentials, Health: health, Metrics: metrics}, nil
}

func (s *Service) CreateConnection(ctx context.Context, actor auth.CurrentUser, input CreateConnectionInput) (ConnectionView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return ConnectionView{}, err
	}
	provider := normalizeKey(input.Provider)
	displayName := strings.TrimSpace(input.DisplayName)
	status := normalizeKey(input.Status)
	if status == "" {
		status = StatusSetupRequired
	}
	ownership := normalizeKey(input.OwnershipModel)
	if ownership == "" {
		ownership = OwnershipMerchantManaged
	}
	environment := normalizeKey(input.Environment)
	if environment == "" {
		environment = EnvironmentSandbox
	}
	if !validProvider(provider) || displayName == "" {
		return ConnectionView{}, httperror.BadRequest("Provider and display name are required")
	}
	if status != StatusNotConnected && status != StatusSetupRequired {
		return ConnectionView{}, httperror.BadRequest("New channels must start as not connected or setup required")
	}
	if !validOwnership(ownership) || !validEnvironment(environment) {
		return ConnectionView{}, httperror.BadRequest("Channel ownership or environment is not valid")
	}
	capabilities, err := normalizeCapabilities(input.Capabilities, provider)
	if err != nil {
		return ConnectionView{}, err
	}
	now := s.now()
	connection := ChannelConnection{
		ID: uuid.New(), OrganizationID: actor.OrganizationID, Provider: provider,
		DisplayName: displayName, Status: status, OwnershipModel: ownership,
		Environment: environment, Capabilities: jsonValue(capabilities), CreatedByUserID: &actor.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&connection).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connection.ID, "channel_connection_created", map[string]any{"provider": provider, "ownership_model": ownership, "environment": environment})
	}); err != nil {
		return ConnectionView{}, err
	}
	return s.connectionView(ctx, connection)
}

func (s *Service) UpdateConnection(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input UpdateConnectionInput) (ConnectionView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return ConnectionView{}, err
	}
	connection, err := s.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionView{}, err
	}
	updates := map[string]any{"updated_at": s.now()}
	changed := []string{}
	previousOwnership := connection.OwnershipModel
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if value == "" {
			return ConnectionView{}, httperror.BadRequest("Display name is required")
		}
		updates["display_name"] = value
		changed = append(changed, "display_name")
	}
	if input.OwnershipModel != nil {
		value := normalizeKey(*input.OwnershipModel)
		if !validOwnership(value) {
			return ConnectionView{}, httperror.BadRequest("Ownership model is not valid")
		}
		updates["ownership_model"] = value
		connection.OwnershipModel = value
		changed = append(changed, "ownership_model")
	}
	if input.Environment != nil {
		value := normalizeKey(*input.Environment)
		if !validEnvironment(value) {
			return ConnectionView{}, httperror.BadRequest("Environment is not valid")
		}
		updates["environment"] = value
		changed = append(changed, "environment")
	}
	if input.Capabilities != nil {
		values, normalizeErr := normalizeCapabilities(*input.Capabilities, connection.Provider)
		if normalizeErr != nil {
			return ConnectionView{}, normalizeErr
		}
		updates["capabilities"] = jsonValue(values)
		changed = append(changed, "capabilities")
	}
	if input.Status != nil {
		value := normalizeKey(*input.Status)
		if !canTransition(connection.Status, value) {
			return ConnectionView{}, httperror.BadRequest("That channel status change is not allowed")
		}
		updates["status"] = value
		changed = append(changed, "status")
		if (value == StatusConnected || value == StatusHealthy) && connection.Status != StatusConnected && connection.Status != StatusHealthy && connection.Status != "active" {
			updates["last_connected_at"] = s.now()
		}
	}
	if len(changed) == 0 {
		return s.connectionView(ctx, connection)
	}
	sort.Strings(changed)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return httperror.NotFound("Channel connection not found")
		}
		if previousOwnership != connection.OwnershipModel {
			if err := auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_ownership_changed", map[string]any{"from": previousOwnership, "to": connection.OwnershipModel}); err != nil {
				return err
			}
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_connection_updated", map[string]any{"fields": changed})
	}); err != nil {
		return ConnectionView{}, err
	}
	connection, err = s.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionView{}, err
	}
	return s.connectionView(ctx, connection)
}

func (s *Service) ArchiveConnection(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (ConnectionView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return ConnectionView{}, err
	}
	connection, err := s.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return ConnectionView{}, err
	}
	if connection.Status == StatusArchived {
		return s.connectionView(ctx, connection)
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, connectionID).Updates(map[string]any{"status": StatusArchived, "updated_at": now}).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_connection", &connectionID, "channel_connection_archived", map[string]any{"previous_status": connection.Status})
	}); err != nil {
		return ConnectionView{}, err
	}
	connection.Status = StatusArchived
	connection.UpdatedAt = now
	return s.connectionView(ctx, connection)
}

func (s *Service) ListCredentials(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) ([]CredentialReferenceView, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return nil, err
	}
	if _, err := s.findConnection(ctx, actor.OrganizationID, connectionID); err != nil {
		return nil, err
	}
	return s.listCredentialViews(ctx, actor.OrganizationID, connectionID)
}

func (s *Service) UpsertCredentialReference(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input CredentialReferenceInput) (CredentialReferenceView, error) {
	if err := require(actor, authz.PermissionChannelsManage); err != nil {
		return CredentialReferenceView{}, err
	}
	connection, err := s.findConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return CredentialReferenceView{}, err
	}
	credentialType := normalizeKey(input.CredentialType)
	status := normalizeKey(input.Status)
	if status == "" {
		status = CredentialMissing
	}
	secretRef := strings.TrimSpace(input.SecretRef)
	if credentialType == "" || !validCredentialStatus(status) {
		return CredentialReferenceView{}, httperror.BadRequest("Credential type or status is not valid")
	}
	if status != CredentialMissing && !validSecretReference(secretRef) {
		return CredentialReferenceView{}, httperror.BadRequest("Use a secret reference such as vault://, secret://, kms://, or env://; raw provider credentials are not accepted")
	}
	if secretRef != "" && !validSecretReference(secretRef) {
		return CredentialReferenceView{}, httperror.BadRequest("Secret reference format is not valid")
	}
	now := s.now()
	var credential CredentialReference
	findErr := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ?", actor.OrganizationID, connectionID, credentialType).First(&credential).Error
	created := errors.Is(findErr, gorm.ErrRecordNotFound)
	if findErr != nil && !created {
		return CredentialReferenceView{}, findErr
	}
	if created {
		credential = CredentialReference{ID: uuid.New(), OrganizationID: actor.OrganizationID, ConnectionID: connectionID, Provider: connection.Provider, CredentialType: credentialType, OwnershipModel: connection.OwnershipModel, SecretRef: secretRef, Status: status, ExpiresAt: input.ExpiresAt, LastValidatedAt: input.LastValidatedAt, Metadata: jsonObject(input.Metadata), CreatedAt: now, UpdatedAt: now}
	} else {
		credential.Provider = connection.Provider
		credential.OwnershipModel = connection.OwnershipModel
		credential.SecretRef = secretRef
		credential.Status = status
		credential.ExpiresAt = input.ExpiresAt
		credential.LastValidatedAt = input.LastValidatedAt
		credential.Metadata = jsonObject(input.Metadata)
		credential.UpdatedAt = now
	}
	action := "channel_credential_reference_updated"
	if created {
		action = "channel_credential_reference_created"
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if created {
			if err := tx.Create(&credential).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&credential).Error; err != nil {
			return err
		}
		return auditTx(tx, &actor.OrganizationID, &actor.ID, "channel_credential_reference", &credential.ID, action, map[string]any{"channel_connection_id": connectionID.String(), "credential_type": credentialType, "status": status})
	}); err != nil {
		return CredentialReferenceView{}, err
	}
	return credentialView(credential), nil
}

func (s *Service) GetHealthSummary(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (HealthSummary, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return HealthSummary{}, err
	}
	if _, err := s.findConnection(ctx, actor.OrganizationID, connectionID); err != nil {
		return HealthSummary{}, err
	}
	return s.healthSummary(ctx, actor.OrganizationID, connectionID)
}

func (s *Service) RecordHealthCheck(ctx context.Context, organizationID, connectionID uuid.UUID, input HealthCheckInput) (HealthCheck, error) {
	connection, err := s.findConnection(ctx, organizationID, connectionID)
	if err != nil {
		return HealthCheck{}, err
	}
	status := normalizeKey(input.Status)
	if status != HealthHealthy && status != HealthDegraded && status != HealthFailed {
		return HealthCheck{}, httperror.BadRequest("Health status is not valid")
	}
	now := s.now()
	check := HealthCheck{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, Status: status, CheckedAt: now, LatencyMS: input.LatencyMS, ErrorCode: strings.TrimSpace(input.ErrorCode), ErrorMessage: strings.TrimSpace(input.ErrorMessage), Metadata: jsonObject(input.Metadata), CreatedAt: now}
	channelStatus := connection.Status
	if connection.Status == StatusConnected || connection.Status == StatusHealthy || connection.Status == StatusDegraded || connection.Status == StatusRequiresAttention {
		switch status {
		case HealthHealthy:
			channelStatus = StatusHealthy
		case HealthDegraded:
			channelStatus = StatusDegraded
		case HealthFailed:
			channelStatus = StatusRequiresAttention
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&check).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelConnection{}).Where("organization_id = ? AND id = ?", organizationID, connectionID).Updates(map[string]any{"last_health_check_at": now, "status": channelStatus, "updated_at": now}).Error; err != nil {
			return err
		}
		if channelStatus != connection.Status {
			return auditTx(tx, &organizationID, nil, "channel_connection", &connectionID, "channel_health_status_changed", map[string]any{"from": connection.Status, "to": channelStatus, "health_check_id": check.ID.String()})
		}
		return nil
	}); err != nil {
		return HealthCheck{}, err
	}
	return check, nil
}

func (s *Service) ListProviderEvents(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, limit int) ([]ProviderEvent, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return nil, err
	}
	if _, err := s.findConnection(ctx, actor.OrganizationID, connectionID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var events []ProviderEvent
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Order("received_at DESC").Limit(limit).Find(&events).Error
	return events, err
}

func (s *Service) RecordProviderEvent(ctx context.Context, input ProviderEventInput) (ProviderEvent, bool, error) {
	connection, err := s.findConnection(ctx, input.OrganizationID, input.ConnectionID)
	if err != nil {
		return ProviderEvent{}, false, err
	}
	provider := normalizeKey(input.Provider)
	providerEventID := strings.TrimSpace(input.ProviderEventID)
	if provider == "" {
		provider = connection.Provider
	}
	if provider != connection.Provider || providerEventID == "" || strings.TrimSpace(input.EventType) == "" {
		return ProviderEvent{}, false, httperror.BadRequest("Provider event identity is not valid for this channel")
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = provider + ":" + providerEventID
	}
	if existing, found, findErr := s.findProviderEvent(ctx, input.OrganizationID, provider, providerEventID, idempotencyKey); findErr != nil || found {
		return existing, false, findErr
	}
	receivedAt := input.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = s.now()
	}
	now := s.now()
	event := ProviderEvent{ID: uuid.New(), OrganizationID: input.OrganizationID, ConnectionID: input.ConnectionID, Provider: provider, EventType: normalizeKey(input.EventType), ProviderEventID: providerEventID, ReceivedAt: receivedAt, NormalizedStatus: defaultString(normalizeKey(input.NormalizedStatus), "received"), IdempotencyKey: idempotencyKey, PayloadMetadata: jsonValue(input.PayloadMetadata), ProcessingError: strings.TrimSpace(input.ProcessingError), CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&event).Error; err != nil {
		if existing, found, findErr := s.findProviderEvent(ctx, input.OrganizationID, provider, providerEventID, idempotencyKey); findErr == nil && found {
			return existing, false, nil
		}
		return ProviderEvent{}, false, err
	}
	return event, true, nil
}

func (s *Service) GetMetricsSummary(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, from, to time.Time) (MetricsSummary, error) {
	if err := require(actor, authz.PermissionChannelsView); err != nil {
		return MetricsSummary{}, err
	}
	if _, err := s.findConnection(ctx, actor.OrganizationID, connectionID); err != nil {
		return MetricsSummary{}, err
	}
	return s.metricsSummary(ctx, actor.OrganizationID, connectionID, from, to)
}

func (s *Service) UpsertMetric(ctx context.Context, input MetricInput) (MetricDaily, error) {
	if _, err := s.findConnection(ctx, input.OrganizationID, input.ConnectionID); err != nil {
		return MetricDaily{}, err
	}
	date := input.MetricDate.UTC().Truncate(24 * time.Hour)
	if input.MetricDate.IsZero() {
		date = s.now().Truncate(24 * time.Hour)
	}
	now := s.now()
	metric := MetricDaily{ID: uuid.New(), OrganizationID: input.OrganizationID, ConnectionID: input.ConnectionID, MetricDate: date, InboundCount: input.InboundCount, OutboundCount: input.OutboundCount, FailedOutboundCount: input.FailedOutboundCount, DeliveredCount: input.DeliveredCount, ReadCount: input.ReadCount, ConversationsStarted: input.ConversationsStarted, ConversationsHumanHandled: input.ConversationsHumanHandled, ConversationsAIHandled: input.ConversationsAIHandled, AverageResponseMS: input.AverageResponseMS, ProviderErrorCount: input.ProviderErrorCount, Metadata: jsonValue(input.Metadata), CreatedAt: now, UpdatedAt: now}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "metric_date"}},
		DoUpdates: clause.AssignmentColumns([]string{"inbound_count", "outbound_count", "failed_outbound_count", "delivered_count", "read_count", "conversations_started", "conversations_human_handled", "conversations_ai_handled", "average_response_ms", "provider_error_count", "metadata", "updated_at"}),
	}).Create(&metric).Error
	if err != nil {
		return MetricDaily{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND metric_date = ?", input.OrganizationID, input.ConnectionID, date).First(&metric).Error; err != nil {
		return MetricDaily{}, err
	}
	return metric, nil
}

func (s *Service) IncrementMetric(ctx context.Context, input MetricInput) (MetricDaily, error) {
	if _, err := s.findConnection(ctx, input.OrganizationID, input.ConnectionID); err != nil {
		return MetricDaily{}, err
	}
	if metricInputHasNegativeValue(input) {
		return MetricDaily{}, httperror.BadRequest("Channel metric increments cannot be negative")
	}
	date := input.MetricDate.UTC().Truncate(24 * time.Hour)
	if input.MetricDate.IsZero() {
		date = s.now().Truncate(24 * time.Hour)
	}
	now := s.now()
	seed := MetricDaily{ID: uuid.New(), OrganizationID: input.OrganizationID, ConnectionID: input.ConnectionID, MetricDate: date, Metadata: "{}", CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "metric_date"}}, DoNothing: true}).Create(&seed).Error; err != nil {
		return MetricDaily{}, err
	}
	updates := map[string]any{
		"inbound_count":               gorm.Expr("inbound_count + ?", input.InboundCount),
		"outbound_count":              gorm.Expr("outbound_count + ?", input.OutboundCount),
		"failed_outbound_count":       gorm.Expr("failed_outbound_count + ?", input.FailedOutboundCount),
		"delivered_count":             gorm.Expr("delivered_count + ?", input.DeliveredCount),
		"read_count":                  gorm.Expr("read_count + ?", input.ReadCount),
		"conversations_started":       gorm.Expr("conversations_started + ?", input.ConversationsStarted),
		"conversations_human_handled": gorm.Expr("conversations_human_handled + ?", input.ConversationsHumanHandled),
		"conversations_ai_handled":    gorm.Expr("conversations_ai_handled + ?", input.ConversationsAIHandled),
		"provider_error_count":        gorm.Expr("provider_error_count + ?", input.ProviderErrorCount),
		"updated_at":                  now,
	}
	if input.AverageResponseMS > 0 {
		updates["average_response_ms"] = input.AverageResponseMS
	}
	if input.Metadata != nil {
		updates["metadata"] = jsonValue(input.Metadata)
	}
	if err := s.db.WithContext(ctx).Model(&MetricDaily{}).Where("organization_id = ? AND channel_connection_id = ? AND metric_date = ?", input.OrganizationID, input.ConnectionID, date).Updates(updates).Error; err != nil {
		return MetricDaily{}, err
	}
	var metric MetricDaily
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND metric_date = ?", input.OrganizationID, input.ConnectionID, date).First(&metric).Error; err != nil {
		return MetricDaily{}, err
	}
	return metric, nil
}

func (s *Service) connectionView(ctx context.Context, connection ChannelConnection) (ConnectionView, error) {
	view := ConnectionView{ChannelConnection: connection, Capabilities: stringList(connection.Capabilities), HealthStatus: "not_checked", CredentialStatus: CredentialMissing}
	var latest HealthCheck
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", connection.OrganizationID, connection.ID).Order("checked_at DESC").First(&latest).Error; err == nil {
		view.HealthStatus = latest.Status
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ConnectionView{}, err
	}
	credentials, err := s.listCredentialViews(ctx, connection.OrganizationID, connection.ID)
	if err != nil {
		return ConnectionView{}, err
	}
	view.CredentialStatus = summarizeCredentialStatus(credentials)
	var identityCount int64
	if err := s.db.WithContext(ctx).Model(&ChannelIdentity{}).Where("organization_id = ? AND channel_connection_id = ?", connection.OrganizationID, connection.ID).Count(&identityCount).Error; err != nil {
		return ConnectionView{}, err
	}
	view.IdentityCount = int(identityCount)
	return view, nil
}

func (s *Service) healthSummary(ctx context.Context, organizationID, connectionID uuid.UUID) (HealthSummary, error) {
	var checks []HealthCheck
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", organizationID, connectionID).Order("checked_at DESC").Limit(100).Find(&checks).Error; err != nil {
		return HealthSummary{}, err
	}
	summary := HealthSummary{ConnectionID: connectionID, Status: "not_checked"}
	if len(checks) > 0 {
		summary.Status = checks[0].Status
		summary.LastCheck = &checks[0]
	}
	for _, check := range checks {
		switch check.Status {
		case HealthHealthy:
			summary.HealthyCount++
		case HealthDegraded:
			summary.DegradedCount++
		case HealthFailed:
			summary.FailedCount++
		}
	}
	return summary, nil
}

func (s *Service) metricsSummary(ctx context.Context, organizationID, connectionID uuid.UUID, from, to time.Time) (MetricsSummary, error) {
	if to.IsZero() {
		to = s.now()
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, -29)
	}
	from = from.UTC().Truncate(24 * time.Hour)
	to = to.UTC().Truncate(24 * time.Hour).Add(24*time.Hour - time.Nanosecond)
	if from.After(to) {
		return MetricsSummary{}, httperror.BadRequest("Metrics start date must be before the end date")
	}
	var rows []MetricDaily
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND metric_date BETWEEN ? AND ?", organizationID, connectionID, from, to).Order("metric_date ASC").Find(&rows).Error; err != nil {
		return MetricsSummary{}, err
	}
	summary := MetricsSummary{ConnectionID: connectionID, From: from, To: to, Days: int64(len(rows))}
	var responseTotal int64
	for _, row := range rows {
		summary.InboundCount += row.InboundCount
		summary.OutboundCount += row.OutboundCount
		summary.FailedOutboundCount += row.FailedOutboundCount
		summary.DeliveredCount += row.DeliveredCount
		summary.ReadCount += row.ReadCount
		summary.ConversationsStarted += row.ConversationsStarted
		summary.ConversationsHumanHandled += row.ConversationsHumanHandled
		summary.ConversationsAIHandled += row.ConversationsAIHandled
		summary.ProviderErrorCount += row.ProviderErrorCount
		responseTotal += row.AverageResponseMS
	}
	if len(rows) > 0 {
		summary.AverageResponseMS = responseTotal / int64(len(rows))
	}
	return summary, nil
}

func (s *Service) listCredentialViews(ctx context.Context, organizationID, connectionID uuid.UUID) ([]CredentialReferenceView, error) {
	var credentials []CredentialReference
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", organizationID, connectionID).Order("credential_type ASC").Find(&credentials).Error; err != nil {
		return nil, err
	}
	views := make([]CredentialReferenceView, 0, len(credentials))
	for _, credential := range credentials {
		views = append(views, credentialView(credential))
	}
	return views, nil
}

func credentialView(credential CredentialReference) CredentialReferenceView {
	return CredentialReferenceView{ID: credential.ID, ConnectionID: credential.ConnectionID, Provider: credential.Provider, CredentialType: credential.CredentialType, OwnershipModel: credential.OwnershipModel, Status: credential.Status, HasSecretReference: credential.SecretRef != "", ReferenceType: secretReferenceType(credential.SecretRef), ExpiresAt: credential.ExpiresAt, LastValidatedAt: credential.LastValidatedAt, UpdatedAt: credential.UpdatedAt}
}

func (s *Service) findConnection(ctx context.Context, organizationID, connectionID uuid.UUID) (ChannelConnection, error) {
	var connection ChannelConnection
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, connectionID).First(&connection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChannelConnection{}, httperror.NotFound("Channel connection not found")
		}
		return ChannelConnection{}, err
	}
	return connection, nil
}

func (s *Service) findProviderEvent(ctx context.Context, organizationID uuid.UUID, provider, providerEventID, idempotencyKey string) (ProviderEvent, bool, error) {
	var event ProviderEvent
	err := s.db.WithContext(ctx).Where("organization_id = ? AND ((provider = ? AND provider_event_id = ?) OR idempotency_key = ?)", organizationID, provider, providerEventID, idempotencyKey).First(&event).Error
	if err == nil {
		return event, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProviderEvent{}, false, nil
	}
	return ProviderEvent{}, false, err
}

func require(actor auth.CurrentUser, permission authz.Permission) error {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(permission) {
		return httperror.Forbidden("You do not have permission to manage this channel information")
	}
	return nil
}

func canTransition(from, to string) bool {
	from = normalizeKey(from)
	to = normalizeKey(to)
	if from == to {
		return true
	}
	allowed := map[string][]string{
		"draft":                 {StatusSetupRequired, StatusArchived},
		"active":                {StatusConnected, StatusHealthy, StatusDegraded, StatusRequiresAttention, StatusDisconnected, StatusArchived},
		"inactive":              {StatusSetupRequired, StatusConnecting, StatusArchived},
		"disabled":              {StatusSetupRequired, StatusConnecting, StatusArchived},
		StatusNotConnected:      {StatusSetupRequired, StatusArchived},
		StatusSetupRequired:     {StatusConnecting, StatusDisconnected, StatusRequiresAttention, StatusArchived},
		StatusConnecting:        {StatusConnected, StatusDegraded, StatusDisconnected, StatusRequiresAttention, StatusArchived},
		StatusConnected:         {StatusHealthy, StatusDegraded, StatusDisconnected, StatusRequiresAttention, StatusArchived},
		StatusHealthy:           {StatusDegraded, StatusDisconnected, StatusRequiresAttention, StatusArchived},
		StatusDegraded:          {StatusHealthy, StatusDisconnected, StatusRequiresAttention, StatusArchived},
		StatusDisconnected:      {StatusConnecting, StatusSetupRequired, StatusArchived},
		StatusRequiresAttention: {StatusConnecting, StatusHealthy, StatusDisconnected, StatusArchived},
		StatusArchived:          {},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func normalizeCapabilities(values []string, provider string) ([]string, error) {
	if len(values) == 0 {
		switch provider {
		case "whatsapp":
			values = []string{"inbound_messages", "outbound_messages", "media", "templates", "delivery_receipts"}
		case "instagram":
			values = []string{"inbound_messages", "outbound_messages", "media"}
		case "web":
			values = []string{"inbound_messages", "outbound_messages", "media", "analytics"}
		}
	}
	known := map[string]bool{}
	for _, capability := range KnownCapabilities {
		known[capability] = true
	}
	selected := map[string]bool{}
	for _, value := range values {
		value = normalizeKey(value)
		if !known[value] {
			return nil, httperror.BadRequest("Channel capability is not supported")
		}
		selected[value] = true
	}
	result := []string{}
	for _, capability := range KnownCapabilities {
		if selected[capability] {
			result = append(result, capability)
		}
	}
	return result, nil
}

func validProvider(value string) bool {
	if value == "" || len(value) > 48 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validOwnership(value string) bool {
	return value == OwnershipMerchantManaged || value == OwnershipZidiManaged
}

func validEnvironment(value string) bool {
	return value == EnvironmentSandbox || value == EnvironmentProduction
}

func validCredentialStatus(value string) bool {
	switch value {
	case CredentialMissing, CredentialPresent, CredentialExpiring, CredentialExpired, CredentialRevoked, CredentialRequiresReauthorization:
		return true
	default:
		return false
	}
}

func validSecretReference(value string) bool {
	return secretReferenceType(value) != ""
}

func secretReferenceType(value string) string {
	value = strings.TrimSpace(value)
	separator := strings.Index(value, "://")
	if separator <= 0 || separator+3 >= len(value) {
		return ""
	}
	scheme := strings.ToLower(value[:separator])
	switch scheme {
	case "vault", "secret", "kms", "env", "legacy", "dbenc":
		return scheme
	default:
		return ""
	}
}

func summarizeCredentialStatus(credentials []CredentialReferenceView) string {
	if len(credentials) == 0 {
		return CredentialMissing
	}
	priority := []string{CredentialRequiresReauthorization, CredentialRevoked, CredentialExpired, CredentialExpiring, CredentialMissing, CredentialPresent}
	for _, status := range priority {
		for _, credential := range credentials {
			if credential.Status == status {
				return status
			}
		}
	}
	return CredentialMissing
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func metricInputHasNegativeValue(input MetricInput) bool {
	return input.InboundCount < 0 || input.OutboundCount < 0 || input.FailedOutboundCount < 0 || input.DeliveredCount < 0 || input.ReadCount < 0 || input.ConversationsStarted < 0 || input.ConversationsHumanHandled < 0 || input.ConversationsAIHandled < 0 || input.AverageResponseMS < 0 || input.ProviderErrorCount < 0
}

func jsonValue(value any) string {
	if value == nil {
		return "{}"
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	if string(body) == "null" {
		return "{}"
	}
	return string(body)
}

func jsonObject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	var object map[string]any
	if json.Unmarshal([]byte(value), &object) != nil {
		return "{}"
	}
	return jsonValue(object)
}

func stringList(value string) []string {
	var values []string
	if json.Unmarshal([]byte(value), &values) != nil {
		return []string{}
	}
	return values
}

func auditTx(tx *gorm.DB, organizationID, actorID *uuid.UUID, targetType string, targetID *uuid.UUID, action string, metadata map[string]any) error {
	if actorID != nil && *actorID == uuid.Nil {
		actorID = nil
	}
	entry := organization.AuditLog{ID: uuid.New(), OrganizationID: organizationID, ActorUserID: actorID, TargetType: targetType, TargetID: targetID, Action: action, Metadata: jsonValue(metadata), CreatedAt: time.Now().UTC()}
	return tx.Create(&entry).Error
}
