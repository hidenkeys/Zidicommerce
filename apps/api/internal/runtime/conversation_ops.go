package runtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var activeHandoffStatuses = []string{"open", "assigned", "reopened"}

func (s *Service) MarkConversationRead(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) (ConversationSession, error) {
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return ConversationSession{}, err
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&session).Updates(map[string]any{"unread_count": 0, "last_read_at": &now, "updated_at": now}).Error; err != nil {
		return ConversationSession{}, err
	}
	s.recordEvent(ctx, session, EventConversationRead, "info", "", "", map[string]any{"actor_user_id": actor.ID.String()})
	_ = s.audit(ctx, actor, "conversation_session", session.ID, "conversation_marked_read", map[string]any{})
	return s.GetConversation(ctx, actor, sessionID)
}

func (s *Service) MarkConversationUnread(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) (ConversationSession, error) {
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return ConversationSession{}, err
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&session).Updates(map[string]any{"unread_count": 1, "updated_at": now}).Error; err != nil {
		return ConversationSession{}, err
	}
	s.recordEvent(ctx, session, EventConversationUnread, "info", "", "", map[string]any{"actor_user_id": actor.ID.String()})
	_ = s.audit(ctx, actor, "conversation_session", session.ID, "conversation_marked_unread", map[string]any{})
	return s.GetConversation(ctx, actor, sessionID)
}

func (s *Service) UpdateConversationStatus(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, input ConversationStatusInput) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return ConversationSession{}, httperror.Forbidden("You cannot update conversations")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if !validConversationStatus(status) {
		return ConversationSession{}, httperror.BadRequest("Conversation status is not valid")
	}
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return ConversationSession{}, err
	}
	now := s.now()
	updates := map[string]any{"conversation_status": status, "updated_at": now}
	if status == ConversationResolved {
		updates["status"] = SessionCompleted
		updates["resolved_at"] = &now
	}
	if status == ConversationAIHandling || status == ConversationOpen || status == ConversationReopened {
		updates["status"] = SessionActive
		updates["resolved_at"] = nil
	}
	if err := s.db.WithContext(ctx).Model(&session).Updates(updates).Error; err != nil {
		return ConversationSession{}, err
	}
	_ = s.audit(ctx, actor, "conversation_session", session.ID, "conversation_status_updated", map[string]any{"previous_status": session.ConversationStatus, "new_status": status, "reason": strings.TrimSpace(input.Reason)})
	return s.GetConversation(ctx, actor, sessionID)
}

func (s *Service) AssignConversation(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, input ConversationAssignInput) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return ConversationSession{}, httperror.Forbidden("You cannot assign conversations")
	}
	if input.UserID == uuid.Nil {
		return ConversationSession{}, httperror.BadRequest("Assignee is required")
	}
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return ConversationSession{}, err
	}
	if err := s.ensureAssignableUser(ctx, actor.OrganizationID, input.UserID); err != nil {
		return ConversationSession{}, err
	}
	handoff, err := s.requestHandoffForSession(ctx, actor, session, strings.TrimSpace(input.Reason), "normal")
	if err != nil {
		return ConversationSession{}, err
	}
	if _, err := s.AssignSupportHandoff(ctx, actor, handoff.ID, SupportHandoffAssignInput{UserID: input.UserID, Reason: input.Reason}); err != nil {
		return ConversationSession{}, err
	}
	return s.GetConversation(ctx, actor, sessionID)
}

func (s *Service) UnassignConversation(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, input SupportHandoffReleaseInput) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return ConversationSession{}, httperror.Forbidden("You cannot release conversations")
	}
	handoff, err := s.activeHandoffForSession(ctx, actor, sessionID)
	if err != nil {
		return ConversationSession{}, err
	}
	if _, err := s.ReleaseSupportHandoff(ctx, actor, handoff.ID, input); err != nil {
		return ConversationSession{}, err
	}
	return s.GetConversation(ctx, actor, sessionID)
}

func (s *Service) ResolveConversation(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, input SupportHandoffResolveInput) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return ConversationSession{}, httperror.Forbidden("You cannot resolve conversations")
	}
	handoff, err := s.activeHandoffForSession(ctx, actor, sessionID)
	if err == nil {
		if _, err := s.ResolveSupportHandoff(ctx, actor, handoff.ID, input); err != nil {
			return ConversationSession{}, err
		}
		return s.GetConversation(ctx, actor, sessionID)
	}
	return s.UpdateConversationStatus(ctx, actor, sessionID, ConversationStatusInput{Status: ConversationResolved, Reason: input.ResolutionNote})
}

func (s *Service) ReopenConversation(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, reason string) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return ConversationSession{}, httperror.Forbidden("You cannot reopen conversations")
	}
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ?", actor.OrganizationID, sessionID).Order("updated_at DESC").First(&handoff).Error
	if err == nil {
		if _, err := s.ReopenSupportHandoff(ctx, actor, handoff.ID); err != nil {
			return ConversationSession{}, err
		}
		return s.GetConversation(ctx, actor, sessionID)
	}
	if err != gorm.ErrRecordNotFound {
		return ConversationSession{}, err
	}
	return s.UpdateConversationStatus(ctx, actor, sessionID, ConversationStatusInput{Status: ConversationReopened, Reason: reason})
}

func (s *Service) RequestSupportHandoff(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, reason string) (SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoff{}, httperror.Forbidden("You cannot request support handoffs")
	}
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return SupportHandoff{}, err
	}
	return s.requestHandoffForSession(ctx, actor, session, reason, "normal")
}

func (s *Service) AssignSupportHandoff(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID, input SupportHandoffAssignInput) (SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoff{}, httperror.Forbidden("You cannot assign support handoffs")
	}
	if input.UserID == uuid.Nil {
		return SupportHandoff{}, httperror.BadRequest("Assignee is required")
	}
	if err := s.ensureAssignableUser(ctx, actor.OrganizationID, input.UserID); err != nil {
		return SupportHandoff{}, err
	}
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
		query = s.scopeHandoffQuery(query, actor)
		if err := query.First(&handoff).Error; err != nil {
			return mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
		}
		if handoff.Status == "resolved" || handoff.Status == "released" {
			return httperror.BadRequest("Closed handoffs cannot be assigned")
		}
		previous := handoff.AssignedUserID
		now := s.now()
		if err := tx.Model(&handoff).Updates(map[string]any{"status": "assigned", "assigned_user_id": input.UserID, "released_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).Updates(map[string]any{"status": SessionHandoff, "conversation_status": ConversationHumanAssigned, "assigned_user_id": input.UserID, "handoff_state": "assigned", "human_owned_at": &now, "human_released_at": nil, "resolved_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if strings.TrimSpace(input.Note) != "" {
			note := SupportHandoffNote{ID: uuid.New(), OrganizationID: actor.OrganizationID, HandoffID: handoffID, ActorUserID: actor.ID, Note: strings.TrimSpace(input.Note), Internal: true}
			if err := tx.Create(&note).Error; err != nil {
				return err
			}
		}
		if err := s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_assigned", map[string]any{"previous_assigned_user_id": uuidString(previous), "new_assigned_user_id": input.UserID.String(), "reason": strings.TrimSpace(input.Reason)}); err != nil {
			return err
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID).First(&handoff).Error
	})
	if err == nil {
		s.recordEvent(ctx, ConversationSession{ID: handoff.SessionID, OrganizationID: actor.OrganizationID}, EventHandoffAssigned, "info", "", "", map[string]any{"assigned_user_id": input.UserID.String()})
	}
	return handoff, err
}

func (s *Service) ReleaseSupportHandoff(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID, input SupportHandoffReleaseInput) (SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoff{}, httperror.Forbidden("You cannot release support handoffs")
	}
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
		query = s.scopeHandoffQuery(query, actor)
		if err := query.First(&handoff).Error; err != nil {
			return mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
		}
		if handoff.AssignedUserID != nil && actor.Role == authz.SupportAgent && *handoff.AssignedUserID != actor.ID {
			return httperror.Forbidden("You cannot release another agent's handoff")
		}
		if handoff.Status == "resolved" {
			return httperror.BadRequest("Resolved handoffs cannot be released")
		}
		now := s.now()
		if err := tx.Model(&handoff).Updates(map[string]any{"status": "released", "assigned_user_id": nil, "released_at": &now, "updated_at": now}).Error; err != nil {
			return err
		}
		sessionStatus := SessionHandoff
		conversationStatus := ConversationHumanRequested
		sessionUpdates := map[string]any{"status": sessionStatus, "conversation_status": conversationStatus, "assigned_user_id": nil, "handoff_state": "released", "human_released_at": &now, "updated_at": now}
		if input.ResumeBot {
			sessionStatus = SessionActive
			conversationStatus = ConversationAIHandling
			sessionUpdates["status"] = sessionStatus
			sessionUpdates["conversation_status"] = conversationStatus
			if startStepKey := s.startStepKeyForHandoffTx(tx, actor.OrganizationID, handoff.SessionID); startStepKey != "" {
				sessionUpdates["current_step_key"] = startStepKey
				sessionUpdates["expected_input"] = ""
			}
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).Updates(sessionUpdates).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_released", map[string]any{"resume_bot": input.ResumeBot, "reason": strings.TrimSpace(input.Reason)}); err != nil {
			return err
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID).First(&handoff).Error
	})
	if err == nil {
		s.recordEvent(ctx, ConversationSession{ID: handoff.SessionID, OrganizationID: actor.OrganizationID}, EventHandoffReleased, "info", "", "", map[string]any{"resume_bot": input.ResumeBot})
	}
	return handoff, err
}

func (s *Service) activeHandoffForSession(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) (SupportHandoff, error) {
	var handoff SupportHandoff
	query := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ? AND status IN ?", actor.OrganizationID, sessionID, activeHandoffStatuses).Order("updated_at DESC")
	query = s.scopeHandoffQuery(query, actor)
	if err := query.First(&handoff).Error; err != nil {
		return SupportHandoff{}, mapNotFoundCode(err, ErrSessionNotFound, "Active support handoff not found")
	}
	return handoff, nil
}

func (s *Service) startStepKeyForHandoffTx(tx *gorm.DB, organizationID uuid.UUID, sessionID uuid.UUID) string {
	var session ConversationSession
	if err := tx.Select("bot_version_id").Where("organization_id = ? AND id = ?", organizationID, sessionID).First(&session).Error; err != nil {
		return ""
	}
	var version bot.BotVersion
	if err := tx.Select("start_step_key").Where("organization_id = ? AND id = ?", organizationID, session.BotVersionID).First(&version).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(version.StartStepKey)
}

func (s *Service) applyActiveHandoffState(ctx context.Context, session *ConversationSession) {
	if session == nil || session.ID == uuid.Nil {
		return
	}
	var handoff SupportHandoff
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ? AND status IN ?", session.OrganizationID, session.ID, activeHandoffStatuses).Order("updated_at DESC").First(&handoff).Error; err != nil {
		return
	}
	session.HandoffState = handoff.Status
	session.Priority = normalizePriority(handoff.Priority)
	if handoff.AssignedUserID != nil {
		session.AssignedUserID = handoff.AssignedUserID
		session.ConversationStatus = ConversationHumanAssigned
		if session.HumanOwnedAt == nil {
			now := s.now()
			session.HumanOwnedAt = &now
		}
		return
	}
	if session.ConversationStatus == "" || session.ConversationStatus == ConversationAIHandling {
		session.ConversationStatus = ConversationHumanRequested
	}
}

func (s *Service) requestHandoffForSession(ctx context.Context, actor auth.CurrentUser, session ConversationSession, reason string, priority string) (SupportHandoff, error) {
	priority = normalizePriority(priority)
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND session_id = ? AND status IN ?", session.OrganizationID, session.ID, activeHandoffStatuses).Order("updated_at DESC").First(&handoff).Error
		if err == nil {
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		now := s.now()
		handoff = SupportHandoff{ID: uuid.New(), OrganizationID: session.OrganizationID, SessionID: session.ID, CustomerID: session.CustomerID, Status: "open", Reason: strings.TrimSpace(reason), Priority: priority, Metadata: jsonMap(map[string]any{"channel_id": session.ChannelID.String(), "external_conversation_id": session.ExternalConversationID}), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&handoff).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", session.OrganizationID, session.ID).Updates(map[string]any{"status": SessionHandoff, "conversation_status": ConversationHumanRequested, "handoff_state": "open", "priority": priority, "updated_at": now}).Error; err != nil {
			return err
		}
		return s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_requested", map[string]any{"conversation_id": session.ID.String(), "reason": strings.TrimSpace(reason)})
	})
	return handoff, err
}

func (s *Service) ensureAssignableUser(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID) error {
	var user organization.User
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", userID, "active").First(&user).Error; err != nil {
		return mapNotFoundCode(err, ErrSessionNotFound, "Assignee not found")
	}
	if user.OrganizationID != nil && *user.OrganizationID == organizationID {
		if !user.Role.HasPermission(authz.PermissionConversationsManage) {
			return httperror.Forbidden("Assignee cannot manage conversations")
		}
		return nil
	}
	var membership organization.OrganizationMembership
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ? AND status = ?", organizationID, userID, "active").First(&membership).Error; err != nil {
		return mapNotFoundCode(err, ErrSessionNotFound, "Assignee not found")
	}
	if !membership.Role.HasPermission(authz.PermissionConversationsManage) {
		return httperror.Forbidden("Assignee cannot manage conversations")
	}
	return nil
}

func (s *Service) audit(ctx context.Context, actor auth.CurrentUser, targetType string, targetID uuid.UUID, action string, metadata map[string]any) error {
	return s.auditTx(s.db.WithContext(ctx), actor, targetType, targetID, action, metadata)
}

func (s *Service) auditTx(tx *gorm.DB, actor auth.CurrentUser, targetType string, targetID uuid.UUID, action string, metadata map[string]any) error {
	orgID := actor.OrganizationID
	actorID := actor.ID
	now := s.now()
	log := organization.AuditLog{ID: uuid.New(), OrganizationID: &orgID, ActorUserID: &actorID, TargetType: targetType, TargetID: &targetID, Action: action, Metadata: jsonValue(metadata), CreatedAt: now}
	return tx.Create(&log).Error
}

func validConversationStatus(status string) bool {
	switch status {
	case ConversationOpen, ConversationAIHandling, ConversationHumanRequested, ConversationHumanAssigned, ConversationPending, ConversationWaiting, ConversationResolved, ConversationReopened:
		return true
	default:
		return false
	}
}

func normalizePriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "low", "high", "urgent":
		return strings.ToLower(strings.TrimSpace(priority))
	default:
		return "normal"
	}
}

func uuidString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func storeIDFromVariables(raw string) *uuid.UUID {
	values := parseJSONMap(raw)
	for _, key := range []string{"store_id", "store.id"} {
		if rawValue, ok := lookupPath(values, key); ok {
			if id := uuidFromAny(rawValue); id != nil {
				return id
			}
		}
		if id := uuidFromAny(values[key]); id != nil {
			return id
		}
	}
	return nil
}

func conversationStatusForRuntime(session ConversationSession) string {
	if session.ConversationStatus != "" {
		return session.ConversationStatus
	}
	switch session.Status {
	case SessionHandoff:
		return ConversationHumanRequested
	case SessionCompleted:
		return ConversationResolved
	case SessionActive:
		return ConversationAIHandling
	default:
		return ConversationOpen
	}
}

func sessionPriority(session ConversationSession) string {
	if session.Priority != "" {
		return session.Priority
	}
	return "normal"
}

func (s *Service) conversationStore(ctx context.Context, orgID uuid.UUID, storeID *uuid.UUID) (string, error) {
	if storeID == nil {
		return "", nil
	}
	var store core.Store
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, *storeID).First(&store).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return store.Name, nil
}

func uuidFromMetadataOrZero(raw string, key string) uuid.UUID {
	values := parseJSONMap(raw)
	parsed, err := uuid.Parse(strings.TrimSpace(stringValue(values[key])))
	if err != nil {
		return uuid.Nil
	}
	return parsed
}
