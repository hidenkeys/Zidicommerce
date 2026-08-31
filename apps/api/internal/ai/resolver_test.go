package ai

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

func TestResolveEntitiesRejectsGenericSuffixMatchForUnknownProduct(t *testing.T) {
	productID := uuid.New()
	result := resolveEntities(ResolverInput{
		Text: "Do you have Dragonfire Tea?",
		Products: []core.Product{{
			ID:          productID,
			Name:        "Coconut Milk Tea",
			Slug:        "coconut-milk-tea",
			Description: "Coconut and milk tea.",
			Status:      core.StatusActive,
			Variants: []core.Variant{{
				ID:        uuid.New(),
				ProductID: productID,
				Name:      "Regular",
				SKU:       "COCONUT-MILK-TEA-REG",
				Status:    core.StatusActive,
			}},
		}},
	})

	if result.Resolved.ProductStatus != ResolutionNoMatch {
		t.Fatalf("expected unknown product to resolve as no match, got %q", result.Resolved.ProductStatus)
	}
	if len(result.Products) != 0 {
		t.Fatalf("expected no grounded products for unknown product, got %+v", result.Products)
	}
}

func TestResolveEntitiesKeepsDiscriminativeKnownProductMatch(t *testing.T) {
	productID := uuid.New()
	result := resolveEntities(ResolverInput{
		Text: "How much is Original Milk Tea?",
		Products: []core.Product{{
			ID:          productID,
			Name:        "Original Milk Tea",
			Slug:        "original-milk-tea",
			Description: "Classic milk tea.",
			Status:      core.StatusActive,
			Variants: []core.Variant{{
				ID:         uuid.New(),
				ProductID:  productID,
				Name:       "Regular",
				SKU:        "ORIGINAL-MILK-TEA-REG",
				PriceMinor: 360000,
				Currency:   "NGN",
				Status:     core.StatusActive,
			}},
		}},
	})

	if result.Resolved.ProductID != productID || result.Resolved.ProductStatus != ResolutionExact {
		t.Fatalf("expected exact known-product match, got status=%q product=%s", result.Resolved.ProductStatus, result.Resolved.ProductID)
	}
}

func TestPlanRetrievalRoutesHumanRequestToMerchantKnowledge(t *testing.T) {
	plan := planRetrieval("Can I speak to a human?", AIConversationState{}, SecurityDecision{})
	if !plan.FAQ || plan.Intent != IntentFAQ {
		t.Fatalf("expected human support request to use FAQ grounding, got %+v", plan)
	}
}
