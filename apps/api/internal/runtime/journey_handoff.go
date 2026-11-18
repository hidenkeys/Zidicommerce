package runtime

import (
	"context"
	"regexp"
	"strings"
)

// Explicit requests take precedence over order tracking and model generation.
var humanRequest = regexp.MustCompile(`\b(?:speak|talk|chat|connect me|transfer me) (?:with|to) (?:a |an |your |the )?(?:human|person|agent|staff|team member|representative|someone)\b`)

func requestsHumanAssistance(text string) bool {
	normalized := normalizeCommand(text)
	for _, negation := range []string{"dont ", "do not ", "no need to ", "rather not ", "not want "} {
		if strings.Contains(normalized, negation) {
			return false
		}
	}
	switch normalized {
	case "human", "human support", "human agent", "talk to staff", "agent please", "customer support":
		return true
	}
	return humanRequest.MatchString(normalized)
}

func (s *Service) handoffJourney(ctx context.Context, session *ConversationSession, reason string) (RuntimeResult, error) {
	rctx := RuntimeContext{Session: *session, Variables: parseJSONMap(session.Variables), System: parseJSONMap(session.SystemContext)}
	if err := s.openSupportHandoff(ctx, *session, reason, rctx); err != nil {
		return RuntimeResult{}, err
	}
	session.Status = SessionHandoff
	session.ConversationStatus = ConversationHumanRequested
	session.HandoffState = "open"
	s.recordEvent(ctx, *session, EventHandoffStarted, "info", session.CurrentStepKey, "", nil)
	return RuntimeResult{ConversationID: session.ID, SessionStatus: session.Status, Handoff: true, Messages: []OutboundMessage{{Type: MessageText, Text: "I’ve passed your conversation to the team. A staff member can continue here."}}}, nil
}
