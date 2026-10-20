package runtime

import (
	"context"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"gorm.io/gorm/clause"
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
	if mode == "ai" && session.ExpectedInput == "" && s.aiInbound != nil {
		response, err := s.aiInbound(ctx, *session, input.Text)
		if err == nil && response.Reply != "" {
			return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageText, Text: response.Reply}}}, nil
		}
	}
	result, err := s.executeJourney(ctx, config, session, input)
	if err != nil {
		return result, err
	}
	system = parseJSONMap(session.SystemContext)
	if mode == "whatsapp_flow" && input.Metadata["journey_flow"] != true && system["journey_flow_launched"] != true && session.ExpectedInput != "" {
		message, err := s.launchJourneyFlow(ctx, *session)
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
