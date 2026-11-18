package runtime

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type JourneyExecutionLease struct {
	SessionID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	Owner          uuid.UUID `gorm:"type:uuid;not null"`
	ExpiresAt      time.Time
}

func (JourneyExecutionLease) TableName() string { return "journey_execution_leases" }

// A DB lease serializes distinct inbound events. Turn/action receipts provide
// durability when a worker crashes after committing a commerce mutation.
func (s *Service) acquireJourney(ctx context.Context, session *ConversationSession) (func(), error) {
	owner := uuid.New()
	now := s.now()
	lease := JourneyExecutionLease{SessionID: session.ID, OrganizationID: session.OrganizationID, Owner: owner, ExpiresAt: now.Add(5 * time.Minute)}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "session_id"}}, DoUpdates: clause.Assignments(map[string]any{"owner": owner, "expires_at": lease.ExpiresAt}), Where: clause.Where{Exprs: []clause.Expression{clause.Lt{Column: clause.Column{Table: "journey_execution_leases", Name: "expires_at"}, Value: now}}}}).Create(&lease)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, runtimeError(ErrSessionConflict, "Another response is being processed. Please try again.")
	}
	release := func() {
		s.db.WithContext(context.WithoutCancel(ctx)).Where("organization_id = ? AND session_id = ? AND owner = ?", session.OrganizationID, session.ID, owner).Delete(&JourneyExecutionLease{})
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", session.OrganizationID, session.ID).First(session).Error; err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (s *Service) executeConfiguredJourney(ctx context.Context, config bot.VersionConfiguration, session *ConversationSession, input InboundMessage) (RuntimeResult, error) {
	system := parseJSONMap(session.SystemContext)
	mode := stringValue(system["journey_mode"])
	settings := bot.JourneySettings(config.Version)
	if !input.SimulatorStart && session.Status == SessionActive && system["journey_preview"] != true && input.Metadata["journey_flow"] != true {
		intent := strings.ToLower(strings.TrimSpace(input.Text))
		if intent != "" && intent != settings.Trigger && len(intent) <= 48 && !strings.ContainsAny(intent, " \t\n") {
			var binding bot.JourneyBinding
			err := s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND trigger = ? AND paused = ?", session.OrganizationID, session.ChannelID, intent, false).First(&binding).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return RuntimeResult{}, err
			}
			if err == nil && binding.BotID != session.BotID {
				vars := parseJSONMap(session.Variables)
				blocked, err := s.checkoutBlocksJourneySwitch(ctx, *session, vars)
				if err != nil {
					return RuntimeResult{}, err
				}
				if blocked {
					return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageText, Text: "Your checkout is in progress. Finish it, send “cancel order” to cancel an unpaid checkout, or contact the team."}}}, nil
				}
				target, err := s.selectJourneyEntry(ctx, session, intent, system)
				if err != nil {
					return RuntimeResult{}, err
				}
				session.Variables = "{}"
				session.StoreID = nil
				session.SystemContext = jsonValue(system)
				expiry := s.now().Add(time.Duration(bot.JourneySettings(target.Version).TTLMinutes) * time.Minute)
				session.ExpiresAt = &expiry
				return s.executeConfiguredJourney(ctx, *target, session, InboundMessage{SimulatorStart: true})
			}
		}
		if session.ExpectedInput == "" && isResetText(input.Text) && (mode == "ai" || mode == "guided_questions" || mode == "automated_commerce") {
			welcome := "Welcome! How can we help?"
			for _, step := range config.Steps {
				if step.StepKey == config.Version.StartStepKey && step.Message != "" {
					welcome = step.Message
					break
				}
			}
			return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageButtons, Text: welcome, Options: []MessageOption{{ID: settings.Trigger, Label: bot.JourneyEntryLabel(mode, settings.EntryCTA, settings.Trigger)}}}}}, nil
		}
	}
	if mode == "ai" && session.ExpectedInput == "" && (input.SimulatorStart || isResetText(input.Text) || strings.EqualFold(strings.TrimSpace(input.Text), bot.JourneySettings(config.Version).Trigger)) {
		welcome := "Welcome! Ask about our products, prices, availability, stores, orders or policies."
		for _, step := range config.Steps {
			if step.StepKey == config.Version.StartStepKey && strings.TrimSpace(step.Message) != "" {
				welcome = step.Message
				break
			}
		}
		return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageText, Text: welcome}}}, nil
	}
	resumeAI := session.Status == SessionActive &&
		session.HandoffState == "released" &&
		session.CurrentStepKey == config.Version.StartStepKey &&
		!input.SimulatorStart &&
		classifyCommand(input.Text) == commandNone &&
		journeyEntryIntent(input.Text) == "default"
	if (mode == "ai" || resumeAI) && session.ExpectedInput == "" && s.aiInbound != nil {
		response, err := s.aiInbound(ctx, *session, input.Text)
		if err == nil && strings.TrimSpace(response.Reply) != "" {
			if strings.TrimSpace(response.Variables) != "" {
				session.Variables = response.Variables
			}
			return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageText, Text: strings.TrimSpace(response.Reply)}}}, nil
		}
		if resumeAI || mode == "ai" {
			return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{
				Type: MessageText,
				Text: "I couldn't answer that right now. Please try again, or send menu to continue.",
			}}}, nil
		}
	}
	result, err := s.executeJourney(ctx, config, session, input)
	if err != nil {
		return result, err
	}
	system = parseJSONMap(session.SystemContext)
	if mode == "whatsapp_flow" && input.Metadata["journey_flow"] != true && system["journey_flow_launched"] != true && session.ExpectedInput != "" {
		message, err := s.launchJourneyFlow(ctx, *session, bot.JourneyEntryLabel(mode, bot.JourneySettings(config.Version).EntryCTA, ""))
		if err != nil {
			result.Messages = append([]OutboundMessage{{Type: MessageText, Text: "The form is unavailable. We can continue safely in this chat."}}, result.Messages...)
			return result, nil
		}
		system["journey_flow_launched"] = true
		session.SystemContext = jsonValue(system)
		result.Messages = []OutboundMessage{message}
	}
	return result, nil
}

func (s *Service) checkoutBlocksJourneySwitch(ctx context.Context, session ConversationSession, vars map[string]any) (bool, error) {
	reference := stringValue(vars["order_id"])
	if reference == "" {
		return stringValue(vars["cart_id"]) != "", nil
	}
	orderID, err := uuid.Parse(reference)
	if err != nil || session.CustomerID == nil {
		return true, nil
	}
	var order core.Order
	err = s.db.WithContext(ctx).Select("status").Where("id = ? AND organization_id = ? AND customer_id = ?", orderID, session.OrganizationID, *session.CustomerID).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	switch order.Status {
	case core.OrderCompleted, core.OrderCancelled, core.OrderRefunded:
		return false, nil
	}
	return true, nil
}
