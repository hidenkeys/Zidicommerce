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

func TestResolveEntitiesNarrowsExactProductWithinSimilarCatalogue(t *testing.T) {
	originalID := uuid.New()
	coconutID := uuid.New()
	sundaeID := uuid.New()
	categoryID := uuid.New()
	result := resolveEntities(ResolverInput{
		Text: "How much is Original Milk Tea?",
		Products: []core.Product{
			{ID: originalID, CategoryID: &categoryID, Name: "Original Milk Tea", Slug: "original-milk-tea", Status: core.StatusActive, Variants: []core.Variant{{ID: uuid.New(), ProductID: originalID, Name: "Regular", SKU: "ORIGINAL", PriceMinor: 360000, Currency: "NGN", Status: core.StatusActive}}},
			{ID: coconutID, CategoryID: &categoryID, Name: "Coconut Milk Tea", Slug: "coconut-milk-tea", Status: core.StatusActive, Variants: []core.Variant{{ID: uuid.New(), ProductID: coconutID, Name: "Regular", SKU: "COCONUT", PriceMinor: 420000, Currency: "NGN", Status: core.StatusActive}}},
			{ID: sundaeID, Name: "Brown Sugar Boba Sundae", Slug: "brown-sugar-boba-sundae", Status: core.StatusActive, Variants: []core.Variant{{ID: uuid.New(), ProductID: sundaeID, Name: "Regular", SKU: "SUNDAE", PriceMinor: 420000, Currency: "NGN", Status: core.StatusActive}}},
		},
		Categories: []core.Category{{ID: categoryID, Name: "Milk tea", Slug: "milk-tea", Status: core.StatusActive}},
	})

	if result.Resolved.ProductID != originalID || len(result.Products) != 1 || result.Products[0].ID != originalID {
		t.Fatalf("expected only the fully referenced product, got resolved=%s products=%+v", result.Resolved.ProductID, result.Products)
	}
}

func TestPlanRetrievalTreatsCanIOrderAsInventoryRequest(t *testing.T) {
	plan := planRetrieval("Can I order Brown Sugar Boba Milk Tea right now?", AIConversationState{}, SecurityDecision{})
	if !plan.Inventory || !plan.Products || !plan.Stores || plan.Intent != IntentInventory {
		t.Fatalf("expected an orderability question to check inventory, got %+v", plan)
	}
}

func TestResolveEntitiesDistinguishesMilkTeaFromSamePrefixSundae(t *testing.T) {
	milkTeaID := uuid.New()
	sundaeID := uuid.New()
	bobaID := uuid.New()
	milkTeaCategoryID := uuid.New()
	query := "Can I order Brown Sugar Boba Milk Tea right now?"
	if !productNameFullyReferenced(query, "Brown Sugar Boba Milk Tea") || productNameFullyReferenced(query, "Brown Sugar Boba Sundae") {
		t.Fatal("strict product-name matching did not distinguish milk tea from sundae")
	}
	result := resolveEntities(ResolverInput{
		Text: query,
		Products: []core.Product{
			{ID: bobaID, Name: "Boba", Slug: "boba", Status: core.StatusActive, Variants: []core.Variant{{ID: uuid.New(), ProductID: bobaID, Name: "Regular", SKU: "BOBA", PriceMinor: 50000, Currency: "NGN", Status: core.StatusActive}}},
			{ID: sundaeID, Name: "Brown Sugar Boba Sundae", Slug: "brown-sugar-boba-sundae", Status: core.StatusActive, Variants: []core.Variant{{ID: uuid.New(), ProductID: sundaeID, Name: "Regular", SKU: "SUNDAE", PriceMinor: 420000, Currency: "NGN", Status: core.StatusActive}}},
			{ID: milkTeaID, CategoryID: &milkTeaCategoryID, Name: "Brown Sugar Boba Milk Tea", Slug: "brown-sugar-boba-milk-tea", Status: core.StatusActive, Variants: []core.Variant{{ID: uuid.New(), ProductID: milkTeaID, Name: "Regular", SKU: "MILK-TEA", PriceMinor: 420000, Currency: "NGN", Status: core.StatusActive}}},
		},
		Categories: []core.Category{{ID: milkTeaCategoryID, Name: "Milk tea", Slug: "milk-tea", Status: core.StatusActive}},
	})

	if result.Resolved.ProductID != milkTeaID || len(result.Products) != 1 || result.Products[0].ID != milkTeaID {
		t.Fatalf("expected milk tea rather than same-prefix sundae, got resolved=%s products=%+v", result.Resolved.ProductID, result.Products)
	}
}

func TestGroundedInventoryExcludesInactiveStores(t *testing.T) {
	productID := uuid.New()
	variantID := uuid.New()
	activeStoreID := uuid.New()
	inactiveStoreID := uuid.New()
	variant := core.Variant{ID: variantID, ProductID: productID, Name: "Regular", Product: core.Product{ID: productID, Name: "Original Milk Tea"}}
	rows := groundedInventory(
		[]core.InventoryLevel{
			{StoreID: activeStoreID, VariantID: variantID, OnHand: 4, Variant: variant},
			{StoreID: inactiveStoreID, VariantID: variantID, OnHand: 9, Variant: variant},
		},
		GroundingContext{Resolved: ResolvedEntities{ProductID: productID, ProductStatus: ResolutionExact}, Requested: RequestedFacts{Quantity: 1}},
		[]core.Store{
			{ID: activeStoreID, Name: "Open Store", Status: core.StatusActive},
			{ID: inactiveStoreID, Name: "Closed Store", Status: core.StatusInactive},
		},
	)

	if len(rows) != 1 || rows[0].StoreID != activeStoreID {
		t.Fatalf("expected only active-store inventory, got %+v", rows)
	}
}

func TestPlanRetrievalRoutesHumanRequestToMerchantKnowledge(t *testing.T) {
	plan := planRetrieval("Can I speak to a human?", AIConversationState{}, SecurityDecision{})
	if !plan.FAQ || plan.Intent != IntentFAQ {
		t.Fatalf("expected human support request to use FAQ grounding, got %+v", plan)
	}
}
