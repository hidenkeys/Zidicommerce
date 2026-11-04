package runtime

import (
	"context"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

// Explicit test-record execution never uses a real channel sender or payment
// provider. Its actor/tenant ownership is persisted with the conversation.
func (s *Service) StartControlledJourneyTest(ctx context.Context, actor auth.CurrentUser, versionID uuid.UUID, acknowledged bool) (JourneyPreviewResult, error) {
	if !actor.Role.HasPermission(authz.PermissionBotPublish) || !actor.Role.HasPermission(authz.PermissionAIUse) {
		return JourneyPreviewResult{}, httperror.Forbidden("You cannot create journey test records")
	}
	if !acknowledged {
		return JourneyPreviewResult{}, httperror.BadRequest("Explicitly acknowledge test record creation")
	}
	config, err := bot.NewService(s.db).GetConfiguration(ctx, actor, versionID)
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	d := bot.JourneySettings(config.Version)
	if !bot.IsJourney(config.Version) || config.Version.PublishedAt == nil || d.ConnectionID == nil {
		return JourneyPreviewResult{}, httperror.BadRequest("Use a published journey bound to a test channel")
	}
	channel, err := s.getChannelForActor(ctx, actor, *d.ConnectionID)
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	if channel.Provider != "test" {
		return JourneyPreviewResult{}, httperror.BadRequest("Test records are restricted to the local test channel; no WhatsApp or payment provider will be contacted")
	}
	test := NewService(s.db, core.NewService(s.db, core.SafeTestProvider{}), s.log)
	session, err := test.StartTestSession(ctx, actor, RuntimeStartInput{BotID: config.Version.BotID, ChannelID: channel.ID, Sender: "journey-test-" + uuid.NewString()})
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	// StartTestSession resolves the current published version; pin the requested
	// immutable revision, including a historical revision selected for testing.
	session.BotVersionID, session.CurrentStepKey = versionID, config.Version.StartStepKey
	system := parseJSONMap(session.SystemContext)
	system["journey_controlled_test"], system["journey_test_actor"], system["journey_mode"] = true, actor.ID.String(), "guided_chat"
	session.SystemContext = jsonValue(system)
	if err := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, session.ID).Updates(map[string]any{"bot_version_id": versionID, "current_step_key": session.CurrentStepKey, "system_context": session.SystemContext}).Error; err != nil {
		return JourneyPreviewResult{}, err
	}
	return test.AnswerControlledJourneyTest(ctx, actor, session.ID, JourneyAnswer{EventID: uuid.NewString(), Text: "start"})
}

func (s *Service) AnswerControlledJourneyTest(ctx context.Context, actor auth.CurrentUser, id uuid.UUID, answer JourneyAnswer) (JourneyPreviewResult, error) {
	if !actor.Role.HasPermission(authz.PermissionBotPublish) || !actor.Role.HasPermission(authz.PermissionAIUse) {
		return JourneyPreviewResult{}, httperror.Forbidden("You cannot create journey test records")
	}
	var session ConversationSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, id).First(&session).Error; err != nil {
		return JourneyPreviewResult{}, httperror.NotFound("Test session not found")
	}
	system := parseJSONMap(session.SystemContext)
	channel, err := s.getChannelForActor(ctx, actor, session.ChannelID)
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	if system["journey_controlled_test"] != true || system["journey_test_actor"] != actor.ID.String() || channel.Provider != "test" {
		return JourneyPreviewResult{}, httperror.Forbidden("This is not your controlled test session")
	}
	if len(answer.Text) > 4096 || answer.EventID == "" || len(answer.EventID) > 128 {
		return JourneyPreviewResult{}, httperror.BadRequest("A bounded event ID and answer are required")
	}
	test := NewService(s.db, core.NewService(s.db, core.SafeTestProvider{}), s.log)
	text := answer.Text
	if answer.Command != "" {
		text = answer.Command
	}
	result, err := test.ProcessTestMessage(ctx, actor, RuntimeMessageInput{SessionID: id, Text: text, ExternalMessageID: answer.EventID, Location: answer.Location, Metadata: map[string]any{"journey_turn": answer.Turn}})
	if err != nil {
		return JourneyPreviewResult{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, id).First(&session).Error; err != nil {
		return JourneyPreviewResult{}, err
	}
	out := journeyPreviewResult(JourneyPreviewSession{ID: id}, session, result)
	out.Controlled = true
	return out, nil
}
