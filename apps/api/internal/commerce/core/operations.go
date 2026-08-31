package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"gorm.io/gorm"
)

func (s *Service) GetOrderOperations(ctx context.Context, actor auth.CurrentUser, orderID uuid.UUID) (OrderOperationsView, error) {
	order, err := s.GetOrder(ctx, actor, orderID)
	if err != nil {
		return OrderOperationsView{}, err
	}
	view := OrderOperationsView{Order: order, Events: []CommerceEvent{}}

	var payment Payment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, order.ID).Order("created_at DESC").First(&payment).Error; err == nil {
		view.Payment = &payment
	} else if err != gorm.ErrRecordNotFound {
		return OrderOperationsView{}, err
	}

	var fulfilment Fulfilment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, order.ID).First(&fulfilment).Error; err == nil {
		view.Fulfilment = &fulfilment
	} else if err != gorm.ErrRecordNotFound {
		return OrderOperationsView{}, err
	}

	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, order.ID).Order("created_at ASC").Find(&view.Events).Error; err != nil {
		return OrderOperationsView{}, err
	}

	var link ConversationOrderLink
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, order.ID).Order("created_at DESC").First(&link).Error; err == nil {
		view.ConversationID = &link.ConversationSessionID
	} else if err != gorm.ErrRecordNotFound {
		return OrderOperationsView{}, err
	}

	view.NextActions = nextOrderOperationalActions(order, view.Fulfilment)
	if !actor.Role.HasPermission(authz.PermissionOrdersManage) {
		view.NextActions = informationalOrderActions(view.NextActions)
	}
	return view, nil
}

func informationalOrderActions(actions []OrderOperationalAction) []OrderOperationalAction {
	filtered := make([]OrderOperationalAction, 0, len(actions))
	for _, action := range actions {
		if action.TargetStatus == "" {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func nextOrderOperationalActions(order Order, fulfilment *Fulfilment) []OrderOperationalAction {
	switch order.Status {
	case OrderAwaitingPayment:
		return []OrderOperationalAction{{Key: "wait_for_payment", Label: "Waiting for verified payment", ResourceType: "payment"}}
	case OrderPaid:
		return []OrderOperationalAction{{Key: "start_preparing", Label: "Start preparing", ResourceType: "order", TargetStatus: OrderProcessing}}
	case OrderProcessing:
		return []OrderOperationalAction{{Key: "mark_ready", Label: "Mark ready", ResourceType: "order", TargetStatus: OrderReady}}
	case OrderReady:
		if order.FulfilmentType == FulfilmentMerchantRider {
			return []OrderOperationalAction{{Key: "out_for_delivery", Label: "Hand to rider", ResourceType: "order", TargetStatus: OrderOutForDelivery}}
		}
		label := "Mark collected"
		if fulfilment != nil && fulfilment.Type == FulfilmentCustomerRider {
			label = "Mark handed to customer rider"
		}
		return []OrderOperationalAction{{Key: "complete", Label: label, ResourceType: "order", TargetStatus: OrderCompleted}}
	case OrderOutForDelivery:
		return []OrderOperationalAction{{Key: "complete", Label: "Mark delivered", ResourceType: "order", TargetStatus: OrderCompleted}}
	default:
		return []OrderOperationalAction{}
	}
}
