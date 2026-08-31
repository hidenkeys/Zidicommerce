package ai

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai/provider"
	aitools "github.com/hidenkeys/zidicommerce/apps/api/internal/ai/tools"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"gorm.io/gorm"
)

const (
	localChannelProvider = "local_test"
	localChannelName     = "AI Local Test"
	localBotName         = "Zidi AI Local Test"
)

type Service struct {
	db                *gorm.DB
	commerce          *core.Service
	chatProvider      provider.ChatProvider
	tools             *aitools.Registry
	maxToolCalls      int
	embeddingProvider EmbeddingProvider
	embeddingOptions  EmbeddingOptions
	now               func() time.Time
}

func NewService(db *gorm.DB, commerce *core.Service, chatProvider provider.ChatProvider, maxToolCalls int) *Service {
	if maxToolCalls <= 0 {
		maxToolCalls = 4
	}
	return &Service{
		db:                db,
		commerce:          commerce,
		chatProvider:      chatProvider,
		tools:             aitools.NewRegistry(db, commerce),
		maxToolCalls:      maxToolCalls,
		embeddingProvider: disabledEmbeddingProvider{},
		embeddingOptions:  normalizeEmbeddingOptions(EmbeddingOptions{}),
		now:               func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Start(ctx context.Context, actor auth.CurrentUser, input StartInput) (SessionResponse, error) {
	if !canUseAIChat(actor.Role) {
		return SessionResponse{}, httperror.Forbidden("You cannot use the AI test chat")
	}
	organizationID, err := s.resolveOrganization(actor, input.OrganizationID)
	if err != nil {
		return SessionResponse{}, err
	}
	if input.CustomerID != nil {
		if err := s.ensureCustomer(ctx, organizationID, *input.CustomerID); err != nil {
			return SessionResponse{}, err
		}
	}
	channel, localBot, version, err := s.ensureLocalRuntimeAssets(ctx, organizationID, actor.ID)
	if err != nil {
		return SessionResponse{}, err
	}
	now := s.now()
	session := runtime.ConversationSession{
		ID:                     uuid.New(),
		OrganizationID:         organizationID,
		BotID:                  localBot.ID,
		BotVersionID:           version.ID,
		ChannelID:              channel.ID,
		CustomerID:             input.CustomerID,
		ExternalConversationID: "ai-local-" + uuid.NewString(),
		CurrentStepKey:         "ai",
		Status:                 runtime.SessionActive,
		ConversationStatus:     runtime.ConversationAIHandling,
		Priority:               "normal",
		Variables:              "{}",
		SystemContext:          "{}",
		LockVersion:            1,
		LastMessageAt:          &now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return SessionResponse{}, err
	}
	return SessionResponse{SessionID: session.ID, OrganizationID: session.OrganizationID, CustomerID: session.CustomerID}, nil
}

func (s *Service) Message(ctx context.Context, actor auth.CurrentUser, input MessageInput) (ChatResponse, error) {
	if strings.TrimSpace(input.Text) == "" {
		return ChatResponse{}, httperror.BadRequest("Message text is required")
	}
	session, err := s.getSessionForActor(ctx, actor, input.SessionID)
	if err != nil {
		return ChatResponse{}, err
	}
	now := s.now()
	inbound := runtime.ConversationMessage{
		ID:                uuid.New(),
		OrganizationID:    session.OrganizationID,
		SessionID:         session.ID,
		ChannelID:         session.ChannelID,
		ExternalMessageID: "ai-in-" + uuid.NewString(),
		Direction:         runtime.DirectionInbound,
		MessageType:       runtime.MessageText,
		Sender:            "local_test_customer",
		Body:              strings.TrimSpace(input.Text),
		Metadata:          jsonValue(map[string]any{"source": "ai_test_chat"}),
		CreatedAt:         now,
	}
	if session.CustomerID != nil {
		inbound.Sender = session.CustomerID.String()
	}
	if err := s.db.WithContext(ctx).Create(&inbound).Error; err != nil {
		return ChatResponse{}, err
	}
	if session.AIShouldPause() || s.hasActiveHumanHandoff(ctx, session) {
		if err := s.db.WithContext(ctx).Model(&runtime.ConversationSession{}).
			Where("organization_id = ? AND id = ?", session.OrganizationID, session.ID).
			Updates(map[string]any{
				"last_message_at":        inbound.CreatedAt,
				"last_message_body":      inbound.Body,
				"last_message_direction": runtime.DirectionInbound,
				"unread_count":           gorm.Expr("unread_count + 1"),
				"updated_at":             inbound.CreatedAt,
			}).Error; err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{
			SessionID:      session.ID,
			OrganizationID: session.OrganizationID,
			CustomerID:     session.CustomerID,
			Message:        rowFromMessage(inbound),
			Debug:          InteractionDebug{Provider: s.chatProvider.Name(), Model: s.chatProvider.Model(), Error: "ai_paused_human_handoff_active"},
		}, nil
	}

	debug := InteractionDebug{Provider: s.chatProvider.Name(), Model: s.chatProvider.Model()}
	start := s.now()
	answer, toolDebug, err := s.complete(ctx, session)
	debug.LatencyMS = s.now().Sub(start).Milliseconds()
	debug.Tools = convertToolDebug(toolDebug)
	if err != nil {
		debug.Error = err.Error()
		return ChatResponse{}, err
	}
	outbound := runtime.ConversationMessage{
		ID:                uuid.New(),
		OrganizationID:    session.OrganizationID,
		SessionID:         session.ID,
		ChannelID:         session.ChannelID,
		ExternalMessageID: "ai-out-" + uuid.NewString(),
		Direction:         runtime.DirectionOutbound,
		MessageType:       runtime.MessageText,
		Sender:            "zidi_ai",
		Body:              answer,
		Metadata:          jsonValue(map[string]any{"source": "ai_test_chat", "provider": debug.Provider, "model": debug.Model, "latency_ms": debug.LatencyMS, "tools": debug.Tools}),
		CreatedAt:         s.now(),
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&outbound).Error; err != nil {
			return err
		}
		return tx.Model(&runtime.ConversationSession{}).
			Where("organization_id = ? AND id = ?", session.OrganizationID, session.ID).
			Updates(map[string]any{
				"last_message_at":        outbound.CreatedAt,
				"last_message_body":      outbound.Body,
				"last_message_direction": runtime.DirectionOutbound,
				"updated_at":             outbound.CreatedAt,
			}).Error
	}); err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		SessionID:      session.ID,
		OrganizationID: session.OrganizationID,
		CustomerID:     session.CustomerID,
		Message:        rowFromMessage(outbound),
		Debug:          debug,
	}, nil
}

// RuntimeReply reuses the grounded read-only AI path for a channel runtime
// entry message. Runtime remains responsible for persistence and all commerce
// mutations; the AI tool registry cannot execute write actions.
func (s *Service) RuntimeReply(ctx context.Context, session runtime.ConversationSession, text string) (string, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", session.Variables, nil
	}
	result, err := s.orchestrate(ctx, session, []provider.Message{{Role: "user", Content: text}})
	if err != nil {
		return "", session.Variables, err
	}
	return strings.TrimSpace(result.Answer), storeAIState(session, result.State), nil
}

func (s *Service) Messages(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) ([]ConversationRow, error) {
	session, err := s.getSessionForActor(ctx, actor, sessionID)
	if err != nil {
		return nil, err
	}
	var messages []runtime.ConversationMessage
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ?", session.OrganizationID, session.ID).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	rows := make([]ConversationRow, 0, len(messages))
	for _, message := range messages {
		rows = append(rows, rowFromMessage(message))
	}
	return rows, nil
}

func (s *Service) hasActiveHumanHandoff(ctx context.Context, session runtime.ConversationSession) bool {
	var count int64
	err := s.db.WithContext(ctx).
		Table("support_handoffs").
		Where("organization_id = ? AND session_id = ? AND status IN ?", session.OrganizationID, session.ID, runtime.ActiveHandoffStatuses()).
		Count(&count).Error
	return err == nil && count > 0
}

func (s *Service) complete(ctx context.Context, session runtime.ConversationSession) (string, []aitools.DebugCall, error) {
	history, err := s.loadHistory(ctx, session)
	if err != nil {
		return "", nil, err
	}
	result, err := s.orchestrate(ctx, session, history)
	if err != nil {
		return "", nil, httperror.Unavailable(err.Error())
	}
	variables := storeAIState(session, result.State)
	if err := s.db.WithContext(ctx).Model(&runtime.ConversationSession{}).
		Where("organization_id = ? AND id = ?", session.OrganizationID, session.ID).
		Update("variables", variables).Error; err != nil {
		return "", result.Debug, err
	}
	return result.Answer, result.Debug, nil
}

func (s *Service) loadHistory(ctx context.Context, session runtime.ConversationSession) ([]provider.Message, error) {
	var rows []runtime.ConversationMessage
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND session_id = ?", session.OrganizationID, session.ID).
		Order("created_at DESC").
		Limit(12).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	messages := make([]provider.Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		role := "assistant"
		if rows[i].Direction == runtime.DirectionInbound {
			role = "user"
		}
		messages = append(messages, provider.Message{Role: role, Content: rows[i].Body})
	}
	return messages, nil
}

func latestUserMessage(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func copyToolInputs(inputs map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range inputs {
		out[key] = value
	}
	return out
}

func (s *Service) resolveOrganization(actor auth.CurrentUser, requested uuid.UUID) (uuid.UUID, error) {
	if actor.Role == authz.PlatformAdmin {
		if requested == uuid.Nil {
			return actor.OrganizationID, nil
		}
		return requested, nil
	}
	if requested != uuid.Nil && requested != actor.OrganizationID {
		return uuid.Nil, httperror.Forbidden("You cannot start AI chat for another organization")
	}
	return actor.OrganizationID, nil
}

func (s *Service) getSessionForActor(ctx context.Context, actor auth.CurrentUser, sessionID uuid.UUID) (runtime.ConversationSession, error) {
	if !canUseAIChat(actor.Role) {
		return runtime.ConversationSession{}, httperror.Forbidden("You cannot use the AI test chat")
	}
	var session runtime.ConversationSession
	query := s.db.WithContext(ctx).Where("id = ?", sessionID)
	if actor.Role != authz.PlatformAdmin {
		query = query.Where("organization_id = ?", actor.OrganizationID)
	}
	if err := query.First(&session).Error; err != nil {
		return runtime.ConversationSession{}, httperror.NotFound("AI chat session not found")
	}
	if actor.Role == authz.PlatformAdmin || session.OrganizationID == actor.OrganizationID {
		return session, nil
	}
	return runtime.ConversationSession{}, httperror.Forbidden("You cannot access this AI chat session")
}

func (s *Service) ensureCustomer(ctx context.Context, organizationID, customerID uuid.UUID) error {
	var customer core.Customer
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, customerID).First(&customer).Error; err != nil {
		return httperror.BadRequest("Selected customer does not belong to the organization")
	}
	return nil
}

func (s *Service) ensureLocalRuntimeAssets(ctx context.Context, organizationID, actorID uuid.UUID) (core.Channel, bot.Bot, bot.BotVersion, error) {
	var channel core.Channel
	var localBot bot.Bot
	var version bot.BotVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if err := tx.Where("organization_id = ? AND provider = ? AND display_name = ?", organizationID, localChannelProvider, localChannelName).First(&channel).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			channel = core.Channel{
				ID:             uuid.New(),
				OrganizationID: organizationID,
				Provider:       localChannelProvider,
				DisplayName:    localChannelName,
				Status:         core.StatusActive,
				Config:         "{}",
				SecretConfig:   "{}",
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&channel).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("organization_id = ? AND name = ?", organizationID, localBotName).First(&localBot).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			localBot = bot.Bot{
				ID:              uuid.New(),
				OrganizationID:  organizationID,
				Name:            localBotName,
				Description:     "Local AI chat test bot",
				Status:          bot.BotStatusActive,
				DefaultLanguage: "en",
				Timezone:        "Africa/Lagos",
				FallbackConfig:  "{}",
				HandoffConfig:   "{}",
				Metadata:        jsonValue(map[string]any{"source": "ai_test_chat"}),
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if err := tx.Create(&localBot).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("organization_id = ? AND bot_id = ? AND version_number = ?", organizationID, localBot.ID, 1).First(&version).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			var createdBy *uuid.UUID
			if actorID != uuid.Nil {
				createdBy = &actorID
			}
			version = bot.BotVersion{
				ID:               uuid.New(),
				OrganizationID:   organizationID,
				BotID:            localBot.ID,
				VersionNumber:    1,
				Status:           bot.VersionStatusPublished,
				StartStepKey:     "ai",
				ValidationErrors: "[]",
				Metadata:         jsonValue(map[string]any{"source": "ai_test_chat"}),
				CreatedByUserID:  createdBy,
				PublishedAt:      &now,
				CreatedAt:        now,
				UpdatedAt:        now,
			}
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
		}
		if localBot.PublishedVersionID == nil || *localBot.PublishedVersionID != version.ID {
			if err := tx.Model(&bot.Bot{}).Where("organization_id = ? AND id = ?", organizationID, localBot.ID).Updates(map[string]any{"published_version_id": version.ID, "updated_at": now}).Error; err != nil {
				return err
			}
			localBot.PublishedVersionID = &version.ID
		}
		return nil
	})
	return channel, localBot, version, err
}

func canUseAIChat(role authz.Role) bool {
	return role.HasPermission(authz.PermissionAIUse)
}

func rowFromMessage(message runtime.ConversationMessage) ConversationRow {
	role := "assistant"
	if message.Direction == runtime.DirectionInbound {
		role = "user"
	}
	return ConversationRow{ID: message.ID, Role: role, Body: message.Body, Metadata: message.Metadata, CreatedAt: message.CreatedAt}
}

func convertToolDebug(calls []aitools.DebugCall) []ToolCallDebug {
	out := make([]ToolCallDebug, 0, len(calls))
	for _, call := range calls {
		out = append(out, ToolCallDebug{Name: call.Name, Inputs: call.Inputs, Result: call.Result, Status: call.Status, Error: call.Error, LatencyMS: call.LatencyMS})
	}
	return out
}

func jsonValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
