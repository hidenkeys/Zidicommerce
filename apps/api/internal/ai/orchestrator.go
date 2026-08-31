package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai/prompts"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai/provider"
	aitools "github.com/hidenkeys/zidicommerce/apps/api/internal/ai/tools"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
)

type orchestrationResult struct {
	Answer     string
	Debug      []aitools.DebugCall
	Grounding  GroundingContext
	State      AIConversationState
	Validation ValidationResult
}

func (s *Service) orchestrate(ctx context.Context, session runtime.ConversationSession, history []provider.Message) (orchestrationResult, error) {
	latestUserText := latestUserMessage(history)
	state := loadAIState(session)
	security := classifySecurity(latestUserText)
	retriever := newRetrievalEngineWithKnowledge(s.db, s.commerce, s.knowledgeRetriever())
	grounding, _, err := retriever.BuildGrounding(ctx, session, latestUserText, state, security)
	if err != nil {
		return orchestrationResult{}, err
	}
	debug := debugFromGrounding(grounding)
	if security.Blocked {
		answer := deterministicFallback(grounding)
		state = updateAIStateFromGrounding(state, grounding)
		return orchestrationResult{Answer: answer, Debug: debug, Grounding: grounding, State: state, Validation: ValidationResult{Valid: true}}, nil
	}
	if grounding.Intent == IntentOutOfScope {
		answer := deterministicFallback(grounding)
		state = updateAIStateFromGrounding(state, grounding)
		return orchestrationResult{Answer: answer, Debug: debug, Grounding: grounding, State: state, Validation: ValidationResult{Valid: true}}, nil
	}
	if canAnswerDeterministically(grounding) {
		answer := deterministicFallback(grounding)
		validation := validateCustomerResponse(answer, grounding)
		if validation.Valid {
			state = updateAIStateFromGrounding(state, grounding)
			return orchestrationResult{Answer: answer, Debug: debug, Grounding: grounding, State: state, Validation: validation}, nil
		}
	}
	answer, validation, err := s.generateValidatedAnswer(ctx, history, grounding)
	if err != nil {
		return orchestrationResult{}, err
	}
	state = updateAIStateFromGrounding(state, grounding)
	return orchestrationResult{Answer: answer, Debug: debug, Grounding: grounding, State: state, Validation: validation}, nil
}

func (s *Service) generateValidatedAnswer(ctx context.Context, history []provider.Message, grounding GroundingContext) (string, ValidationResult, error) {
	messages := buildGenerationMessages(history, grounding, "")
	response, err := s.chatProvider.Complete(ctx, provider.ChatRequest{Messages: messages})
	if err != nil {
		return "", ValidationResult{}, err
	}
	answer := strings.TrimSpace(response.Message.Content)
	validation := validateCustomerResponse(answer, grounding)
	if validation.Valid {
		return answer, validation, nil
	}
	retryInstruction := fmt.Sprintf("Your previous answer failed validation for: %s. Rewrite it using only the authoritative facts. If the facts are missing, say the merchant has not provided that information. Do not output JSON, tool names, IDs, or internal details.", strings.Join(validation.Reasons, ", "))
	retryMessages := buildGenerationMessages(history, grounding, retryInstruction)
	retry, err := s.chatProvider.Complete(ctx, provider.ChatRequest{Messages: retryMessages})
	if err != nil {
		return "", validation, err
	}
	retryAnswer := strings.TrimSpace(retry.Message.Content)
	retryValidation := validateCustomerResponse(retryAnswer, grounding)
	if retryValidation.Valid {
		return retryAnswer, retryValidation, nil
	}
	return deterministicFallback(grounding), retryValidation, nil
}

func buildGenerationMessages(history []provider.Message, grounding GroundingContext, retryInstruction string) []provider.Message {
	relevant := trimHistory(history, 8)
	system := prompts.GroundedSystemPrompt()
	context := grounding.PromptContext()
	if retryInstruction != "" {
		context += "\nVALIDATION REPAIR INSTRUCTION:\n" + retryInstruction + "\n"
	}
	messages := []provider.Message{{Role: "system", Content: system}, {Role: "system", Content: context}}
	messages = append(messages, relevant...)
	messages = append(messages, provider.Message{Role: "user", Content: "Answer the latest customer message using only the authoritative facts and constraints above."})
	return messages
}

func trimHistory(history []provider.Message, limit int) []provider.Message {
	if len(history) <= limit {
		return history
	}
	return history[len(history)-limit:]
}

func debugFromGrounding(grounding GroundingContext) []aitools.DebugCall {
	calls := []aitools.DebugCall{}
	for _, source := range grounding.Sources {
		calls = append(calls, aitools.DebugCall{
			Name:   source.Name,
			Status: "ok",
			Result: map[string]any{
				"authority":  source.Authority,
				"confidence": source.Confidence,
			},
		})
	}
	if grounding.Security.Blocked {
		calls = append(calls, aitools.DebugCall{Name: "security_classification", Status: "blocked", Error: grounding.Security.Reason})
	}
	if len(calls) == 0 {
		calls = append(calls, aitools.DebugCall{Name: "grounding", Status: "ok", Result: map[string]any{"intent": grounding.Intent}})
	}
	return calls
}
