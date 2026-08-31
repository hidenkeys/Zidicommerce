package whatsapp

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"gorm.io/gorm"
)

func (s *Service) ApplyDeliveryUpdate(ctx context.Context, configuration Configuration, update channelplatform.DeliveryUpdate) (bool, error) {
	providerMessageID := strings.TrimSpace(update.ProviderMessageID)
	if providerMessageID == "" {
		return false, nil
	}
	status := strings.ToLower(strings.TrimSpace(update.Status))
	var sentAt *time.Time
	if status == "delivered" {
		var delivery struct{ SentAt *time.Time }
		if err := s.db.WithContext(ctx).Table("channel_outbound_messages").Select("sent_at").Where("organization_id = ? AND channel_id = ? AND provider_message_id = ?", configuration.OrganizationID, configuration.ConnectionID, providerMessageID).Scan(&delivery).Error; err == nil {
			sentAt = delivery.SentAt
		}
	}
	updates := map[string]any{"updated_at": s.now()}
	switch status {
	case "sent":
		updates["status"] = "sent"
		updates["sent_at"] = update.OccurredAt
	case "delivered":
		updates["status"] = "delivered"
		updates["delivered_at"] = update.OccurredAt
	case "read":
		updates["status"] = "read"
		updates["read_at"] = update.OccurredAt
	case "failed":
		updates["status"] = "failed_permanently"
		updates["error_message"] = deliveryError(update)
	default:
		return false, nil
	}
	result := s.db.WithContext(ctx).Table("channel_outbound_messages").Where("organization_id = ? AND channel_id = ? AND provider_message_id = ?", configuration.OrganizationID, configuration.ConnectionID, providerMessageID).Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	metric := channelplatform.MetricInput{OrganizationID: configuration.OrganizationID, ConnectionID: configuration.ConnectionID}
	switch status {
	case "delivered":
		metric.DeliveredCount = 1
	case "read":
		metric.ReadCount = 1
	case "failed":
		metric.ProviderErrorCount = 1
	}
	if metric.DeliveredCount != 0 || metric.ReadCount != 0 || metric.ProviderErrorCount != 0 {
		if _, err := s.platform.IncrementMetric(ctx, metric); err != nil {
			return false, err
		}
	}
	if status == "delivered" && sentAt != nil && update.OccurredAt.After(*sentAt) {
		_ = s.incrementOperationalMetric(ctx, configuration.OrganizationID, configuration.ConnectionID, OperationalMetricDaily{ProviderDeliveryLatencyMS: update.OccurredAt.Sub(*sentAt).Milliseconds(), ProviderDeliverySamples: 1})
	}
	if err := audit(s.db.WithContext(ctx), &configuration.OrganizationID, nil, configuration.ConnectionID, "whatsapp_delivery_status_updated", map[string]any{"provider_message_id": providerMessageID, "status": status, "error_code": update.ErrorCode}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) MarkProviderEventResult(ctx context.Context, eventID uuid.UUID, organizationID uuid.UUID, status, processingError string) error {
	updates := map[string]any{"normalized_status": strings.ToLower(strings.TrimSpace(status)), "processing_error": strings.TrimSpace(processingError), "updated_at": s.now()}
	result := s.db.WithContext(ctx).Model(&channelplatform.ProviderEvent{}).Where("id = ? AND organization_id = ?", eventID, organizationID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) ClaimProviderEventRetry(ctx context.Context, eventID uuid.UUID, organizationID uuid.UUID) (bool, error) {
	result := s.db.WithContext(ctx).Model(&channelplatform.ProviderEvent{}).Where("id = ? AND organization_id = ? AND normalized_status = ?", eventID, organizationID, "failed").Updates(map[string]any{"normalized_status": "processing", "processing_error": "", "updated_at": s.now()})
	return result.RowsAffected == 1, result.Error
}

func (s *Service) AuditProviderActivity(ctx context.Context, configuration Configuration, action string, metadata map[string]any) error {
	return audit(s.db.WithContext(ctx), &configuration.OrganizationID, nil, configuration.ConnectionID, action, metadata)
}

func deliveryError(update channelplatform.DeliveryUpdate) string {
	if strings.TrimSpace(update.ErrorCode) != "" {
		return "WhatsApp delivery failed (" + strings.TrimSpace(update.ErrorCode) + ")"
	}
	return "WhatsApp delivery failed"
}

func statusEventID(update channelplatform.DeliveryUpdate) string {
	return strings.Join([]string{update.ProviderMessageID, strings.ToLower(strings.TrimSpace(update.Status))}, ":")
}
