package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

const serviceWindowDuration = 24 * time.Hour

type PolicyBlockedError struct {
	Decision PolicyDecision
}

func (e PolicyBlockedError) Error() string   { return e.Decision.Reason }
func (e PolicyBlockedError) Permanent() bool { return e.Decision.Decision != PolicyBlockedRateLimit }

func DetectConsentKeyword(body string) (string, bool) {
	value := strings.ToUpper(strings.TrimSpace(body))
	value = strings.Trim(value, " \t\r\n.!?,;:\"'()[]{}")
	if strings.ContainsAny(value, " \t\r\n") || value == "" {
		return "", false
	}
	switch value {
	case "STOP", "UNSUBSCRIBE", "CANCEL":
		return ConsentOptedOut, true
	case "START", "YES", "SUBSCRIBE":
		return ConsentOptedIn, true
	default:
		return "", false
	}
}

func (s *Service) RecordInboundContact(ctx context.Context, configuration Configuration, event channelplatform.InboundEvent) (ContactState, bool, error) {
	normalized, err := normalizeRecipient(event.ExternalCustomerID)
	if err != nil {
		return ContactState{}, false, err
	}
	observedAt := event.Timestamp.UTC()
	if observedAt.IsZero() {
		observedAt = s.now()
	}
	hash := contactIdentityHash(configuration.OrganizationID, configuration.ConnectionID, normalized)
	consent, isKeyword := DetectConsentKeyword(event.Body)
	changed := false
	var row ContactState
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		findErr := tx.Where("organization_id = ? AND channel_connection_id = ? AND external_customer_id_hash = ?", configuration.OrganizationID, configuration.ConnectionID, hash).First(&row).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			row = ContactState{ID: uuid.New(), OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, ExternalCustomerIDHash: hash, MaskedPhone: maskRecipient(normalized), ConsentStatus: ConsentUnknown, ConsentSource: ConsentSourceInbound, Metadata: "{}", CreatedAt: s.now()}
		} else if findErr != nil {
			return findErr
		}
		if row.LastInboundAt == nil || observedAt.After(*row.LastInboundAt) {
			expires := observedAt.Add(serviceWindowDuration)
			row.LastInboundAt = &observedAt
			row.ServiceWindowExpiresAt = &expires
		}
		if isKeyword && row.ConsentStatus != consent {
			changed = true
			row.ConsentStatus = consent
			row.ConsentSource = ConsentSourceInbound
			if consent == ConsentOptedOut {
				row.OptedOutAt = &observedAt
			} else {
				row.OptedInAt, row.OptedOutAt = &observedAt, nil
			}
		}
		row.UpdatedAt = s.now()
		if row.CreatedAt.IsZero() {
			row.CreatedAt = row.UpdatedAt
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "external_customer_id_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"masked_phone", "consent_status", "consent_source", "opted_in_at", "opted_out_at", "last_inbound_at", "service_window_expires_at", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		if changed {
			return audit(tx, &configuration.OrganizationID, nil, configuration.ConnectionID, "whatsapp_contact_consent_keyword", map[string]any{"contact_state_id": row.ID, "consent_status": consent, "source": ConsentSourceInbound})
		}
		return nil
	})
	if err != nil {
		return ContactState{}, false, err
	}
	if changed {
		_, _, _ = s.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider, EventType: "consent_changed", ProviderEventID: "consent:" + event.ProviderEventID, IdempotencyKey: "whatsapp:consent:" + event.ProviderEventID, NormalizedStatus: consent, ReceivedAt: observedAt, PayloadMetadata: map[string]any{"consent_status": consent, "source": ConsentSourceInbound}})
	}
	return row, changed, nil
}

func (s *Service) EvaluateAndRecordPolicy(ctx context.Context, configuration Configuration, command channelplatform.OutboundCommand) (PolicyDecision, *MessageTemplate, error) {
	now := s.now()
	decision := PolicyDecision{Decision: PolicyBlockedUnknown, Reason: "WhatsApp messaging policy could not determine whether this message is allowed"}
	if configuration.RateLimitedUntil != nil && configuration.RateLimitedUntil.After(now) {
		decision.Decision, decision.Reason = PolicyBlockedRateLimit, "WhatsApp is temporarily rate limited; wait before retrying"
		return decision, nil, s.finishPolicyDecision(ctx, configuration, command, nil, nil, decision)
	}
	state, stateErr := s.contactStateForExternalID(ctx, configuration.OrganizationID, configuration.ConnectionID, command.ExternalCustomerID)
	if stateErr != nil && !errors.Is(stateErr, gorm.ErrRecordNotFound) {
		return decision, nil, stateErr
	}
	var stateRef *ContactState
	if stateErr == nil {
		stateRef = &state
		decision.ServiceWindowExpiresAt = state.ServiceWindowExpiresAt
		if state.ConsentStatus == ConsentOptedOut {
			decision.Decision, decision.Reason = PolicyBlockedOptedOut, "This contact has opted out of WhatsApp messages"
			return decision, nil, s.finishPolicyDecision(ctx, configuration, command, stateRef, nil, decision)
		}
	}
	if command.Template != nil {
		template, templateDecision, err := s.validateTemplateDispatch(ctx, configuration, command.Template)
		if err != nil {
			return decision, nil, err
		}
		decision = templateDecision
		decision.ServiceWindowExpiresAt = nil
		if stateRef != nil {
			decision.ServiceWindowExpiresAt = stateRef.ServiceWindowExpiresAt
		}
		if !decision.Allowed {
			return decision, &template, s.finishPolicyDecision(ctx, configuration, command, stateRef, &template, decision)
		}
		if err := s.finishPolicyDecision(ctx, configuration, command, stateRef, &template, decision); err != nil {
			return decision, nil, err
		}
		return decision, &template, nil
	}
	if stateRef == nil {
		decision.Decision, decision.Reason = PolicyBlockedNoContactState, "No WhatsApp contact history is available; use an approved template"
		return decision, nil, s.finishPolicyDecision(ctx, configuration, command, nil, nil, decision)
	}
	if state.ServiceWindowExpiresAt == nil || !state.ServiceWindowExpiresAt.After(now) {
		decision.Decision, decision.Reason = PolicyBlockedWindowClosed, "The 24-hour WhatsApp service window is closed; use an approved template"
		return decision, nil, s.finishPolicyDecision(ctx, configuration, command, stateRef, nil, decision)
	}
	decision.Decision, decision.Allowed, decision.Reason = PolicyAllowedFreeform, true, "The 24-hour WhatsApp service window is open"
	if err := s.finishPolicyDecision(ctx, configuration, command, stateRef, nil, decision); err != nil {
		return decision, nil, err
	}
	return decision, nil, nil
}

func (s *Service) validateTemplateDispatch(ctx context.Context, configuration Configuration, reference *channelplatform.TemplateReference) (MessageTemplate, PolicyDecision, error) {
	decision := PolicyDecision{Decision: PolicyBlockedMissingTemplate, Reason: "An approved WhatsApp template is required"}
	if reference == nil || reference.ID == uuid.Nil {
		return MessageTemplate{}, decision, nil
	}
	var row MessageTemplate
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND id = ?", configuration.OrganizationID, configuration.ConnectionID, reference.ID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MessageTemplate{}, decision, nil
	}
	if err != nil {
		return MessageTemplate{}, decision, err
	}
	decision.TemplateID = &row.ID
	if row.Status != TemplateApproved {
		decision.Decision, decision.Reason = PolicyBlockedTemplateNotApproved, "The selected WhatsApp template is not approved"
		return row, decision, nil
	}
	if reference.Name != "" && normalizeTemplateName(reference.Name) != row.Name || reference.Language != "" && normalizeTemplateLanguage(reference.Language) != row.Language {
		decision.Decision, decision.Reason = PolicyBlockedMissingTemplate, "The selected WhatsApp template reference does not match"
		return row, decision, nil
	}
	_, missing := templateVariableValues(row, reference.Variables)
	if len(missing) > 0 {
		decision.Decision, decision.Reason = PolicyBlockedTemplateVariables, "Required WhatsApp template variables are missing"
		return row, decision, nil
	}
	decision.Decision, decision.Allowed, decision.Reason = PolicyAllowedTemplate, true, "An approved WhatsApp template is available"
	return row, decision, nil
}

func (s *Service) finishPolicyDecision(ctx context.Context, configuration Configuration, command channelplatform.OutboundCommand, state *ContactState, template *MessageTemplate, decision PolicyDecision) error {
	record := PolicyDecisionRecord{ID: uuid.New(), OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, MessageType: strings.ToLower(strings.TrimSpace(command.MessageType)), Decision: decision.Decision, Allowed: decision.Allowed, Reason: decision.Reason, IdempotencyKey: command.IdempotencyKey, Metadata: "{}", CreatedAt: s.now()}
	if state != nil {
		record.ContactStateID = &state.ID
	}
	if template != nil && template.ID != uuid.Nil {
		record.TemplateID = &template.ID
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	status := "allowed"
	if !decision.Allowed {
		status = "blocked"
	}
	_, _, _ = s.platform.RecordProviderEvent(ctx, channelplatform.ProviderEventInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, Provider: Provider, EventType: "policy_" + status, ProviderEventID: "policy:" + command.IdempotencyKey, IdempotencyKey: "whatsapp:policy:" + command.IdempotencyKey, NormalizedStatus: decision.Decision, PayloadMetadata: map[string]any{"allowed": decision.Allowed, "decision": decision.Decision, "message_type": record.MessageType}})
	metric := OperationalMetricDaily{}
	if !decision.Allowed {
		metric.PolicyBlockedCount = 1
		if decision.Decision == PolicyBlockedWindowClosed {
			metric.WindowClosedBlockedCount = 1
		}
	}
	if !decision.Allowed {
		if err := s.incrementOperationalMetric(ctx, configuration.OrganizationID, configuration.ConnectionID, metric); err != nil {
			return err
		}
	}
	return audit(s.db.WithContext(ctx), &configuration.OrganizationID, nil, configuration.ConnectionID, "whatsapp_policy_"+status, map[string]any{"decision": decision.Decision, "message_type": record.MessageType, "template_id": record.TemplateID})
}

func (s *Service) RecordOutboundContact(ctx context.Context, configuration Configuration, externalCustomerID string) error {
	normalized, err := normalizeRecipient(externalCustomerID)
	if err != nil {
		return err
	}
	hash := contactIdentityHash(configuration.OrganizationID, configuration.ConnectionID, normalized)
	now := s.now()
	seed := ContactState{ID: uuid.New(), OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID, ExternalCustomerIDHash: hash, MaskedPhone: maskRecipient(normalized), ConsentStatus: ConsentUnknown, ConsentSource: "system", LastOutboundAt: &now, Metadata: "{}", CreatedAt: now, UpdatedAt: now}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "external_customer_id_hash"}},
		DoUpdates: clause.Assignments(map[string]any{"last_outbound_at": now, "updated_at": now}),
	}).Create(&seed).Error
}

func (s *Service) contactStateForExternalID(ctx context.Context, organizationID, connectionID uuid.UUID, externalCustomerID string) (ContactState, error) {
	normalized, err := normalizeRecipient(externalCustomerID)
	if err != nil {
		return ContactState{}, err
	}
	var row ContactState
	err = s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND external_customer_id_hash = ?", organizationID, connectionID, contactIdentityHash(organizationID, connectionID, normalized)).First(&row).Error
	return row, err
}

func contactIdentityHash(organizationID, connectionID uuid.UUID, value string) string {
	sum := sha256.Sum256([]byte(organizationID.String() + ":" + connectionID.String() + ":" + strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) incrementOperationalMetric(ctx context.Context, organizationID, connectionID uuid.UUID, increment OperationalMetricDaily) error {
	now := s.now()
	date := now.UTC().Truncate(24 * time.Hour)
	seed := OperationalMetricDaily{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, MetricDate: date, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "metric_date"}}, DoNothing: true}).Create(&seed).Error; err != nil {
		return err
	}
	updates := map[string]any{
		"freeform_send_count":          gorm.Expr("freeform_send_count + ?", increment.FreeformSendCount),
		"template_send_count":          gorm.Expr("template_send_count + ?", increment.TemplateSendCount),
		"policy_blocked_count":         gorm.Expr("policy_blocked_count + ?", increment.PolicyBlockedCount),
		"window_closed_blocked_count":  gorm.Expr("window_closed_blocked_count + ?", increment.WindowClosedBlockedCount),
		"rate_limit_count":             gorm.Expr("rate_limit_count + ?", increment.RateLimitCount),
		"invalid_recipient_count":      gorm.Expr("invalid_recipient_count + ?", increment.InvalidRecipientCount),
		"invalid_credential_count":     gorm.Expr("invalid_credential_count + ?", increment.InvalidCredentialCount),
		"provider_delivery_latency_ms": gorm.Expr("provider_delivery_latency_ms + ?", increment.ProviderDeliveryLatencyMS),
		"provider_delivery_samples":    gorm.Expr("provider_delivery_samples + ?", increment.ProviderDeliverySamples),
		"updated_at":                   now,
	}
	return s.db.WithContext(ctx).Model(&OperationalMetricDaily{}).Where("organization_id = ? AND channel_connection_id = ? AND metric_date = ?", organizationID, connectionID, date).Updates(updates).Error
}

func (s *Service) GetAdvancedMetrics(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) (AdvancedMetrics, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, "channels.view"); err != nil {
		return AdvancedMetrics{}, err
	}
	base, err := s.platform.GetMetricsSummary(ctx, actor, connectionID, time.Time{}, time.Time{})
	if err != nil {
		return AdvancedMetrics{}, err
	}
	result := AdvancedMetrics{InboundMessages: base.InboundCount, OutboundMessages: base.OutboundCount, FailedSends: base.FailedOutboundCount, Delivered: base.DeliveredCount, Read: base.ReadCount, AIHandledConversations: base.ConversationsAIHandled, HumanHandledConversations: base.ConversationsHumanHandled, ProviderErrors: base.ProviderErrorCount}
	var metrics []OperationalMetricDaily
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID).Find(&metrics).Error; err != nil {
		return AdvancedMetrics{}, err
	}
	var deliverySamples int64
	for _, row := range metrics {
		result.FreeformSends += row.FreeformSendCount
		result.TemplateSends += row.TemplateSendCount
		result.PolicyBlockedSends += row.PolicyBlockedCount
		result.ServiceWindowClosedBlocks += row.WindowClosedBlockedCount
		result.RateLimits += row.RateLimitCount
		result.InvalidRecipientErrors += row.InvalidRecipientCount
		result.InvalidCredentialErrors += row.InvalidCredentialCount
		if row.ProviderDeliverySamples > 0 {
			result.AverageProviderDeliveryMS += row.ProviderDeliveryLatencyMS
			deliverySamples += row.ProviderDeliverySamples
		}
	}
	if deliverySamples > 0 {
		result.AverageProviderDeliveryMS /= deliverySamples
	}
	now := s.now()
	_ = s.db.WithContext(ctx).Model(&ContactState{}).Where("organization_id = ? AND channel_connection_id = ? AND service_window_expires_at > ?", actor.OrganizationID, connectionID, now).Count(&result.ServiceWindowOpenContacts).Error
	_ = s.db.WithContext(ctx).Model(&ContactState{}).Where("organization_id = ? AND channel_connection_id = ? AND consent_status = ?", actor.OrganizationID, connectionID, ConsentOptedIn).Count(&result.OptedInContacts).Error
	_ = s.db.WithContext(ctx).Model(&ContactState{}).Where("organization_id = ? AND channel_connection_id = ? AND consent_status = ?", actor.OrganizationID, connectionID, ConsentOptedOut).Count(&result.OptedOutContacts).Error
	if total := result.AIHandledConversations + result.HumanHandledConversations; total > 0 {
		result.HandoffRate = float64(result.HumanHandledConversations) / float64(total)
	}
	result.AverageFirstResponseMS = s.averageFirstResponseMS(ctx, actor.OrganizationID, connectionID)
	return result, nil
}

func (s *Service) averageFirstResponseMS(ctx context.Context, organizationID, connectionID uuid.UUID) int64 {
	if !s.db.Migrator().HasTable("conversation_messages") {
		return 0
	}
	type timingRow struct {
		SessionID uuid.UUID
		Direction string
		CreatedAt time.Time
	}
	var rows []timingRow
	if err := s.db.WithContext(ctx).Table("conversation_messages").Select("session_id, direction, created_at").Where("organization_id = ? AND channel_id = ? AND created_at >= ?", organizationID, connectionID, s.now().AddDate(0, 0, -30)).Order("created_at ASC").Find(&rows).Error; err != nil {
		return 0
	}
	firstInbound := map[uuid.UUID]time.Time{}
	responded := map[uuid.UUID]bool{}
	var totalMS, samples int64
	for _, row := range rows {
		if row.Direction == "inbound" {
			if _, exists := firstInbound[row.SessionID]; !exists {
				firstInbound[row.SessionID] = row.CreatedAt
			}
			continue
		}
		inboundAt, exists := firstInbound[row.SessionID]
		if row.Direction == "outbound" && exists && !responded[row.SessionID] && row.CreatedAt.After(inboundAt) {
			totalMS += row.CreatedAt.Sub(inboundAt).Milliseconds()
			samples++
			responded[row.SessionID] = true
		}
	}
	if samples == 0 {
		return 0
	}
	return totalMS / samples
}

func (s *Service) ListPolicyBlocks(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, limit int) ([]PolicyDecisionRecord, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, "channels.view"); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var rows []PolicyDecisionRecord
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND allowed = ?", actor.OrganizationID, connectionID, false).Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *Service) GetConversationPolicyContext(ctx context.Context, actor auth.CurrentUser, connectionID, conversationID uuid.UUID) (ConversationPolicyContext, error) {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(authz.PermissionConversationsView) {
		return ConversationPolicyContext{}, httperror.Forbidden("You do not have permission to view conversation messaging policy")
	}
	var session struct {
		ChannelID              uuid.UUID
		ExternalConversationID string
	}
	if err := s.db.WithContext(ctx).Table("conversation_sessions").Select("channel_id, external_conversation_id").Where("organization_id = ? AND id = ? AND channel_id = ?", actor.OrganizationID, conversationID, connectionID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ConversationPolicyContext{}, httperror.NotFound("Conversation not found")
		}
		return ConversationPolicyContext{}, err
	}
	var configuration Configuration
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, session.ChannelID).First(&configuration).Error; err != nil {
		return ConversationPolicyContext{}, httperror.NotFound("WhatsApp connection not found")
	}
	state, err := s.contactStateForExternalID(ctx, actor.OrganizationID, connectionID, session.ExternalConversationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConversationPolicyContext{ConsentStatus: ConsentUnknown, TemplateRequired: true, Reason: "No WhatsApp contact history is available; use an approved template"}, nil
	}
	if err != nil {
		return ConversationPolicyContext{}, err
	}
	result := ConversationPolicyContext{ConsentStatus: state.ConsentStatus, MaskedPhone: state.MaskedPhone, LastInboundAt: state.LastInboundAt, ServiceWindowExpiresAt: state.ServiceWindowExpiresAt}
	if state.ConsentStatus == ConsentOptedOut {
		result.Reason = "This contact has opted out of WhatsApp messages"
		return result, nil
	}
	result.ServiceWindowOpen = state.ServiceWindowExpiresAt != nil && state.ServiceWindowExpiresAt.After(s.now())
	result.CanSendFreeform = result.ServiceWindowOpen
	result.TemplateRequired = !result.ServiceWindowOpen
	if result.ServiceWindowOpen {
		result.Reason = "Free-form replies are allowed while the 24-hour service window is open"
	} else {
		result.Reason = "The 24-hour service window is closed; use an approved template"
	}
	return result, nil
}
