package ai

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
)

const aiStateKey = "ai_state"

type AIConversationState struct {
	CurrentIntent    AIIntent          `json:"current_intent,omitempty"`
	CurrentCategory  string            `json:"current_category,omitempty"`
	CurrentProductID string            `json:"current_product_id,omitempty"`
	CurrentProduct   string            `json:"current_product_name,omitempty"`
	CurrentVariantID string            `json:"current_variant_id,omitempty"`
	CurrentVariant   string            `json:"current_variant_name,omitempty"`
	SelectedStoreID  string            `json:"selected_store_id,omitempty"`
	SelectedStore    string            `json:"selected_store_name,omitempty"`
	Quantity         int               `json:"quantity,omitempty"`
	Filters          map[string]string `json:"current_filters,omitempty"`
	LastCandidates   []StateCandidate  `json:"last_candidates,omitempty"`
	UnresolvedRef    string            `json:"unresolved_reference,omitempty"`
}

type StateCandidate struct {
	ProductID   string `json:"product_id,omitempty"`
	ProductName string `json:"product_name,omitempty"`
	VariantID   string `json:"variant_id,omitempty"`
	VariantName string `json:"variant_name,omitempty"`
	PriceMinor  int64  `json:"price_minor,omitempty"`
	Currency    string `json:"currency,omitempty"`
}

func loadAIState(session runtime.ConversationSession) AIConversationState {
	root := map[string]any{}
	_ = json.Unmarshal([]byte(defaultJSONObject(session.Variables)), &root)
	raw, ok := root[aiStateKey]
	if !ok {
		return AIConversationState{Filters: map[string]string{}}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return AIConversationState{Filters: map[string]string{}}
	}
	var state AIConversationState
	if err := json.Unmarshal(payload, &state); err != nil {
		return AIConversationState{Filters: map[string]string{}}
	}
	if state.Filters == nil {
		state.Filters = map[string]string{}
	}
	return state
}

func storeAIState(session runtime.ConversationSession, state AIConversationState) string {
	root := map[string]any{}
	_ = json.Unmarshal([]byte(defaultJSONObject(session.Variables)), &root)
	root[aiStateKey] = state
	raw, err := json.Marshal(root)
	if err != nil {
		return defaultJSONObject(session.Variables)
	}
	return string(raw)
}

func updateAIStateFromGrounding(state AIConversationState, grounding GroundingContext) AIConversationState {
	state.CurrentIntent = grounding.Intent
	if state.Filters == nil {
		state.Filters = map[string]string{}
	}
	for key, value := range grounding.Resolved.Filters {
		if strings.TrimSpace(value) != "" {
			state.Filters[key] = value
		}
	}
	if grounding.Resolved.CategoryName != "" {
		state.CurrentCategory = grounding.Resolved.CategoryName
	}
	if grounding.Resolved.ProductID != uuid.Nil {
		state.CurrentProductID = grounding.Resolved.ProductID.String()
		state.CurrentProduct = grounding.Resolved.ProductName
	}
	if grounding.Resolved.VariantID != uuid.Nil {
		state.CurrentVariantID = grounding.Resolved.VariantID.String()
		state.CurrentVariant = grounding.Resolved.VariantName
	}
	if grounding.Resolved.StoreID != uuid.Nil {
		state.SelectedStoreID = grounding.Resolved.StoreID.String()
		state.SelectedStore = grounding.Resolved.StoreName
	}
	if grounding.Requested.Quantity > 0 {
		state.Quantity = grounding.Requested.Quantity
	}
	state.LastCandidates = candidatesFromGrounding(grounding)
	if grounding.Resolved.ProductStatus == ResolutionAmbiguous || grounding.Resolved.StoreStatus == ResolutionAmbiguous {
		state.UnresolvedRef = grounding.Resolved.AmbiguousReason
	} else {
		state.UnresolvedRef = ""
	}
	return state
}

func candidatesFromGrounding(grounding GroundingContext) []StateCandidate {
	candidates := []StateCandidate{}
	for _, product := range grounding.Products {
		for _, variant := range product.Variants {
			candidates = append(candidates, StateCandidate{
				ProductID:   product.ID.String(),
				ProductName: product.Name,
				VariantID:   variant.ID.String(),
				VariantName: variant.Name,
				PriceMinor:  variant.PriceMinor,
				Currency:    variant.Currency,
			})
			if len(candidates) >= 8 {
				return candidates
			}
		}
	}
	return candidates
}

func defaultJSONObject(value string) string {
	if strings.TrimSpace(value) == "" {
		return "{}"
	}
	return value
}
