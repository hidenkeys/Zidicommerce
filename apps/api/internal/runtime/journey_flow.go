package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

type JourneyFlowRequest struct {
	Version   string `json:"version"`
	Action    string `json:"action"`
	Screen    string `json:"screen"`
	FlowToken string `json:"flow_token"`
	Data      struct {
		Answer string `json:"answer"`
		Turn   string `json:"turn"`
	} `json:"data"`
}

func (s *Service) launchJourneyFlow(ctx context.Context, session ConversationSession) (OutboundMessage, error) {
	var deployment bot.JourneyFlowDeployment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND version_id = ? AND connection_id = ? AND status = ?", session.OrganizationID, session.BotVersionID, session.ChannelID, "published").First(&deployment).Error; err != nil {
		return OutboundMessage{}, httperror.BadRequest("Journey Flow is unavailable")
	}
	token := opaqueJourneyID() + opaqueJourneyID()
	expires := s.now().Add(24 * time.Hour)
	if session.ExpiresAt != nil {
		expires = *session.ExpiresAt
	}
	row := JourneyFlowSession{ID: uuid.New(), OrganizationID: session.OrganizationID, ConnectionID: session.ChannelID, ConversationID: session.ID, VersionID: session.BotVersionID, TokenHash: flowTokenHash(token), ExpiresAt: expires}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return OutboundMessage{}, err
	}
	return OutboundMessage{Type: "flow", Text: "Continue your request securely in WhatsApp.", Metadata: map[string]any{"flow_id": deployment.ProviderFlowID, "flow_token": token, "flow_cta": "Continue", "flow_action": "data_exchange"}}, nil
}

// The unguessable token binds the encrypted request to a specific tenant,
// connection, customer conversation and immutable journey version.
func (s *Service) ExchangeJourneyFlow(ctx context.Context, organizationID, connectionID uuid.UUID, request JourneyFlowRequest) (map[string]any, error) {
	if request.Version != "3.0" {
		return nil, httperror.BadRequest("Unsupported Flow data version")
	}
	if request.Action == "ping" {
		return map[string]any{"version": "3.0", "data": map[string]any{"status": "active"}}, nil
	}
	if !containsJourney([]string{"INIT", "BACK", "data_exchange"}, request.Action) {
		return nil, httperror.BadRequest("Unsupported Flow action")
	}
	var flow JourneyFlowSession
	if len(request.FlowToken) > 256 || s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND token_hash = ? AND expires_at > ?", organizationID, connectionID, flowTokenHash(request.FlowToken), s.now()).First(&flow).Error != nil {
		return nil, httperror.Forbidden("This Flow session has expired")
	}
	var session ConversationSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_id = ? AND id = ?", organizationID, connectionID, flow.ConversationID).First(&session).Error; err != nil {
		return nil, httperror.NotFound("Flow conversation not found")
	}
	if session.BotVersionID != flow.VersionID {
		return nil, httperror.Forbidden("Flow version changed")
	}
	config, err := s.loadSnapshot(ctx, organizationID, session.BotID, flow.VersionID)
	if err != nil {
		return nil, err
	}
	result := RuntimeResult{}
	if request.Action != "INIT" {
		key := "flow:" + flowTokenHash(jsonValue(request))
		// ProcessMessage also records inbound events. This replay receipt includes
		// the screen response so old retries cannot accidentally advance again.
		var receipt JourneyExchangeReceipt
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND session_id = ? AND event_key = ?", organizationID, flow.ID, key).First(&receipt).Error; err == nil {
			var response map[string]any
			err = json.Unmarshal([]byte(receipt.Result), &response)
			if err == nil {
				var channel core.Channel
				if err = s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, connectionID).First(&channel).Error; err == nil {
					if processed, found, e := s.findProcessed(ctx, channel, key); e != nil {
						err = e
					} else if found && processed.Status == "processed" {
						var original RuntimeResult
						if err = json.Unmarshal([]byte(processed.Result), &original); err == nil {
							err = s.dispatchJourneyFlow(ctx, channel, session, key, original)
						}
					}
				}
			}
			return response, err
		} else if err != gorm.ErrRecordNotFound {
			return nil, err
		}
		validScreen := request.Screen == bot.FlowScreenID(session.CurrentStepKey)
		if request.Action == "BACK" {
			history := mapSlice(parseJSONMap(session.SystemContext)["journey_history"])
			if len(history) > 0 {
				validScreen = validScreen || request.Screen == bot.FlowScreenID(stringValue(history[len(history)-1]["step"]))
			}
		}
		if !validScreen || (request.Action == "data_exchange" && request.Data.Turn != journeyTurn(session)) {
			return nil, httperror.BadRequest("This Flow selection is stale")
		}
		text := request.Data.Answer
		if request.Action == "BACK" {
			text = "back"
		}
		turn := request.Data.Turn
		if request.Action == "BACK" {
			turn = journeyTurn(session)
		}
		result, err = s.ProcessMessage(ctx, InboundMessage{ChannelID: &connectionID, TrustedOrganizationID: &organizationID, ExternalMessageID: key, ExternalConversationID: session.ExternalConversationID, Sender: stringValue(parseJSONMap(session.SystemContext)["sender"]), Text: text, Metadata: map[string]any{"journey_flow": true, "journey_turn": turn}})
		if err != nil {
			return nil, err
		}
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, session.ID).First(&session).Error; err != nil {
			return nil, err
		}
		response := journeyFlowResponse(config, session, result)
		if err := s.db.WithContext(ctx).Create(&JourneyExchangeReceipt{ID: uuid.New(), OrganizationID: organizationID, SessionID: flow.ID, EventKey: key, Result: jsonValue(response)}).Error; err != nil {
			return nil, err
		}
		// Payment links and final confirmations use the existing durable outbox.
		var channel core.Channel
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, connectionID).First(&channel).Error; err != nil {
			return nil, err
		}
		if err := s.dispatchJourneyFlow(ctx, channel, session, key, result); err != nil {
			return nil, err
		}
		return response, nil
	}
	return journeyFlowResponse(config, session, result), nil
}

func (s *Service) dispatchJourneyFlow(ctx context.Context, channel core.Channel, session ConversationSession, key string, result RuntimeResult) error {
	if result.SessionStatus == SessionActive && !strings.Contains(strings.ToLower(joinJourneyMessages(result.Messages)), "payment link") {
		return nil
	}
	_, err := s.DispatchOutbound(ctx, channel, InboundMessage{ExternalMessageID: key, ExternalConversationID: session.ExternalConversationID, Sender: stringValue(parseJSONMap(session.SystemContext)["sender"])}, result)
	return err
}

func journeyFlowResponse(config bot.VersionConfiguration, session ConversationSession, result RuntimeResult) map[string]any {
	if session.Status != SessionActive {
		return map[string]any{"version": "3.0", "screen": "FINISH", "data": map[string]any{"prompt": defaultString(joinJourneyMessages(result.Messages), "Your request has been received. Check this chat for updates.")}}
	}
	var step bot.Step
	for _, candidate := range config.Steps {
		if candidate.StepKey == session.CurrentStepKey {
			step = candidate
			break
		}
	}
	settings := bot.JourneyStepSettings(step)
	system := parseJSONMap(session.SystemContext)
	options := []map[string]string{}
	for _, option := range mapSlice(system["journey_options"]) {
		options = append(options, map[string]string{"id": stringValue(option["id"]), "title": stringValue(option["label"])})
	}
	return map[string]any{"version": "3.0", "screen": bot.FlowScreenID(session.CurrentStepKey), "data": map[string]any{"prompt": defaultString(joinJourneyMessages(result.Messages), settings.Text), "error": "", "turn": journeyTurn(session), "options": options}}
}
func joinJourneyMessages(messages []OutboundMessage) string {
	out := []string{}
	for _, message := range messages {
		if strings.TrimSpace(message.Text) != "" {
			out = append(out, message.Text)
		}
	}
	return strings.Join(out, "\n\n")
}

func (s *Service) journeyRoute(ctx context.Context, channel core.Channel, text string) (*uuid.UUID, error) {
	// Only databases upgraded to R1 participate; legacy tests/bots retain their
	// established routing. No process-local state participates in sessions.
	if !s.db.Migrator().HasTable(&bot.JourneyBinding{}) {
		return nil, nil
	}
	intent := classifyNaturalEntryIntent(text)
	normalized := strings.ToLower(strings.TrimSpace(text))
	if intent == "" {
		switch normalized {
		case "order", "shop", "buy":
			intent = "order"
		case "track", "track_order":
			intent = "track_order"
		case "catalogue", "catalog", "browse":
			intent = "catalogue"
		case "support", "help":
			intent = "support"
		case "complaint":
			intent = "complaint"
		}
	}
	if intent == "" {
		intent = "default"
	}
	var binding bot.JourneyBinding
	err := s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND trigger = ?", channel.OrganizationID, channel.ID, intent).First(&binding).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if binding.Paused {
		return nil, httperror.BadRequest("This journey is paused. Please contact the merchant")
	}
	return &binding.BotID, nil
}

func (s *Service) selectJourneyEntry(ctx context.Context, session *ConversationSession, intent string, system map[string]any) (*bot.VersionConfiguration, error) {
	if intent == "__ai" || intent == "__human" {
		return nil, nil
	}
	var binding bot.JourneyBinding
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND trigger = ? AND paused = ?", session.OrganizationID, session.ChannelID, intent, false).First(&binding).Error; err != nil {
		return nil, httperror.BadRequest("This journey is unavailable; use the current menu")
	}
	config, err := s.loadSnapshot(ctx, session.OrganizationID, binding.BotID, binding.VersionID)
	if err != nil {
		return nil, err
	}
	session.BotID, session.BotVersionID, session.CurrentStepKey, session.ExpectedInput = binding.BotID, binding.VersionID, config.Version.StartStepKey, ""
	for _, key := range []string{"journey_options", "journey_history", "journey_turn", "journey_retries", "journey_flow_launched"} {
		delete(system, key)
	}
	system["journey_mode"], system["bot_id"], system["bot_version_id"] = binding.Mode, binding.BotID.String(), binding.VersionID.String()
	if system["journey_preview"] == true {
		system["journey_mode"] = "guided_chat"
	}
	return &config, nil
}

func (s *Service) JourneyEntryMenu(ctx context.Context, organizationID, connectionID uuid.UUID) (OutboundMessage, error) {
	var bindings []bot.JourneyBinding
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND paused = ?", organizationID, connectionID, false).Order("trigger ASC").Find(&bindings).Error; err != nil {
		return OutboundMessage{}, err
	}
	message := OutboundMessage{Type: MessageList, Text: "Welcome. What would you like to do?"}
	for _, binding := range bindings {
		message.Options = append(message.Options, MessageOption{ID: binding.Trigger, Label: strings.Title(strings.ReplaceAll(binding.Trigger, "_", " "))})
	}
	if len(message.Options) > 10 {
		return message, fmt.Errorf("Entry menu exceeds the provider option limit")
	}
	return message, nil
}
