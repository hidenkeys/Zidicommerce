package runtime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultSessionTTL = 24 * time.Hour

var runtimeUsableChannelStatuses = []string{core.StatusActive, "connected", "healthy", "degraded", "requires_attention"}

type Service struct {
	db              *gorm.DB
	commerce        *core.Service
	jobs            *jobs.Service
	actions         *ActionRegistry
	senders         map[string]ChannelSender
	log             *slog.Logger
	handoffInbound  HandoffInboundHandler
	aiInbound       AIInboundHandler
	lifecycleCancel LifecycleCancelHandler
	now             func() time.Time
}

func NewService(db *gorm.DB, commerce *core.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, commerce: commerce, actions: NewActionRegistry(db, commerce), senders: map[string]ChannelSender{}, log: logger, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ConfigureJobs(jobService *jobs.Service) {
	s.jobs = jobService
}

func (s *Service) ProcessMessage(ctx context.Context, input InboundMessage) (RuntimeResult, error) {
	start := s.now()
	if input.Timestamp.IsZero() {
		input.Timestamp = start
	}
	if input.ExternalMessageID == "" {
		input.ExternalMessageID = uuid.NewString()
	}
	if input.ExternalConversationID == "" {
		input.ExternalConversationID = input.Sender
	}
	channel, err := s.resolveChannel(ctx, input)
	if err != nil {
		return RuntimeResult{}, err
	}
	if input.TrustedOrganizationID != nil && channel.OrganizationID != *input.TrustedOrganizationID {
		return RuntimeResult{}, httperror.Forbidden("Channel does not belong to your organization")
	}
	processed, found, err := s.findProcessed(ctx, channel, input.ExternalMessageID)
	if err != nil {
		return RuntimeResult{}, err
	}
	if found {
		switch processed.Status {
		case "processing":
			return RuntimeResult{}, runtimeError(ErrSessionConflict, "This message is already being processed. Please try again.")
		case "processed":
			var result RuntimeResult
			if err := json.Unmarshal([]byte(defaultObject(processed.Result)), &result); err == nil {
				return result, nil
			}
			return RuntimeResult{}, runtimeError(ErrRuntimeConfigurationError, "This message could not be replayed safely.")
		case "failed":
			claimed, claimErr := s.claimFailedProcessed(ctx, processed)
			if claimErr != nil {
				return RuntimeResult{}, claimErr
			}
			if !claimed {
				return RuntimeResult{}, runtimeError(ErrSessionConflict, "This message is already being retried. Please try again.")
			}
			processed.Status, processed.Result, processed.SessionID = "processing", "{}", nil
		default:
			return RuntimeResult{}, runtimeError(ErrRuntimeConfigurationError, "This message has an unsupported processing state.")
		}
	}
	if !found {
		processed = ProcessedMessage{ID: uuid.New(), OrganizationID: channel.OrganizationID, ChannelID: channel.ID, ExternalMessageID: input.ExternalMessageID, ExternalConversationID: input.ExternalConversationID, Status: "processing", Result: "{}"}
		if err := s.db.WithContext(ctx).Create(&processed).Error; err != nil {
			if duplicateProcessed, ok, findErr := s.findProcessed(ctx, channel, input.ExternalMessageID); findErr == nil && ok {
				switch duplicateProcessed.Status {
				case "processing":
					return RuntimeResult{}, runtimeError(ErrSessionConflict, "This message is already being processed. Please try again.")
				case "processed":
					var result RuntimeResult
					if err := json.Unmarshal([]byte(defaultObject(duplicateProcessed.Result)), &result); err == nil {
						return result, nil
					}
					return RuntimeResult{}, runtimeError(ErrRuntimeConfigurationError, "This message could not be replayed safely.")
				case "failed":
					claimed, claimErr := s.claimFailedProcessed(ctx, duplicateProcessed)
					if claimErr != nil {
						return RuntimeResult{}, claimErr
					}
					if !claimed {
						return RuntimeResult{}, runtimeError(ErrSessionConflict, "This message is already being retried. Please try again.")
					}
					processed = duplicateProcessed
					processed.Status, processed.Result, processed.SessionID = "processing", "{}", nil
				default:
					return RuntimeResult{}, runtimeError(ErrRuntimeConfigurationError, "This message has an unsupported processing state.")
				}
			}
			return RuntimeResult{}, err
		}
	}
	customer, err := s.resolveCustomer(ctx, channel, input)
	if err != nil {
		_ = s.markProcessed(ctx, processed, nil, "failed", RuntimeResult{Error: publicRuntimeError(err)})
		return RuntimeResult{}, err
	}
	input.CustomerID = &customer.ID
	s.log.Info("runtime inbound message",
		"organization_id", channel.OrganizationID,
		"channel_id", channel.ID,
		"provider", channel.Provider,
		"message_id", input.ExternalMessageID,
		"command", string(classifyCommand(input.Text)),
		"text_preview", truncateLogText(input.Text, 80),
	)

	session, snapshot, err := s.loadOrCreateSession(ctx, channel, input)
	if err != nil {
		_ = s.markProcessed(ctx, processed, nil, "failed", RuntimeResult{Error: publicRuntimeError(err)})
		return RuntimeResult{}, err
	}
	result, err := s.execute(ctx, snapshot, &session, input)
	if err != nil {
		s.log.Error("runtime execution failed", "organization_id", channel.OrganizationID, "conversation_id", session.ID, "current_step", session.CurrentStepKey, "module", currentModuleName(session), "error", publicRuntimeError(err).Message)
		s.recordEvent(ctx, session, EventRuntimeError, "error", "", "", map[string]any{"error": publicRuntimeError(err)})
		result = RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{s.fallbackMessage(snapshot, err)}, Error: publicRuntimeError(err)}
	}
	result.Metadata = mergeMaps(result.Metadata, map[string]any{"processing_ms": s.now().Sub(start).Milliseconds(), "bot_version_id": session.BotVersionID.String()})
	if saveErr := s.persistResult(ctx, &session, input, result); saveErr != nil {
		return RuntimeResult{}, saveErr
	}
	if err := s.markProcessed(ctx, processed, &session.ID, "processed", result); err != nil {
		return RuntimeResult{}, err
	}
	s.log.Info("runtime message processed",
		"organization_id", channel.OrganizationID,
		"conversation_id", session.ID,
		"bot_version_id", session.BotVersionID,
		"channel_id", channel.ID,
		"current_step", session.CurrentStepKey,
		"module", currentModuleName(session),
		"selected_option", strings.TrimSpace(input.Text),
		"message_id", input.ExternalMessageID,
		"status", result.SessionStatus,
		"outbound_count", len(result.Messages),
	)
	return result, nil
}

func (s *Service) StartTestSession(ctx context.Context, actor auth.CurrentUser, input RuntimeStartInput) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionAIUse) {
		return ConversationSession{}, httperror.Forbidden("You cannot use the runtime simulator")
	}
	channel, err := s.getChannelForActor(ctx, actor, input.ChannelID)
	if err != nil {
		return ConversationSession{}, err
	}
	start := InboundMessage{ChannelID: &channel.ID, TrustedOrganizationID: &actor.OrganizationID, SimulatorBotID: &input.BotID, ExternalMessageID: "start-" + uuid.NewString(), ExternalConversationID: defaultString(input.ExternalConversationID, "test-"+uuid.NewString()), Sender: defaultString(input.Sender, "simulator"), Text: "start", Timestamp: s.now(), SimulatorStart: true}
	customer, err := s.resolveCustomer(ctx, channel, start)
	if err != nil {
		return ConversationSession{}, err
	}
	start.CustomerID = &customer.ID
	session, _, err := s.loadOrCreateSession(ctx, channel, start)
	return session, err
}

func (s *Service) ProcessTestMessage(ctx context.Context, actor auth.CurrentUser, input RuntimeMessageInput) (RuntimeResult, error) {
	if !actor.Role.HasPermission(authz.PermissionAIUse) {
		return RuntimeResult{}, httperror.Forbidden("You cannot use the runtime simulator")
	}
	var session ConversationSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, input.SessionID).First(&session).Error; err != nil {
		return RuntimeResult{}, mapNotFoundCode(err, ErrSessionNotFound, "Runtime session not found")
	}
	sender := defaultString(session.ExternalConversationID, "simulator")
	if session.CustomerID != nil {
		var customer core.Customer
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, *session.CustomerID).First(&customer).Error; err == nil && strings.TrimSpace(customer.Phone) != "" {
			sender = customer.Phone
		}
	}
	inbound := InboundMessage{ChannelID: &session.ChannelID, TrustedOrganizationID: &actor.OrganizationID, ExternalMessageID: defaultString(input.ExternalMessageID, uuid.NewString()), ExternalConversationID: session.ExternalConversationID, Sender: sender, Text: input.Text, Location: input.Location, Metadata: input.Metadata, Timestamp: s.now()}
	return s.ProcessMessage(ctx, inbound)
}

func (s *Service) ListConversations(ctx context.Context, actor auth.CurrentUser, filter ConversationFilter) ([]ConversationSummary, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsView) && actor.Role != authz.ServiceProvider {
		return nil, httperror.Forbidden("You cannot view runtime conversations")
	}
	var sessions []ConversationSession
	query := s.db.WithContext(ctx).Where("conversation_sessions.organization_id = ?", actor.OrganizationID).Order("conversation_sessions.updated_at DESC").Limit(100)
	query = s.scopeConversationQuery(query, actor)
	if status := strings.ToLower(strings.TrimSpace(filter.Status)); status != "" && status != "all" {
		if !validConversationStatus(status) {
			return nil, httperror.BadRequest("Conversation status is not valid")
		}
		query = query.Where("conversation_status = ?", status)
	}
	switch strings.ToLower(strings.TrimSpace(filter.Assigned)) {
	case "me":
		query = query.Where("assigned_user_id = ?", actor.ID)
	case "unassigned":
		query = query.Where("assigned_user_id IS NULL")
	}
	if filter.Unread {
		query = query.Where("unread_count > 0")
	}
	if err := query.Find(&sessions).Error; err != nil {
		return nil, err
	}
	summaries := make([]ConversationSummary, 0, len(sessions))
	for _, session := range sessions {
		conversationStatus := conversationStatusForRuntime(session)
		summary := ConversationSummary{
			ID:                     session.ID,
			OrganizationID:         session.OrganizationID,
			BotID:                  session.BotID,
			BotVersionID:           session.BotVersionID,
			ChannelID:              session.ChannelID,
			CustomerID:             session.CustomerID,
			StoreID:                session.StoreID,
			ExternalConversationID: session.ExternalConversationID,
			CurrentStepKey:         session.CurrentStepKey,
			ExpectedInput:          session.ExpectedInput,
			Status:                 session.Status,
			ConversationStatus:     conversationStatus,
			AssignedUserID:         session.AssignedUserID,
			Priority:               sessionPriority(session),
			HandoffState:           session.HandoffState,
			UnreadCount:            session.UnreadCount,
			CurrentModule:          currentModuleName(session),
			LastMessage:            session.LastMessageBody,
			LastMessageDirection:   session.LastMessageDirection,
			LastMessageAt:          session.LastMessageAt,
			LastReadAt:             session.LastReadAt,
			ResolvedAt:             session.ResolvedAt,
			UpdatedAt:              session.UpdatedAt,
			CreatedAt:              session.CreatedAt,
		}
		if session.CustomerID != nil {
			var customer core.Customer
			if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, *session.CustomerID).First(&customer).Error; err == nil {
				summary.CustomerName = customer.Name
				summary.CustomerPhone = customer.Phone
			}
		}
		if session.StoreID != nil {
			storeName, err := s.conversationStore(ctx, actor.OrganizationID, session.StoreID)
			if err != nil {
				return nil, err
			}
			summary.StoreName = storeName
		}
		if session.AssignedUserID != nil {
			var user organizationUserLite
			if err := s.db.WithContext(ctx).Table("users").Select("first_name, last_name, email").Where("id = ?", *session.AssignedUserID).First(&user).Error; err == nil {
				summary.AssignedUserName = strings.TrimSpace(strings.TrimSpace(user.FirstName+" "+user.LastName) + " " + user.Email)
			}
		}
		if summary.LastMessage == "" {
			var last ConversationMessage
			if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ?", actor.OrganizationID, session.ID).Order("created_at DESC").First(&last).Error; err == nil {
				summary.LastMessage = last.Body
				summary.LastMessageDirection = last.Direction
			}
		}
		var handoff SupportHandoff
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ? AND status IN ?", actor.OrganizationID, session.ID, activeHandoffStatuses).Order("created_at DESC").First(&handoff).Error; err == nil {
			summary.HandoffStatus = handoff.Status
			summary.HandoffID = &handoff.ID
			if handoff.Priority != "" {
				summary.Priority = handoff.Priority
			}
		}
		if err := s.attachConversationCommerceContext(ctx, actor, &summary); err != nil {
			return nil, err
		}
		if strings.TrimSpace(filter.Search) != "" {
			haystack := strings.ToLower(summary.CustomerName + " " + summary.CustomerPhone + " " + summary.StoreName + " " + summary.LastMessage + " " + summary.OrderNumber + " " + summary.OrderStatus + " " + summary.PaymentStatus + " " + summary.FulfilmentStatus)
			if !strings.Contains(haystack, strings.ToLower(strings.TrimSpace(filter.Search))) {
				continue
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

type organizationUserLite struct {
	FirstName string
	LastName  string
	Email     string
}

func (s *Service) GetConversationDetail(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) (ConversationSummary, error) {
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return ConversationSummary{}, err
	}
	return s.hydrateConversationSummary(ctx, actor, session)
}

func (s *Service) hydrateConversationSummary(ctx context.Context, actor auth.CurrentUser, session ConversationSession) (ConversationSummary, error) {
	summary := ConversationSummary{
		ID:                     session.ID,
		OrganizationID:         session.OrganizationID,
		BotID:                  session.BotID,
		BotVersionID:           session.BotVersionID,
		ChannelID:              session.ChannelID,
		CustomerID:             session.CustomerID,
		StoreID:                session.StoreID,
		ExternalConversationID: session.ExternalConversationID,
		CurrentStepKey:         session.CurrentStepKey,
		ExpectedInput:          session.ExpectedInput,
		Status:                 session.Status,
		ConversationStatus:     conversationStatusForRuntime(session),
		AssignedUserID:         session.AssignedUserID,
		Priority:               sessionPriority(session),
		HandoffState:           session.HandoffState,
		UnreadCount:            session.UnreadCount,
		CurrentModule:          currentModuleName(session),
		LastMessage:            session.LastMessageBody,
		LastMessageDirection:   session.LastMessageDirection,
		LastMessageAt:          session.LastMessageAt,
		LastReadAt:             session.LastReadAt,
		ResolvedAt:             session.ResolvedAt,
		UpdatedAt:              session.UpdatedAt,
		CreatedAt:              session.CreatedAt,
	}
	if session.CustomerID != nil {
		var customer core.Customer
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, *session.CustomerID).First(&customer).Error; err == nil {
			summary.CustomerName = customer.Name
			summary.CustomerPhone = customer.Phone
		}
	}
	if session.StoreID != nil {
		storeName, err := s.conversationStore(ctx, actor.OrganizationID, session.StoreID)
		if err != nil {
			return summary, err
		}
		summary.StoreName = storeName
	}
	if session.AssignedUserID != nil {
		var user organizationUserLite
		if err := s.db.WithContext(ctx).Table("users").Select("first_name, last_name, email").Where("id = ?", *session.AssignedUserID).First(&user).Error; err == nil {
			summary.AssignedUserName = strings.TrimSpace(strings.TrimSpace(user.FirstName+" "+user.LastName) + " " + user.Email)
		}
	}
	if summary.LastMessage == "" {
		var last ConversationMessage
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ?", actor.OrganizationID, session.ID).Order("created_at DESC").First(&last).Error; err == nil {
			summary.LastMessage = last.Body
			summary.LastMessageDirection = last.Direction
		}
	}
	var handoff SupportHandoff
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ? AND status IN ?", actor.OrganizationID, session.ID, activeHandoffStatuses).Order("created_at DESC").First(&handoff).Error; err == nil {
		summary.HandoffStatus = handoff.Status
		summary.HandoffID = &handoff.ID
		if handoff.Priority != "" {
			summary.Priority = handoff.Priority
		}
	}
	if err := s.attachConversationCommerceContext(ctx, actor, &summary); err != nil {
		return summary, err
	}
	return summary, nil
}

func (s *Service) GetConversation(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) (ConversationSession, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsView) && actor.Role != authz.ServiceProvider {
		return ConversationSession{}, httperror.Forbidden("You cannot view runtime conversations")
	}
	var session ConversationSession
	query := s.db.WithContext(ctx).Where("conversation_sessions.organization_id = ? AND conversation_sessions.id = ?", actor.OrganizationID, sessionID)
	query = s.scopeConversationQuery(query, actor)
	err := query.First(&session).Error
	return session, mapNotFoundCode(err, ErrSessionNotFound, "Runtime session not found")
}

func (s *Service) ListConversationMessages(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) ([]ConversationMessage, error) {
	if _, err := s.GetConversation(ctx, actor, sessionID); err != nil {
		return nil, err
	}
	var messages []ConversationMessage
	err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ?", actor.OrganizationID, sessionID).Order("created_at ASC").Find(&messages).Error
	return messages, err
}

func (s *Service) ReplyToConversation(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID, input ConversationReplyInput) (ConversationMessage, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) && actor.Role != authz.ServiceProvider {
		return ConversationMessage{}, httperror.Forbidden("You cannot reply to conversations")
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return ConversationMessage{}, httperror.BadRequest("Reply text is required")
	}
	session, err := s.GetConversation(ctx, actor, sessionID)
	if err != nil {
		return ConversationMessage{}, err
	}
	var channel core.Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, session.ChannelID).First(&channel).Error; err != nil {
		return ConversationMessage{}, mapNotFoundCode(err, ErrChannelNotFound, "Channel not found")
	}
	now := s.now()
	outbound := ConversationMessage{ID: uuid.New(), OrganizationID: session.OrganizationID, SessionID: session.ID, ChannelID: session.ChannelID, Direction: DirectionOutbound, MessageType: MessageText, Sender: "agent", Body: text, Metadata: jsonValue(map[string]any{"source": "admin_reply", "actor_user_id": actor.ID.String()}), CreatedAt: now}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&outbound).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", session.OrganizationID, session.ID).Updates(map[string]any{
			"last_message_at":        &now,
			"last_message_body":      text,
			"last_message_direction": DirectionOutbound,
			"conversation_status":    ConversationWaiting,
			"updated_at":             now,
		}).Error; err != nil {
			return err
		}
		return s.auditTx(tx, actor, "conversation_session", session.ID, "conversation_agent_reply_added", map[string]any{"message_id": outbound.ID.String()})
	}); err != nil {
		return ConversationMessage{}, err
	}
	result := RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageText, Text: text}}}
	inbound := InboundMessage{ChannelID: &channel.ID, ExternalMessageID: "agent-reply-" + outbound.ID.String(), ExternalConversationID: session.ExternalConversationID, Sender: session.ExternalConversationID}
	if _, err := s.DispatchOutbound(ctx, channel, inbound, result); err != nil {
		s.log.Warn("runtime agent reply dispatch failed", "conversation_id", session.ID, "channel_id", channel.ID)
	}
	s.log.Info("runtime agent reply sent", "organization_id", session.OrganizationID, "conversation_id", session.ID, "channel_id", channel.ID)
	return outbound, nil
}

func (s *Service) ReopenSupportHandoff(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID) (SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoff{}, httperror.Forbidden("You cannot reopen support handoffs")
	}
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
		query = s.scopeHandoffQuery(query, actor)
		if err := query.First(&handoff).Error; err != nil {
			return mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
		}
		now := s.now()
		newStatus := "open"
		conversationStatus := ConversationHumanRequested
		if handoff.AssignedUserID != nil {
			newStatus = "assigned"
			conversationStatus = ConversationHumanAssigned
		}
		if err := tx.Model(&handoff).Updates(map[string]any{"status": newStatus, "resolved_at": nil, "released_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).Updates(map[string]any{"status": SessionHandoff, "conversation_status": conversationStatus, "assigned_user_id": handoff.AssignedUserID, "handoff_state": newStatus, "human_owned_at": &now, "resolved_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_reopened", map[string]any{"previous_status": handoff.Status, "new_status": newStatus}); err != nil {
			return err
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID).First(&handoff).Error
	})
	if err == nil {
		s.recordEvent(ctx, ConversationSession{ID: handoff.SessionID, OrganizationID: actor.OrganizationID}, EventHandoffReopened, "info", "", "", map[string]any{"status": handoff.Status})
	}
	return handoff, err
}

func (s *Service) ListSupportHandoffs(ctx context.Context, actor auth.CurrentUser, status string) ([]SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsView) {
		return nil, httperror.Forbidden("You cannot view support handoffs")
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Limit(100)
	query = s.scopeHandoffQuery(query, actor)
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	}
	var handoffs []SupportHandoff
	return handoffs, query.Find(&handoffs).Error
}

func (s *Service) ListSupportTickets(ctx context.Context, actor auth.CurrentUser, status string) ([]SupportTicket, error) {
	if !actor.Role.HasPermission(authz.PermissionComplaintsView) {
		return nil, httperror.Forbidden("You cannot view support tickets")
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Limit(100)
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(status))
	}
	var tickets []SupportTicket
	return tickets, query.Find(&tickets).Error
}

func (s *Service) ClaimSupportHandoff(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID, input SupportHandoffClaimInput) (SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoff{}, httperror.Forbidden("You cannot claim support handoffs")
	}
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
		query = s.scopeHandoffQuery(query, actor)
		if err := query.First(&handoff).Error; err != nil {
			return mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
		}
		if handoff.Status == "resolved" || handoff.Status == "released" {
			return httperror.BadRequest("Closed handoffs cannot be claimed")
		}
		if handoff.AssignedUserID != nil && *handoff.AssignedUserID != actor.ID && actor.Role == authz.SupportAgent {
			return httperror.Forbidden("You cannot claim another agent's handoff")
		}
		now := s.now()
		priority := normalizePriority(input.Priority)
		if input.Priority == "" && handoff.Priority != "" {
			priority = handoff.Priority
		}
		updates := map[string]any{"status": "assigned", "assigned_user_id": actor.ID, "priority": priority, "released_at": nil, "updated_at": now}
		if err := tx.Model(&handoff).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).Updates(map[string]any{"status": SessionHandoff, "conversation_status": ConversationHumanAssigned, "assigned_user_id": actor.ID, "priority": priority, "handoff_state": "assigned", "human_owned_at": &now, "human_released_at": nil, "resolved_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if strings.TrimSpace(input.Note) != "" {
			note := SupportHandoffNote{ID: uuid.New(), OrganizationID: actor.OrganizationID, HandoffID: handoffID, ActorUserID: actor.ID, Note: strings.TrimSpace(input.Note), Internal: true}
			if err := tx.Create(&note).Error; err != nil {
				return err
			}
		}
		if err := s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_claimed", map[string]any{"previous_status": handoff.Status, "assigned_user_id": actor.ID.String()}); err != nil {
			return err
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID).First(&handoff).Error
	})
	if err == nil {
		s.recordEvent(ctx, ConversationSession{ID: handoff.SessionID, OrganizationID: actor.OrganizationID}, EventHandoffClaimed, "info", "", "", map[string]any{"assigned_user_id": actor.ID.String()})
	}
	return handoff, err
}

func (s *Service) AddSupportHandoffNote(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID, input SupportHandoffNoteInput) (SupportHandoffNote, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoffNote{}, httperror.Forbidden("You cannot add support notes")
	}
	noteText := strings.TrimSpace(input.Note)
	if noteText == "" {
		return SupportHandoffNote{}, httperror.BadRequest("Note is required")
	}
	var handoff SupportHandoff
	query := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
	query = s.scopeHandoffQuery(query, actor)
	if err := query.First(&handoff).Error; err != nil {
		return SupportHandoffNote{}, mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
	}
	note := SupportHandoffNote{ID: uuid.New(), OrganizationID: actor.OrganizationID, HandoffID: handoffID, ActorUserID: actor.ID, Note: noteText, Internal: input.Internal, CreatedAt: s.now()}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&note).Error; err != nil {
			return err
		}
		now := s.now()
		channelID := uuidFromMetadataOrZero(handoff.Metadata, "channel_id")
		if channelID == uuid.Nil {
			var session ConversationSession
			if err := tx.Select("channel_id").Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).First(&session).Error; err == nil {
				channelID = session.ChannelID
			}
		}
		message := ConversationMessage{ID: uuid.New(), OrganizationID: actor.OrganizationID, SessionID: handoff.SessionID, ChannelID: channelID, Direction: DirectionInternal, MessageType: MessageText, Sender: "agent_note", Body: noteText, Metadata: jsonValue(map[string]any{"handoff_id": handoff.ID.String(), "internal": input.Internal, "actor_user_id": actor.ID.String()}), CreatedAt: now}
		if !input.Internal {
			message.Direction = DirectionOutbound
			message.Sender = "agent"
		}
		if message.ChannelID != uuid.Nil {
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
			if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).Updates(map[string]any{"last_message_at": &now, "last_message_body": noteText, "last_message_direction": message.Direction, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_note_added", map[string]any{"internal": input.Internal})
	})
	return note, err
}

func (s *Service) ListSupportHandoffNotes(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID) ([]SupportHandoffNote, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsView) {
		return nil, httperror.Forbidden("You cannot view support notes")
	}
	var handoff SupportHandoff
	query := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
	query = s.scopeHandoffQuery(query, actor)
	if err := query.First(&handoff).Error; err != nil {
		return nil, mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
	}
	var notes []SupportHandoffNote
	err := s.db.WithContext(ctx).Where("organization_id = ? AND handoff_id = ?", actor.OrganizationID, handoffID).Order("created_at ASC").Find(&notes).Error
	return notes, err
}

func (s *Service) ResolveSupportHandoff(ctx context.Context, actor auth.CurrentUser, handoffID uuid.UUID, input SupportHandoffResolveInput) (SupportHandoff, error) {
	if !actor.Role.HasPermission(authz.PermissionConversationsManage) {
		return SupportHandoff{}, httperror.Forbidden("You cannot resolve support handoffs")
	}
	var handoff SupportHandoff
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID)
		query = s.scopeHandoffQuery(query, actor)
		if err := query.First(&handoff).Error; err != nil {
			return mapNotFoundCode(err, ErrSessionNotFound, "Support handoff not found")
		}
		if handoff.Status == "resolved" {
			return nil
		}
		now := s.now()
		metadata := parseJSONMap(handoff.Metadata)
		if strings.TrimSpace(input.ResolutionNote) != "" {
			metadata["resolution_note"] = strings.TrimSpace(input.ResolutionNote)
		}
		if handoff.AssignedUserID != nil && actor.Role == authz.SupportAgent && *handoff.AssignedUserID != actor.ID {
			return httperror.Forbidden("You cannot resolve another agent's handoff")
		}
		updates := map[string]any{"status": "resolved", "resolved_at": &now, "metadata": jsonMap(metadata), "updated_at": now}
		if err := tx.Model(&handoff).Updates(updates).Error; err != nil {
			return err
		}
		sessionStatus := SessionCompleted
		conversationStatus := ConversationResolved
		sessionUpdates := map[string]any{"status": sessionStatus, "conversation_status": conversationStatus, "assigned_user_id": nil, "handoff_state": "resolved", "human_released_at": &now, "resolved_at": &now, "updated_at": now}
		if input.ResumeBot {
			sessionStatus = SessionActive
			conversationStatus = ConversationAIHandling
			sessionUpdates["status"] = sessionStatus
			sessionUpdates["conversation_status"] = conversationStatus
			sessionUpdates["resolved_at"] = nil
			if startStepKey := s.startStepKeyForHandoffTx(tx, actor.OrganizationID, handoff.SessionID); startStepKey != "" {
				sessionUpdates["current_step_key"] = startStepKey
				sessionUpdates["expected_input"] = ""
			}
		}
		if err := tx.Model(&ConversationSession{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, handoff.SessionID).Updates(sessionUpdates).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, actor, "support_handoff", handoff.ID, "handoff_resolved", map[string]any{"previous_status": handoff.Status, "resume_bot": input.ResumeBot, "resolution_note": strings.TrimSpace(input.ResolutionNote)}); err != nil {
			return err
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, handoffID).First(&handoff).Error
	})
	if err == nil {
		s.recordEvent(ctx, ConversationSession{ID: handoff.SessionID, OrganizationID: actor.OrganizationID}, EventHandoffResolved, "info", "", "", map[string]any{"resume_bot": input.ResumeBot})
	}
	return handoff, err
}

func (s *Service) scopeConversationQuery(query *gorm.DB, actor auth.CurrentUser) *gorm.DB {
	if actor.Role != authz.SupportAgent {
		return query
	}
	return query.Joins(
		"JOIN support_handoffs sh_scope ON sh_scope.organization_id = conversation_sessions.organization_id AND sh_scope.session_id = conversation_sessions.id AND ((sh_scope.status IN ? AND sh_scope.assigned_user_id IS NULL) OR sh_scope.assigned_user_id = ?)",
		activeHandoffStatuses,
		actor.ID,
	)
}

func (s *Service) scopeHandoffQuery(query *gorm.DB, actor auth.CurrentUser) *gorm.DB {
	if actor.Role != authz.SupportAgent {
		return query
	}
	return query.Where("((status IN ? AND assigned_user_id IS NULL) OR assigned_user_id = ?)", activeHandoffStatuses, actor.ID)
}

func (s *Service) VerifyWhatsAppRequest(channel core.Channel, signature string, body []byte) bool {
	secret := stringValue(parseJSONMap(channel.SecretConfig)["app_secret"])
	if secret == "" {
		return false
	}
	signature = strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (s *Service) GetWhatsAppChannel(ctx context.Context, phoneNumberID string) (core.Channel, error) {
	var channel core.Channel
	err := s.db.WithContext(ctx).Where("provider = ? AND phone_number_id = ? AND status = ?", "whatsapp", strings.TrimSpace(phoneNumberID), core.StatusActive).First(&channel).Error
	return channel, mapNotFoundCode(err, ErrChannelNotFound, "WhatsApp channel not found")
}

func (s *Service) VerifyWhatsAppToken(ctx context.Context, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	var channels []core.Channel
	if err := s.db.WithContext(ctx).Where("provider = ? AND status = ?", "whatsapp", core.StatusActive).Find(&channels).Error; err != nil {
		return false
	}
	for _, channel := range channels {
		config := parseJSONMap(channel.Config)
		secrets := parseJSONMap(channel.SecretConfig)
		if stringValue(config["verify_token"]) == token || stringValue(secrets["verify_token"]) == token {
			return true
		}
	}
	return false
}

func (s *Service) ParseWhatsAppWebhook(payload WhatsAppWebhookPayload) []InboundMessage {
	var inputs []InboundMessage
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			phoneNumberID := change.Value.Metadata.PhoneNumberID
			for _, msg := range change.Value.Messages {
				text := msg.Text.Body
				if msg.Interactive.ButtonReply.ID != "" {
					text = msg.Interactive.ButtonReply.ID
				} else if msg.Interactive.ListReply.ID != "" {
					text = msg.Interactive.ListReply.ID
				} else if msg.Button.Payload != "" {
					text = msg.Button.Payload
				}
				var location *Location
				if msg.Type == "location" {
					location = &Location{Latitude: msg.Location.Latitude, Longitude: msg.Location.Longitude, Name: msg.Location.Name, Address: msg.Location.Address}
				}
				inputs = append(inputs, InboundMessage{Provider: "whatsapp", ProviderChannelID: phoneNumberID, ExternalMessageID: msg.ID, ExternalConversationID: msg.From, Sender: msg.From, Text: text, Location: location, Timestamp: s.now(), Metadata: map[string]any{"source": "whatsapp", "message_type": msg.Type}})
			}
		}
	}
	return inputs
}

func (s *Service) TranslateWhatsAppOutbound(message OutboundMessage) map[string]any {
	return translateWhatsAppOutbound(message)
}

func translateWhatsAppOutbound(message OutboundMessage) map[string]any {
	switch message.Type {
	case MessageButtons:
		buttons := make([]map[string]any, 0, len(message.Options))
		for _, option := range message.Options {
			buttons = append(buttons, map[string]any{"type": "reply", "reply": map[string]any{"id": option.ID, "title": option.Label}})
		}
		return map[string]any{"type": "interactive", "interactive": map[string]any{"type": "button", "body": map[string]any{"text": message.Text}, "action": map[string]any{"buttons": buttons}}}
	case MessageList:
		rows := make([]map[string]any, 0, len(message.Options))
		for _, option := range message.Options {
			rows = append(rows, map[string]any{"id": option.ID, "title": option.Label, "description": option.Description})
		}
		return map[string]any{"type": "interactive", "interactive": map[string]any{"type": "list", "body": map[string]any{"text": message.Text}, "action": map[string]any{"button": "Choose", "sections": []map[string]any{{"title": "Options", "rows": rows}}}}}
	case MessageImage:
		return map[string]any{"type": "image", "image": map[string]any{"link": message.MediaURL, "caption": message.Text}}
	default:
		return map[string]any{"type": "text", "text": map[string]any{"body": message.Text}}
	}
}

func (s *Service) loadOrCreateSession(ctx context.Context, channel core.Channel, input InboundMessage) (ConversationSession, bot.VersionConfiguration, error) {
	var session ConversationSession
	err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND channel_id = ? AND external_conversation_id = ?", channel.OrganizationID, channel.ID, input.ExternalConversationID).First(&session).Error
	newConversation := err == gorm.ErrRecordNotFound
	if err != nil && !newConversation {
		return ConversationSession{}, bot.VersionConfiguration{}, err
	}
	if !newConversation {
		expired := session.ExpiresAt != nil && session.ExpiresAt.Before(s.now())
		if expired && session.Status == SessionActive {
			session.Status = SessionExpired
		}
		terminal := session.Status == SessionCompleted || session.Status == SessionExpired || session.Status == SessionCancelled
		preserveCompletedCheckout := terminal && isCommerceCancelText(input.Text)
		shouldReset := input.SimulatorStart || isHardResetText(input.Text) || (terminal && !preserveCompletedCheckout)
		if !shouldReset {
			if session.CustomerID == nil && input.CustomerID != nil {
				if err := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("id = ?", session.ID).Update("customer_id", input.CustomerID).Error; err != nil {
					return session, bot.VersionConfiguration{}, err
				}
				session.CustomerID = input.CustomerID
			}
			snapshot, err := s.loadSnapshot(ctx, channel.OrganizationID, session.BotID, session.BotVersionID)
			return session, snapshot, err
		}
	}
	botRecord, snapshot, err := s.resolvePublishedBot(ctx, channel, input.SimulatorBotID)
	if err != nil {
		return ConversationSession{}, bot.VersionConfiguration{}, err
	}
	systemContext := map[string]any{"organization_id": channel.OrganizationID.String(), "bot_id": botRecord.ID.String(), "bot_version_id": snapshot.Version.ID.String(), "channel_id": channel.ID.String(), "channel_provider": channel.Provider, "sender": input.Sender, "customer_phone": input.Sender}
	if input.CustomerID != nil {
		systemContext["customer_id"] = input.CustomerID.String()
	}
	now := s.now()
	expiresAt := now.Add(defaultSessionTTL)
	if newConversation {
		session = ConversationSession{ID: uuid.New(), OrganizationID: channel.OrganizationID, BotID: botRecord.ID, BotVersionID: snapshot.Version.ID, ChannelID: channel.ID, CustomerID: input.CustomerID, ExternalConversationID: input.ExternalConversationID, CurrentStepKey: snapshot.Version.StartStepKey, Status: SessionActive, ConversationStatus: ConversationAIHandling, Priority: "normal", Variables: "{}", SystemContext: jsonMap(systemContext), LockVersion: 1, LastMessageAt: &now, ExpiresAt: &expiresAt}
		if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
			return ConversationSession{}, bot.VersionConfiguration{}, err
		}
		s.recordEvent(ctx, session, EventConversationStarted, "info", "", "", nil)
		return session, snapshot, nil
	}
	session.BotID = botRecord.ID
	session.BotVersionID = snapshot.Version.ID
	session.CustomerID = input.CustomerID
	session.CurrentStepKey = snapshot.Version.StartStepKey
	session.ExpectedInput = ""
	session.Status = SessionActive
	session.ConversationStatus = ConversationAIHandling
	session.AssignedUserID = nil
	session.Priority = "normal"
	session.HandoffState = ""
	session.Variables = "{}"
	session.SystemContext = jsonMap(systemContext)
	session.LastMessageAt = &now
	session.ExpiresAt = &expiresAt
	if err := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("id = ?", session.ID).Updates(map[string]any{"bot_id": session.BotID, "bot_version_id": session.BotVersionID, "customer_id": session.CustomerID, "current_step_key": session.CurrentStepKey, "expected_input": "", "status": SessionActive, "conversation_status": ConversationAIHandling, "assigned_user_id": nil, "priority": "normal", "handoff_state": "", "variables": "{}", "system_context": session.SystemContext, "last_message_at": &now, "expires_at": &expiresAt, "resolved_at": nil, "human_released_at": &now, "updated_at": now, "lock_version": gorm.Expr("lock_version + 1")}).Error; err != nil {
		return ConversationSession{}, bot.VersionConfiguration{}, err
	}
	session.LockVersion++
	s.recordEvent(ctx, session, EventConversationStarted, "info", "", "", map[string]any{"reset": true})
	return session, snapshot, nil
}

func (s *Service) resolveCustomer(ctx context.Context, channel core.Channel, input InboundMessage) (core.Customer, error) {
	name := stringValue(input.Metadata["profile_name"])
	customer, err := s.commerce.FindOrCreateCustomer(ctx, auth.CurrentUser{ID: uuid.Nil, OrganizationID: channel.OrganizationID, Role: authz.MerchantAdmin}, core.CustomerInput{Name: name, Phone: input.Sender, Metadata: `{"source":"runtime"}`})
	if err != nil {
		return core.Customer{}, runtimeErrorf(ErrActionFailed, "I could not identify this customer.", "resolve customer failed: %v", err)
	}
	return customer, nil
}

func (s *Service) openSupportHandoff(ctx context.Context, session ConversationSession, reason string, runtimeContext RuntimeContext) error {
	var existing SupportHandoff
	err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ? AND status IN ?", session.OrganizationID, session.ID, activeHandoffStatuses).First(&existing).Error
	if err == nil {
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	orderID := uuidFromAny(runtimeContext.Variables["order_id"])
	if orderID == nil {
		if raw, ok := lookupPath(runtimeContext.Variables, "order.id"); ok {
			orderID = uuidFromAny(raw)
		}
	}
	if orderID == nil {
		var link core.ConversationOrderLink
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND conversation_session_id = ?", session.OrganizationID, session.ID).Order("created_at DESC").First(&link).Error; err == nil {
			orderID = &link.OrderID
		}
	}
	handoff := SupportHandoff{
		ID:             uuid.New(),
		OrganizationID: session.OrganizationID,
		SessionID:      session.ID,
		CustomerID:     session.CustomerID,
		OrderID:        orderID,
		Status:         "open",
		Reason:         strings.TrimSpace(reason),
		Priority:       "normal",
		Metadata:       jsonMap(map[string]any{"bot_id": session.BotID.String(), "bot_version_id": session.BotVersionID.String(), "channel_id": session.ChannelID.String(), "external_conversation_id": session.ExternalConversationID}),
	}
	return s.db.WithContext(ctx).Create(&handoff).Error
}

func uuidFromAny(value any) *uuid.UUID {
	raw := strings.TrimSpace(stringValue(value))
	if raw == "" {
		return nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &parsed
}

func currentModuleName(session ConversationSession) string {
	system := parseJSONMap(session.SystemContext)
	stack := moduleStack(system)
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1].ModuleKey
}

func (s *Service) execute(ctx context.Context, snapshot bot.VersionConfiguration, session *ConversationSession, input InboundMessage) (RuntimeResult, error) {
	variables := parseJSONMap(session.Variables)
	defer func() {
		session.Variables = jsonMap(variables)
	}()
	system := parseJSONMap(session.SystemContext)
	runtimeContext := RuntimeContext{Session: *session, Variables: variables, System: system, Source: core.CommerceEventSourceRuntime}
	result := RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{}, Metadata: map[string]any{}}
	if session.Status == SessionHandoff {
		s.applyActiveHandoffState(ctx, session)
		if classifyCommand(input.Text) == commandRestart || classifyCommand(input.Text) == commandMenu {
			returnToEntry(snapshot, session, runtimeContext)
			variables = map[string]any{}
			runtimeContext.Variables = variables
			session.ConversationStatus = ConversationAIHandling
			session.AssignedUserID = nil
			session.HandoffState = "released"
			releasedAt := s.now()
			session.HumanReleasedAt = &releasedAt
		} else {
			if session.AIShouldPause() {
				return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Handoff: true, Messages: []OutboundMessage{}, Metadata: map[string]any{"ai_paused": true, "reason": "human_handoff_active"}}, nil
			}
			if s.handoffInbound != nil {
				handled, reply, err := s.handoffInbound(ctx, session.OrganizationID, session.ID, input.Text)
				if err != nil {
					return result, err
				}
				if handled {
					messages := []OutboundMessage{}
					if strings.TrimSpace(reply) != "" {
						messages = append(messages, OutboundMessage{Type: MessageText, Text: reply})
					}
					return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Handoff: true, Messages: messages}, nil
				}
			}
			return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Handoff: true, Messages: []OutboundMessage{}, Metadata: map[string]any{"ai_paused": true, "reason": "handoff_requested"}}, nil
		}
	}
	if session.Status == SessionCompleted || session.Status == SessionExpired || session.Status == SessionCancelled {
		if !isResetText(input.Text) && !isCommerceCancelText(input.Text) {
			return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Messages: []OutboundMessage{{Type: MessageText, Text: "This conversation has ended. Send 'hi' or 'menu' to begin again."}}}, nil
		}
		if !isCommerceCancelText(input.Text) {
			returnToEntry(snapshot, session, runtimeContext)
			variables = map[string]any{}
			runtimeContext.Variables = variables
		}
	}
	if session.Status == SessionActive && session.ExpectedInput == "" && session.CurrentStepKey == snapshot.Version.StartStepKey && !input.SimulatorStart && !isResetText(input.Text) {
		if intent := classifyNaturalEntryIntent(input.Text); intent != "" {
			if start, ok := indexSteps(snapshot.Steps)[snapshot.Version.StartStepKey]; ok && start.NextStepKey != "" {
				setPath(runtimeContext.Variables, "intent", intent)
				session.CurrentStepKey = start.NextStepKey
			}
		} else if s.aiInbound != nil {
			response, err := s.aiInbound(ctx, *session, input.Text)
			if err != nil {
				s.log.Warn("grounded AI entry handling failed; falling back to workflow", "organization_id", session.OrganizationID, "conversation_id", session.ID, "error", err.Error())
			} else if strings.TrimSpace(response.Reply) != "" {
				if strings.TrimSpace(response.Variables) != "" {
					variables = parseJSONMap(response.Variables)
					runtimeContext.Variables = variables
				}
				result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: strings.TrimSpace(response.Reply)})
				result.SessionStatus = session.Status
				return result, nil
			}
		}
	}
	if handled, err := s.handleLifecycle(ctx, snapshot, session, input, runtimeContext, &result); err != nil {
		return result, err
	} else if handled {
		if session.ExpectedInput != "" {
			result.SessionStatus = session.Status
			return result, nil
		}
	}
	steps := indexSteps(snapshot.Steps)
	if session.ExpectedInput == lifecycleTrackEmpty {
		if handled := s.handleTrackEmptyChoice(snapshot, session, input, runtimeContext, &result); handled {
			if session.ExpectedInput != "" {
				result.SessionStatus = session.Status
				return result, nil
			}
		}
	} else if session.ExpectedInput != "" {
		step, ok := steps[session.CurrentStepKey]
		if !ok {
			return result, runtimeErrorf(ErrInvalidStep, "This bot is not configured correctly.", "waiting step %s not found", session.CurrentStepKey)
		}
		if err := s.acceptAnswer(step, snapshot, session, input, runtimeContext); err != nil {
			retries := int64Value(runtimeContext.System["invalid_input_retries"]) + 1
			runtimeContext.System["invalid_input_retries"] = retries
			session.SystemContext = jsonMap(runtimeContext.System)
			if retries >= maxInvalidRetries {
				result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: "Let's start over from the main menu."})
				returnToEntry(snapshot, session, runtimeContext)
				variables = map[string]any{}
				runtimeContext.Variables = variables
			} else {
				result.Messages = append(result.Messages, s.validationMessage(snapshot, err))
				return result, nil
			}
		} else {
			runtimeContext.System["invalid_input_retries"] = 0
			session.SystemContext = jsonMap(runtimeContext.System)
			s.recordEvent(ctx, *session, EventAnswerReceived, "info", step.StepKey, "", map[string]any{"selected_option": strings.TrimSpace(input.Text)})
		}
	}
	guard := 0
	for session.Status == SessionActive && session.ExpectedInput == "" {
		guard++
		if guard > 50 {
			return result, runtimeError(ErrRuntimeConfigurationError, "This bot has too many automatic steps.")
		}
		step, ok := steps[session.CurrentStepKey]
		if !ok {
			return result, runtimeErrorf(ErrInvalidStep, "This bot is not configured correctly.", "step %s not found", session.CurrentStepKey)
		}
		s.recordEvent(ctx, *session, EventStepExecuted, "info", step.StepKey, "", nil)
		continued, err := s.executeStep(ctx, step, snapshot, session, runtimeContext, &result)
		if err != nil {
			return result, err
		}
		if !continued {
			break
		}
		runtimeContext.Session = *session
	}
	result.SessionStatus = session.Status
	result.Handoff = session.Status == SessionHandoff
	return result, nil
}

func classifyNaturalEntryIntent(text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
	if normalized == "" {
		return ""
	}
	for _, term := range []string{"track my order", "track order", "order status", "where is my order", "my order"} {
		if strings.Contains(normalized, term) {
			return "track_order"
		}
	}
	for _, term := range []string{"complaint", "complain", "wrong item", "damaged item", "problem with my order"} {
		if strings.Contains(normalized, term) {
			return "complaint"
		}
	}
	for _, term := range []string{"speak to a human", "speak to someone", "human agent", "customer support", "talk to an agent"} {
		if strings.Contains(normalized, term) {
			return "support"
		}
	}
	for _, term := range []string{"i want to order", "i would like to order", "i want to buy", "i would like to buy", "can i order", "can i buy", "place an order", "add to cart", "i'll take", "ill take"} {
		if strings.Contains(normalized, term) {
			return "order"
		}
	}
	return ""
}

func (s *Service) executeStep(ctx context.Context, step bot.Step, snapshot bot.VersionConfiguration, session *ConversationSession, runtimeContext RuntimeContext, result *RuntimeResult) (bool, error) {
	switch step.Type {
	case bot.StepMessage:
		if step.Message != "" {
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: runtimeContext.Render(step.Message, fallbackVariable(snapshot))})
		}
		return s.advance(session, step.NextStepKey), nil
	case bot.StepQuestion, bot.StepChoice:
		question, err := findQuestion(snapshot.Questions, step.QuestionID)
		if err != nil {
			return false, err
		}
		options := parseOptions(s.effectiveQuestionOptions(snapshot, step, question))
		msgType := MessageText
		if len(options) > 0 || question.ResponseMode == "buttons" || question.ResponseMode == "single_choice" {
			msgType = MessageButtons
			if len(options) > 3 {
				msgType = MessageList
			}
		}
		result.Messages = append(result.Messages, OutboundMessage{Type: msgType, Text: runtimeContext.Render(question.Text, fallbackVariable(snapshot)), Options: options})
		session.ExpectedInput = question.ID.String()
		s.recordEvent(ctx, *session, EventQuestionPresented, "info", step.StepKey, "", map[string]any{"question_key": question.QuestionKey})
		return false, nil
	case bot.StepCondition:
		condition, err := findCondition(snapshot.Conditions, step.ConditionID)
		if err != nil {
			return false, err
		}
		matches, err := evaluateCondition(condition, runtimeContext)
		if err != nil {
			return false, err
		}
		if matches {
			return s.advance(session, step.NextStepKey), nil
		}
		return s.advance(session, step.FallbackStepKey), nil
	case bot.StepAction:
		action, err := findAction(snapshot.Actions, step.ActionID)
		if err != nil {
			return false, err
		}
		policy := authz.PolicyForRuntimeAction(action.ActionType)
		actionMetadata := map[string]any{"action_type": action.ActionType, "class": policy.Class, "confirmation": policy.Confirmation}
		s.recordEvent(ctx, *session, EventActionRequested, "info", step.StepKey, action.ActionKey, actionMetadata)
		s.recordEvent(ctx, *session, EventActionStarted, "info", step.StepKey, action.ActionKey, actionMetadata)
		if err := s.actions.Authorize(ctx, action.ActionType, runtimeContext); err != nil {
			s.recordEvent(ctx, *session, EventActionDenied, "warning", step.StepKey, action.ActionKey, map[string]any{"action_type": action.ActionType, "error": publicRuntimeError(err)})
			s.recordEvent(ctx, *session, EventActionFailed, "error", step.StepKey, action.ActionKey, map[string]any{"error": publicRuntimeError(err)})
			return false, err
		}
		s.recordEvent(ctx, *session, EventActionAuthorized, "info", step.StepKey, action.ActionKey, actionMetadata)
		outputs, err := s.actions.ExecuteAuthorized(ctx, action, runtimeContext)
		if err != nil {
			s.recordEvent(ctx, *session, EventActionFailed, "error", step.StepKey, action.ActionKey, map[string]any{"error": publicRuntimeError(err)})
			return false, err
		}
		s.recordEvent(ctx, *session, EventActionCompleted, "info", step.StepKey, action.ActionKey, map[string]any{"outputs": scrubOutputs(outputs), "action_result": stringValue(outputs["message"])})
		if stringValue(outputs["message"]) != "" {
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: stringValue(outputs["message"])})
		}
		if outputs["handoff"] == true {
			session.Status = SessionHandoff
			session.ConversationStatus = ConversationHumanRequested
			session.HandoffState = "open"
			_ = s.openSupportHandoff(ctx, *session, stringValue(outputs["reason"]), runtimeContext)
			return false, nil
		}
		if action.ActionType == "get_stores" && outputs["auto_selected"] == true {
			setPath(runtimeContext.Variables, "store_choice", "1")
			setPath(runtimeContext.Variables, "store_id", stringValue(outputs["store_id"]))
			setPath(runtimeContext.Variables, "store_name", stringValue(outputs["store_name"]))
			setPath(runtimeContext.Variables, "store_address", stringValue(outputs["store_address"]))
			return s.advance(session, skipSelectableQuestion(snapshot, step.NextStepKey)), nil
		}
		if action.ActionType == "create_order" {
			s.recordEvent(ctx, *session, EventCommerceOrderLinked, "info", step.StepKey, action.ActionKey, map[string]any{"order_id": stringValue(outputs["order_id"]), "store_id": stringValue(outputs["store_id"])})
		}
		if action.ActionType == "get_customer_orders" {
			count := int(int64Value(outputs["count"]))
			if count == 0 {
				if len(result.Messages) > 0 {
					result.Messages[len(result.Messages)-1].Type = MessageButtons
					result.Messages[len(result.Messages)-1].Options = trackEmptyOptions()
				}
				session.ExpectedInput = lifecycleTrackEmpty
				return false, nil
			}
			if count == 1 {
				next, ok := stepByKey(snapshot, step.NextStepKey)
				if ok && next.QuestionID != nil {
					if question, qerr := findQuestion(snapshot.Questions, next.QuestionID); qerr == nil && question.VariableName != "" {
						setPath(runtimeContext.Variables, question.VariableName, "1")
					}
				} else {
					setPath(runtimeContext.Variables, "track_order_choice", "1")
				}
				return s.advance(session, skipSelectableQuestion(snapshot, step.NextStepKey)), nil
			}
		}
		return s.advance(session, step.NextStepKey), nil
	case bot.StepModule:
		module, err := findModule(snapshot.Modules, step.ModuleID)
		if err != nil {
			return false, err
		}
		if !runtimeModuleEnabled(module.Metadata) {
			return false, runtimeErrorf(ErrRuntimeConfigurationError, "That option is not available right now.", "module %s is disabled", module.ModuleKey)
		}
		s.recordEvent(ctx, *session, EventModuleStarted, "info", step.StepKey, "", map[string]any{"module_key": module.ModuleKey})
		params := parseJSONMap(module.Parameters)
		entry := stringValue(params["entry_step"])
		if entry == "" {
			entry = step.NextStepKey
			s.recordEvent(ctx, *session, EventModuleCompleted, "info", step.StepKey, "", map[string]any{"module_key": module.ModuleKey})
		} else if step.NextStepKey != "" {
			if err := pushModuleFrame(runtimeContext.System, module.ModuleKey, step.StepKey, step.NextStepKey); err != nil {
				return false, err
			}
			session.SystemContext = jsonMap(runtimeContext.System)
		}
		return s.advance(session, entry), nil
	case bot.StepHandoff:
		if step.Message != "" {
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: runtimeContext.Render(step.Message, fallbackVariable(snapshot))})
		}
		session.Status = SessionHandoff
		session.ConversationStatus = ConversationHumanRequested
		session.HandoffState = "open"
		_ = s.openSupportHandoff(ctx, *session, step.Message, runtimeContext)
		s.recordEvent(ctx, *session, EventHandoffStarted, "info", step.StepKey, "", nil)
		return false, nil
	case bot.StepEnd:
		if step.Message != "" {
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: runtimeContext.Render(step.Message, fallbackVariable(snapshot))})
		}
		frame, ok := popModuleFrame(runtimeContext.System)
		if ok {
			session.SystemContext = jsonMap(runtimeContext.System)
			s.recordEvent(ctx, *session, EventModuleCompleted, "info", step.StepKey, "", map[string]any{"module_key": frame.ModuleKey, "return_step_key": frame.ReturnStepKey})
			return s.advance(session, frame.ReturnStepKey), nil
		}
		session.Status = SessionCompleted
		session.ConversationStatus = ConversationResolved
		resolvedAt := s.now()
		session.ResolvedAt = &resolvedAt
		s.recordEvent(ctx, *session, EventConversationCompleted, "info", step.StepKey, "", nil)
		return false, nil
	default:
		return false, runtimeErrorf(ErrInvalidStep, "This bot is not configured correctly.", "unsupported step type %s", step.Type)
	}
}

func (s *Service) handleLifecycle(ctx context.Context, snapshot bot.VersionConfiguration, session *ConversationSession, input InboundMessage, runtimeContext RuntimeContext, result *RuntimeResult) (bool, error) {
	command := classifyCommand(input.Text)
	if command == commandNone {
		return false, nil
	}
	if s.commandCapturedByCurrentQuestion(snapshot, session, input) {
		return false, nil
	}
	atEntry := session.CurrentStepKey == snapshot.Version.StartStepKey
	switch command {
	case commandGreeting:
		if atEntry && session.ExpectedInput != "" && session.ExpectedInput != lifecycleTrackEmpty {
			session.ExpectedInput = ""
			return true, nil
		}
		if atEntry && session.ExpectedInput == "" {
			return false, nil
		}
		result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: "Starting over from the main menu."})
		returnToEntry(snapshot, session, runtimeContext)
		clearRuntimeVariables(runtimeContext)
		return true, nil
	case commandMenu, commandRestart, commandCancel:
		if command == commandCancel {
			message := "Okay, I cancelled that request."
			handled, cancellationMessage, err := s.cancelCommerceLifecycle(ctx, runtimeContext)
			if !handled && err == nil && s.lifecycleCancel != nil {
				handled, cancellationMessage, err = s.lifecycleCancel(ctx, runtimeContext)
			}
			if handled || err != nil {
				if err != nil {
					s.log.Warn("runtime lifecycle cancellation failed", "organization_id", session.OrganizationID, "conversation_id", session.ID, "error", err)
					result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: "I couldn't cancel that request because its payment or fulfilment status may have changed. Please check its status or contact support."})
					return true, nil
				}
				if handled && strings.TrimSpace(cancellationMessage) != "" {
					message = cancellationMessage
				}
			}
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: message})
		}
		returnToEntry(snapshot, session, runtimeContext)
		clearRuntimeVariables(runtimeContext)
		return true, nil
	case commandBack:
		frame, ok := popModuleFrame(runtimeContext.System)
		session.SystemContext = jsonMap(runtimeContext.System)
		if ok && strings.TrimSpace(frame.ReturnStepKey) != "" {
			session.ExpectedInput = ""
			session.CurrentStepKey = frame.ReturnStepKey
			result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: "Going back."})
			return true, nil
		}
		result.Messages = append(result.Messages, OutboundMessage{Type: MessageText, Text: "Returning to the main menu."})
		returnToEntry(snapshot, session, runtimeContext)
		clearRuntimeVariables(runtimeContext)
		return true, nil
	default:
		return false, nil
	}
}

func (s *Service) commandCapturedByCurrentQuestion(snapshot bot.VersionConfiguration, session *ConversationSession, input InboundMessage) bool {
	command := classifyCommand(input.Text)
	if command != commandCancel && command != commandBack {
		return false
	}
	if session.ExpectedInput == "" || session.ExpectedInput == lifecycleTrackEmpty {
		return false
	}
	step, ok := stepByKey(snapshot, session.CurrentStepKey)
	if !ok || step.QuestionID == nil {
		return false
	}
	question, err := findQuestion(snapshot.Questions, step.QuestionID)
	if err != nil {
		return false
	}
	options := parseOptions(s.effectiveQuestionOptions(snapshot, step, question))
	if len(options) == 0 {
		return false
	}
	_, matchErr := matchSingleChoice(input.Text, options)
	return matchErr == nil
}

func clearRuntimeVariables(runtimeContext RuntimeContext) {
	for key := range runtimeContext.Variables {
		delete(runtimeContext.Variables, key)
	}
}

func (s *Service) handleTrackEmptyChoice(snapshot bot.VersionConfiguration, session *ConversationSession, input InboundMessage, runtimeContext RuntimeContext, result *RuntimeResult) bool {
	text := normalizeCommand(input.Text)
	switch text {
	case "1", "order", "place an order", "place order":
		session.ExpectedInput = ""
		clearRuntimeVariables(runtimeContext)
		setPath(runtimeContext.Variables, "intent", "order")
		if key := findStepKeyByModule(snapshot, "ORDER"); key != "" {
			session.CurrentStepKey = key
			delete(runtimeContext.System, "module_stack")
			session.SystemContext = jsonMap(runtimeContext.System)
			return true
		}
		returnToEntry(snapshot, session, runtimeContext)
		return true
	case "2", "support", "contact support", "complaint":
		session.ExpectedInput = ""
		clearRuntimeVariables(runtimeContext)
		setPath(runtimeContext.Variables, "intent", "support")
		if key := findHandoffStep(snapshot); key != "" {
			session.CurrentStepKey = key
			delete(runtimeContext.System, "module_stack")
			session.SystemContext = jsonMap(runtimeContext.System)
			return true
		}
		if key := findStepKeyByModule(snapshot, "CONTACT_SUPPORT"); key != "" {
			session.CurrentStepKey = key
			return true
		}
		returnToEntry(snapshot, session, runtimeContext)
		return true
	case "3", "menu", "main menu":
		session.ExpectedInput = ""
		clearRuntimeVariables(runtimeContext)
		returnToEntry(snapshot, session, runtimeContext)
		return true
	default:
		if classifyCommand(input.Text) != commandNone {
			session.ExpectedInput = ""
			clearRuntimeVariables(runtimeContext)
			returnToEntry(snapshot, session, runtimeContext)
			return true
		}
		result.Messages = append(result.Messages, OutboundMessage{Type: MessageButtons, Text: unrecognizedOptionMessage(), Options: trackEmptyOptions()})
		return true
	}
}

func (s *Service) acceptAnswer(step bot.Step, snapshot bot.VersionConfiguration, session *ConversationSession, input InboundMessage, ctx RuntimeContext) error {
	question, err := findQuestion(snapshot.Questions, step.QuestionID)
	if err != nil {
		return err
	}
	value, err := validateAnswer(question, s.effectiveQuestionOptions(snapshot, step, question), input)
	if err != nil {
		return err
	}
	if question.VariableName != "" {
		setPath(ctx.Variables, question.VariableName, value)
	}
	session.ExpectedInput = ""
	session.CurrentStepKey = step.NextStepKey
	if session.CurrentStepKey == "" {
		session.Status = SessionCompleted
	}
	return nil
}

func validateAnswer(question bot.Question, rawOptions string, input InboundMessage) (any, error) {
	text := strings.TrimSpace(input.Text)
	switch question.Type {
	case "text":
		if text == "" && question.Required {
			return nil, runtimeError(ErrInvalidInput, "Please enter a response.")
		}
		return text, nil
	case "number":
		n, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Please enter a valid number.")
		}
		validation := parseJSONMap(question.Validation)
		if min, ok := numberValue(validation["min"]); ok && n < min {
			return nil, runtimeErrorf(ErrInvalidInput, fmt.Sprintf("Enter a quantity from %.0f to %.0f.", min, numberOrDefault(validation["max"], min)), "number below minimum for %s", question.QuestionKey)
		}
		if max, ok := numberValue(validation["max"]); ok && n > max {
			return nil, runtimeErrorf(ErrInvalidInput, fmt.Sprintf("Enter a quantity from %.0f to %.0f.", numberOrDefault(validation["min"], max), max), "number above maximum for %s", question.QuestionKey)
		}
		return n, nil
	case "email":
		if _, err := mail.ParseAddress(text); err != nil {
			return nil, runtimeError(ErrInvalidInput, "That does not look like a valid email address. Please enter your email address.")
		}
		return strings.ToLower(text), nil
	case "phone":
		cleaned := regexp.MustCompile(`[^0-9+]`).ReplaceAllString(text, "")
		if len(cleaned) < 7 {
			return nil, runtimeError(ErrInvalidInput, "Please enter a valid phone number.")
		}
		return cleaned, nil
	case "yes_no":
		switch strings.ToLower(text) {
		case "yes", "y", "true", "1":
			return true, nil
		case "no", "n", "false", "0":
			return false, nil
		default:
			return nil, runtimeError(ErrInvalidInput, "Please reply yes or no.")
		}
	case "location":
		if input.Location == nil {
			return nil, runtimeError(ErrInvalidInput, "Please share a location.")
		}
		return map[string]any{"latitude": input.Location.Latitude, "longitude": input.Location.Longitude, "name": input.Location.Name, "address": input.Location.Address}, nil
	case "single_choice", "product_selector", "order_selector":
		return matchSingleChoice(text, parseOptions(rawOptions))
	case "multiple_choice":
		options := parseOptions(rawOptions)
		parts := strings.Split(text, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			value, err := matchSingleChoice(strings.TrimSpace(part), options)
			if err != nil {
				return nil, err
			}
			values = append(values, stringValue(value))
		}
		return values, nil
	default:
		return nil, runtimeError(ErrInvalidInput, "That response is not valid.")
	}
}

func numberOrDefault(value any, fallback float64) float64 {
	if n, ok := numberValue(value); ok {
		return n
	}
	return fallback
}

func matchSingleChoice(text string, options []MessageOption) (any, error) {
	if len(options) == 0 {
		if text == "" {
			return nil, runtimeError(ErrInvalidInput, "Please choose an option.")
		}
		return text, nil
	}
	for index, option := range options {
		if text == option.ID || strings.EqualFold(text, option.Label) || text == strconv.Itoa(index+1) {
			return option.ID, nil
		}
	}
	return nil, runtimeError(ErrInvalidInput, unrecognizedOptionMessage())
}

func (s *Service) effectiveQuestionOptions(snapshot bot.VersionConfiguration, step bot.Step, question bot.Question) string {
	if isCustomerEntryMenu(snapshot, step, question) {
		options := customerEntryMenuOptions(snapshot.Modules)
		if len(options) > 0 {
			return jsonValue(options)
		}
	}
	return defaultString(step.Options, question.Options)
}

func isCustomerEntryMenu(snapshot bot.VersionConfiguration, step bot.Step, question bot.Question) bool {
	return step.StepKey == snapshot.Version.StartStepKey && question.QuestionKey == "main_menu"
}

func customerEntryMenuOptions(modules []bot.VersionModule) []MessageOption {
	ordered := append([]bot.VersionModule(nil), modules...)
	sort.SliceStable(ordered, func(left, right int) bool {
		leftOrder := ordered[left].SortOrder
		rightOrder := ordered[right].SortOrder
		if leftOrder == rightOrder {
			return ordered[left].CreatedAt.Before(ordered[right].CreatedAt)
		}
		return leftOrder < rightOrder
	})
	options := make([]MessageOption, 0, len(ordered))
	seen := map[string]struct{}{}
	for _, module := range ordered {
		if !runtimeModuleEnabled(module.Metadata) {
			continue
		}
		intent := moduleMenuIntent(module)
		if intent == "" {
			continue
		}
		if _, exists := seen[intent]; exists {
			continue
		}
		seen[intent] = struct{}{}
		options = append(options, MessageOption{ID: intent, Label: moduleMenuLabel(module), Description: strings.TrimSpace(module.Description)})
	}
	return options
}

func moduleMenuIntent(module bot.VersionModule) string {
	params := parseJSONMap(module.Parameters)
	if intent := strings.TrimSpace(stringValue(params["menu_intent"])); intent != "" {
		return intent
	}
	switch strings.ToUpper(strings.TrimSpace(module.ModuleKey)) {
	case "ORDER":
		return "order"
	case "TRACK_ORDER":
		return "track_order"
	case "FAQ":
		return "faq"
	case "COMPLAINT":
		return "complaint"
	case "HUMAN_HANDOFF", "CONTACT_SUPPORT":
		return "support"
	default:
		return ""
	}
}

func moduleMenuLabel(module bot.VersionModule) string {
	params := parseJSONMap(module.Parameters)
	if label := strings.TrimSpace(stringValue(params["menu_label"])); label != "" {
		return label
	}
	if label := strings.TrimSpace(module.Name); label != "" {
		return label
	}
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(module.ModuleKey), "_", " "))
	for index, word := range words {
		if word == "" {
			continue
		}
		words[index] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func runtimeModuleEnabled(raw string) bool {
	metadata := parseJSONMap(raw)
	value, ok := metadata["enabled"]
	if !ok || value == nil {
		return true
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err != nil || parsed
	default:
		return true
	}
}

func (s *Service) advance(session *ConversationSession, nextStepKey string) bool {
	if nextStepKey == "" {
		return false
	}
	session.CurrentStepKey = nextStepKey
	return true
}

func (s *Service) resolveChannel(ctx context.Context, input InboundMessage) (core.Channel, error) {
	var channel core.Channel
	query := s.db.WithContext(ctx).Where("status IN ?", runtimeUsableChannelStatuses)
	if input.ChannelID != nil {
		err := query.Where("id = ?", *input.ChannelID).First(&channel).Error
		return channel, mapNotFoundCode(err, ErrChannelNotFound, "Channel not found")
	}
	if input.Provider == "" || input.ProviderChannelID == "" {
		return core.Channel{}, runtimeError(ErrChannelNotFound, "Channel could not be resolved.")
	}
	err := query.Where("provider = ? AND phone_number_id = ?", strings.ToLower(strings.TrimSpace(input.Provider)), strings.TrimSpace(input.ProviderChannelID)).First(&channel).Error
	return channel, mapNotFoundCode(err, ErrChannelNotFound, "Channel not found")
}

func (s *Service) getChannelForActor(ctx context.Context, actor auth.CurrentUser, channelID uuid.UUID) (core.Channel, error) {
	var channel core.Channel
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ? AND status IN ?", actor.OrganizationID, channelID, runtimeUsableChannelStatuses).First(&channel).Error
	return channel, mapNotFoundCode(err, ErrChannelNotFound, "Channel not found")
}

func (s *Service) resolvePublishedBot(ctx context.Context, channel core.Channel, preferredBotID *uuid.UUID) (bot.Bot, bot.VersionConfiguration, error) {
	config := parseJSONMap(channel.Config)
	var botID uuid.UUID
	if preferredBotID != nil && *preferredBotID != uuid.Nil {
		botID = *preferredBotID
	} else if raw := stringValue(config["bot_id"]); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return bot.Bot{}, bot.VersionConfiguration{}, runtimeError(ErrRuntimeConfigurationError, "This channel is not configured correctly.")
		}
		botID = parsed
	}
	var botRecord bot.Bot
	query := s.db.WithContext(ctx).Where("organization_id = ? AND status = ?", channel.OrganizationID, bot.BotStatusActive)
	if botID != uuid.Nil {
		query = query.Where("id = ?", botID)
	}
	if err := query.Order("updated_at DESC").First(&botRecord).Error; err != nil {
		return bot.Bot{}, bot.VersionConfiguration{}, mapNotFoundCode(err, ErrBotNotFound, "Bot not found")
	}
	if botRecord.PublishedVersionID == nil {
		return bot.Bot{}, bot.VersionConfiguration{}, runtimeError(ErrNoPublishedVersion, "No published bot version is available.")
	}
	snapshot, err := s.loadSnapshot(ctx, channel.OrganizationID, botRecord.ID, *botRecord.PublishedVersionID)
	return botRecord, snapshot, err
}

func (s *Service) loadSnapshot(ctx context.Context, orgID, botID, versionID uuid.UUID) (bot.VersionConfiguration, error) {
	var published bot.PublishedSnapshot
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND bot_id = ? AND version_id = ?", orgID, botID, versionID).First(&published).Error; err != nil {
		return bot.VersionConfiguration{}, mapNotFoundCode(err, ErrNoPublishedVersion, "Published bot snapshot not found")
	}
	var snapshot bot.VersionConfiguration
	if err := json.Unmarshal([]byte(published.Snapshot), &snapshot); err != nil {
		return bot.VersionConfiguration{}, runtimeErrorf(ErrRuntimeConfigurationError, "This bot is not configured correctly.", "snapshot decode failed: %v", err)
	}
	return snapshot, nil
}

func (s *Service) persistResult(ctx context.Context, session *ConversationSession, input InboundMessage, result RuntimeResult) error {
	now := s.now()
	session.LastMessageAt = &now
	session.UpdatedAt = now
	lastBody := strings.TrimSpace(input.Text)
	lastDirection := DirectionInbound
	if len(result.Messages) > 0 {
		last := result.Messages[len(result.Messages)-1]
		if strings.TrimSpace(last.Text) != "" {
			lastBody = strings.TrimSpace(last.Text)
			lastDirection = DirectionOutbound
		}
	}
	if session.ConversationStatus == "" {
		session.ConversationStatus = conversationStatusForRuntime(*session)
	}
	if session.Priority == "" {
		session.Priority = "normal"
	}
	storeID := session.StoreID
	if storeID == nil {
		storeID = storeIDFromVariables(session.Variables)
	}
	unreadCount := session.UnreadCount
	if input.Text != "" {
		unreadCount++
	}
	if lastDirection == DirectionOutbound && len(result.Messages) > 0 {
		unreadCount = 0
	}
	updates := map[string]any{"current_step_key": session.CurrentStepKey, "expected_input": session.ExpectedInput, "status": session.Status, "conversation_status": session.ConversationStatus, "assigned_user_id": session.AssignedUserID, "priority": sessionPriority(*session), "handoff_state": session.HandoffState, "store_id": storeID, "unread_count": unreadCount, "last_message_body": lastBody, "last_message_direction": lastDirection, "variables": defaultObject(session.Variables), "system_context": defaultObject(session.SystemContext), "last_message_at": &now, "human_owned_at": session.HumanOwnedAt, "human_released_at": session.HumanReleasedAt, "resolved_at": session.ResolvedAt, "updated_at": now, "lock_version": gorm.Expr("lock_version + 1")}
	tx := s.db.WithContext(ctx).Model(&ConversationSession{}).Where("id = ? AND lock_version = ?", session.ID, session.LockVersion).Updates(updates)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return runtimeError(ErrSessionConflict, "This conversation is already being processed. Please try again.")
	}
	session.LockVersion++
	session.UnreadCount = unreadCount
	session.LastMessageBody = lastBody
	session.LastMessageDirection = lastDirection
	session.StoreID = storeID
	inbound := ConversationMessage{ID: uuid.New(), OrganizationID: session.OrganizationID, SessionID: session.ID, ChannelID: session.ChannelID, ExternalMessageID: input.ExternalMessageID, Direction: DirectionInbound, MessageType: MessageText, Sender: input.Sender, Body: input.Text, Metadata: jsonValue(input.Metadata)}
	if err := s.db.WithContext(ctx).Create(&inbound).Error; err != nil {
		return err
	}
	for _, message := range result.Messages {
		outbound := ConversationMessage{ID: uuid.New(), OrganizationID: session.OrganizationID, SessionID: session.ID, ChannelID: session.ChannelID, Direction: DirectionOutbound, MessageType: message.Type, Body: message.Text, Metadata: jsonValue(message)}
		if err := s.db.WithContext(ctx).Create(&outbound).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) findProcessed(ctx context.Context, channel core.Channel, externalMessageID string) (ProcessedMessage, bool, error) {
	var processed ProcessedMessage
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_id = ? AND external_message_id = ?", channel.OrganizationID, channel.ID, externalMessageID).First(&processed).Error
	if err == gorm.ErrRecordNotFound {
		return ProcessedMessage{}, false, nil
	}
	return processed, err == nil, err
}

func (s *Service) markProcessed(ctx context.Context, processed ProcessedMessage, sessionID *uuid.UUID, status string, result RuntimeResult) error {
	return s.db.WithContext(ctx).Model(&processed).Updates(map[string]any{"session_id": sessionID, "status": status, "result": jsonValue(result), "updated_at": s.now()}).Error
}

func (s *Service) claimFailedProcessed(ctx context.Context, processed ProcessedMessage) (bool, error) {
	result := s.db.WithContext(ctx).Model(&ProcessedMessage{}).
		Where("id = ? AND organization_id = ? AND channel_id = ? AND status = ?", processed.ID, processed.OrganizationID, processed.ChannelID, "failed").
		Updates(map[string]any{"session_id": nil, "status": "processing", "result": "{}", "updated_at": s.now()})
	return result.RowsAffected == 1, result.Error
}

func (s *Service) recordEvent(ctx context.Context, session ConversationSession, eventType, severity, stepKey, actionKey string, metadata map[string]any) {
	event := RuntimeEvent{ID: uuid.New(), OrganizationID: session.OrganizationID, SessionID: &session.ID, BotID: &session.BotID, BotVersionID: &session.BotVersionID, ChannelID: &session.ChannelID, EventType: eventType, Severity: defaultString(severity, "info"), StepKey: stepKey, ActionKey: actionKey, Metadata: jsonValue(metadata)}
	_ = s.db.WithContext(ctx).Create(&event).Error
}

func (s *Service) validationMessage(snapshot bot.VersionConfiguration, err error) OutboundMessage {
	message := "That response was not valid. Please try again."
	if runtimeErr, ok := err.(RuntimeError); ok && runtimeErr.Message != "" {
		message = runtimeErr.Message
	}
	return OutboundMessage{Type: MessageText, Text: defaultString(message, fallbackMessageText(snapshot))}
}

func (s *Service) fallbackMessage(snapshot bot.VersionConfiguration, err error) OutboundMessage {
	message := fallbackMessageText(snapshot)
	if runtimeErr, ok := err.(RuntimeError); ok && runtimeErr.Message != "" {
		message = runtimeErr.Message
	}
	return OutboundMessage{Type: MessageText, Text: message}
}

func indexSteps(steps []bot.Step) map[string]bot.Step {
	index := map[string]bot.Step{}
	for _, step := range steps {
		index[step.StepKey] = step
	}
	return index
}

func findQuestion(questions []bot.Question, id *uuid.UUID) (bot.Question, error) {
	if id == nil {
		return bot.Question{}, runtimeError(ErrRuntimeConfigurationError, "This bot question is not configured correctly.")
	}
	for _, question := range questions {
		if question.ID == *id {
			return question, nil
		}
	}
	return bot.Question{}, runtimeError(ErrRuntimeConfigurationError, "This bot question is not configured correctly.")
}

func findAction(actions []bot.Action, id *uuid.UUID) (bot.Action, error) {
	if id == nil {
		return bot.Action{}, runtimeError(ErrRuntimeConfigurationError, "This bot action is not configured correctly.")
	}
	for _, action := range actions {
		if action.ID == *id {
			return action, nil
		}
	}
	return bot.Action{}, runtimeError(ErrRuntimeConfigurationError, "This bot action is not configured correctly.")
}

func findCondition(conditions []bot.Condition, id *uuid.UUID) (bot.Condition, error) {
	if id == nil {
		return bot.Condition{}, runtimeError(ErrRuntimeConfigurationError, "This bot condition is not configured correctly.")
	}
	for _, condition := range conditions {
		if condition.ID == *id {
			return condition, nil
		}
	}
	return bot.Condition{}, runtimeError(ErrRuntimeConfigurationError, "This bot condition is not configured correctly.")
}

func findModule(modules []bot.VersionModule, id *uuid.UUID) (bot.VersionModule, error) {
	if id == nil {
		return bot.VersionModule{}, runtimeError(ErrRuntimeConfigurationError, "This bot module is not configured correctly.")
	}
	for _, module := range modules {
		if module.ID == *id {
			return module, nil
		}
	}
	return bot.VersionModule{}, runtimeError(ErrRuntimeConfigurationError, "This bot module is not configured correctly.")
}

func parseOptions(raw string) []MessageOption {
	raw = defaultArray(raw)
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	options := make([]MessageOption, 0, len(values))
	for index, value := range values {
		switch v := value.(type) {
		case string:
			options = append(options, MessageOption{ID: v, Label: v})
		case map[string]any:
			id := defaultString(stringValue(v["id"]), strconv.Itoa(index+1))
			label := defaultString(stringValue(v["label"]), id)
			options = append(options, MessageOption{ID: id, Label: label, Description: stringValue(v["description"])})
		}
	}
	return options
}

func fallbackMessageText(snapshot bot.VersionConfiguration) string {
	config := parseJSONMap(snapshot.Fallback)
	return defaultString(stringValue(config["message"]), "Sorry, something went wrong while processing that. Please try again or speak to a member of our team.")
}

func fallbackVariable(snapshot bot.VersionConfiguration) string {
	config := parseJSONMap(snapshot.Fallback)
	return defaultString(stringValue(config["missing_variable"]), "unavailable")
}

func defaultObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "{}"
	}
	return raw
}

func defaultArray(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]"
	}
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return "[]"
	}
	return raw
}

func publicRuntimeError(err error) *RuntimeError {
	if err == nil {
		return nil
	}
	if runtimeErr, ok := err.(RuntimeError); ok {
		return &RuntimeError{Code: runtimeErr.Code, Message: runtimeErr.Message}
	}
	return &RuntimeError{Code: ErrActionFailed, Message: "Sorry, something went wrong while processing that."}
}

func mapNotFoundCode(err error, code string, message string) error {
	if err == nil {
		return nil
	}
	if err == gorm.ErrRecordNotFound {
		return runtimeError(code, message)
	}
	return err
}

func canUseRuntimeSimulator(role authz.Role) bool {
	return role == authz.PlatformAdmin || role == authz.MerchantAdmin || role == authz.SupportAgent
}

func mergeMaps(left map[string]any, right map[string]any) map[string]any {
	if left == nil {
		left = map[string]any{}
	}
	for key, value := range right {
		left[key] = value
	}
	return left
}

func scrubOutputs(outputs map[string]any) map[string]any {
	scrubbed := map[string]any{}
	for key, value := range outputs {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") || strings.Contains(normalized, "password") {
			continue
		}
		scrubbed[key] = value
	}
	return scrubbed
}

func formatRuntimeReference(prefix string, id uuid.UUID) string {
	return fmt.Sprintf("%s-%s", prefix, strings.ReplaceAll(id.String(), "-", ""))
}

type moduleFrame struct {
	ModuleKey     string `json:"module_key"`
	CallerStepKey string `json:"caller_step_key"`
	ReturnStepKey string `json:"return_step_key"`
}

func pushModuleFrame(system map[string]any, moduleKey, callerStepKey, returnStepKey string) error {
	stack := moduleStack(system)
	if len(stack) >= 10 {
		return runtimeError(ErrRuntimeConfigurationError, "This bot has too many nested modules.")
	}
	stack = append(stack, moduleFrame{ModuleKey: moduleKey, CallerStepKey: callerStepKey, ReturnStepKey: returnStepKey})
	system["module_stack"] = stack
	return nil
}

func popModuleFrame(system map[string]any) (moduleFrame, bool) {
	stack := moduleStack(system)
	if len(stack) == 0 {
		return moduleFrame{}, false
	}
	frame := stack[len(stack)-1]
	stack = stack[:len(stack)-1]
	if len(stack) == 0 {
		delete(system, "module_stack")
	} else {
		system["module_stack"] = stack
	}
	return frame, true
}

func moduleStack(system map[string]any) []moduleFrame {
	raw, ok := system["module_stack"]
	if !ok {
		return nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var stack []moduleFrame
	if err := json.Unmarshal(body, &stack); err != nil {
		return nil
	}
	return stack
}
