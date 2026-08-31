package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
)

func (s *Service) OnPaymentPaid(ctx context.Context, organizationID, orderID uuid.UUID, _ string) error {
	if organizationID == uuid.Nil || orderID == uuid.Nil {
		return nil
	}
	var links []core.ConversationOrderLink
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", organizationID, orderID).Find(&links).Error; err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}
	config, err := bot.LoadEffectiveCommerceWorkflowConfiguration(ctx, s.db, organizationID)
	if err != nil {
		return err
	}
	postPaymentSteps := config.PostPaymentStepValues()
	for _, link := range links {
		var session ConversationSession
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, link.ConversationSessionID).First(&session).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return err
		}
		variables := parseJSONMap(session.Variables)
		variables["order_id"] = orderID.String()
		variables["order_status"] = core.OrderPaid
		variables["payment_status"] = core.PaymentPaid
		system := parseJSONMap(session.SystemContext)
		system["post_payment_steps"] = postPaymentSteps
		if len(postPaymentSteps) > 0 {
			system["next_operation"] = postPaymentSteps[0]
		}
		updates := map[string]any{"variables": jsonMap(variables), "system_context": jsonMap(system), "updated_at": s.now()}
		if !session.AIShouldPause() && session.ConversationStatus == ConversationWaiting {
			updates["conversation_status"] = ConversationAIHandling
		}
		if err := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", organizationID, session.ID).Updates(updates).Error; err != nil {
			return err
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&RuntimeEvent{}).Where("organization_id = ? AND session_id = ? AND event_type = ? AND action_key = ?", organizationID, session.ID, EventCommercePaymentPaid, orderID.String()).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			s.recordEvent(ctx, session, EventCommercePaymentPaid, "info", session.CurrentStepKey, orderID.String(), map[string]any{"order_id": orderID.String(), "post_payment_steps": postPaymentSteps})
		}
	}
	return nil
}

func (s *Service) attachConversationCommerceContext(ctx context.Context, actor auth.CurrentUser, summary *ConversationSummary) error {
	var link core.ConversationOrderLink
	err := s.db.WithContext(ctx).Where("organization_id = ? AND conversation_session_id = ?", actor.OrganizationID, summary.ID).Order("created_at DESC").First(&link).Error
	if err == gorm.ErrRecordNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	var order core.Order
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, link.OrderID).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	summary.OrderID = &order.ID
	summary.OrderNumber = order.OrderNumber
	summary.OrderStatus = order.Status
	summary.FulfilmentType = order.FulfilmentType
	if summary.StoreID == nil {
		summary.StoreID = &order.StoreID
	}
	if summary.StoreName == "" {
		storeName, err := s.conversationStore(ctx, actor.OrganizationID, summary.StoreID)
		if err != nil {
			return err
		}
		summary.StoreName = storeName
	}
	var payment core.Payment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, order.ID).Order("created_at DESC").First(&payment).Error; err == nil {
		summary.PaymentStatus = payment.Status
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	var fulfilment core.Fulfilment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, order.ID).First(&fulfilment).Error; err == nil {
		summary.FulfilmentStatus = fulfilment.Status
		if summary.FulfilmentType == "" {
			summary.FulfilmentType = fulfilment.Type
		}
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	return nil
}
