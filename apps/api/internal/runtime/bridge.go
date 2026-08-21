package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
)

type HandoffInboundHandler func(ctx context.Context, organizationID, sessionID uuid.UUID, text string) (handled bool, reply string, err error)

func (s *Service) RegisterAction(key string, handler ActionHandler) {
	if s.actions == nil {
		s.actions = NewActionRegistry(s.db, s.commerce)
	}
	s.actions.Register(key, handler)
}

func (s *Service) ConfigureHandoffInbound(handler HandoffInboundHandler) {
	s.handoffInbound = handler
}

func (s *Service) SendText(ctx context.Context, organizationID uuid.UUID, recipient, text string) error {
	recipient = strings.TrimSpace(recipient)
	text = strings.TrimSpace(text)
	if recipient == "" || text == "" {
		return nil
	}
	// Prefer WhatsApp, but fall back to any active channel so a workspace running
	// on the test channel (a demo, a pilot before Meta approval) still records
	// what would have been sent instead of dropping it silently.
	var channel core.Channel
	err := s.db.WithContext(ctx).Where("organization_id = ? AND provider = ? AND status = ?", organizationID, "whatsapp", core.StatusActive).Order("updated_at DESC").First(&channel).Error
	if err != nil {
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND status = ?", organizationID, core.StatusActive).Order("updated_at DESC").First(&channel).Error; err != nil {
			s.log.Info("skipping outbound text; no active channel", "organization_id", organizationID)
			return nil
		}
	}
	inbound := InboundMessage{ChannelID: &channel.ID, ExternalConversationID: recipient, Sender: recipient, ExternalMessageID: "notify-" + uuid.NewString()}
	result := RuntimeResult{Messages: []OutboundMessage{{Type: MessageText, Text: text}}}
	_, err = s.DispatchOutbound(ctx, channel, inbound, result)
	return err
}

// AssignHandoff routes a conversation to a named person. A conversation that
// already reached the bot's handoff step has an open handoff, so this claims
// that one rather than opening a second: only one handoff may be open per
// session.
func (s *Service) AssignHandoff(ctx context.Context, organizationID, sessionID, customerID, assignedUserID, requestID uuid.UUID) (uuid.UUID, error) {
	now := s.now()
	metadata := fmt.Sprintf(`{"request_id":%q}`, requestID)
	var existing SupportHandoff
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND session_id = ? AND status IN ?", organizationID, sessionID, []string{"open", "assigned"}).
		First(&existing).Error
	switch {
	case err == nil:
		updates := map[string]any{"status": "assigned", "assigned_user_id": optionalUUID(assignedUserID), "reason": "service_assignment", "metadata": metadata, "updated_at": now}
		if existing.CustomerID == nil && customerID != uuid.Nil {
			updates["customer_id"] = customerID
		}
		if err := s.db.WithContext(ctx).Model(&SupportHandoff{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return uuid.Nil, err
		}
		if err := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", organizationID, sessionID).Updates(map[string]any{"status": SessionHandoff, "updated_at": now}).Error; err != nil {
			return uuid.Nil, err
		}
		return existing.ID, nil
	case err != gorm.ErrRecordNotFound:
		return uuid.Nil, err
	}
	handoff := SupportHandoff{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		SessionID:      sessionID,
		CustomerID:     optionalUUID(customerID),
		AssignedUserID: optionalUUID(assignedUserID),
		Status:         "assigned",
		Reason:         "service_assignment",
		Metadata:       metadata,
	}
	if err := s.db.WithContext(ctx).Create(&handoff).Error; err != nil {
		return uuid.Nil, err
	}
	if err := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", organizationID, sessionID).Updates(map[string]any{"status": SessionHandoff, "updated_at": now}).Error; err != nil {
		return uuid.Nil, err
	}
	return handoff.ID, nil
}

func optionalUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
