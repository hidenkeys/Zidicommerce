package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

type mockEmbeddingProvider struct {
	name   string
	model  string
	vector []float32
	err    error
	calls  int
}

func (p *mockEmbeddingProvider) Name() string {
	if p.name == "" {
		return "mock"
	}
	return p.name
}

func (p *mockEmbeddingProvider) Model() string {
	if p.model == "" {
		return "mock-embedding"
	}
	return p.model
}

func (p *mockEmbeddingProvider) Embed(context.Context, string) ([]float32, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return p.vector, nil
}

func TestPhaseD4RefreshKnowledgeEmbeddingLifecycle(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	provider := &mockEmbeddingProvider{vector: []float32{1, 0, 0}, model: "mock-v1"}
	f.service.ConfigureEmbeddings(provider, EmbeddingOptions{Enabled: true, Model: "mock-v1", Dimensions: 3, VectorSearchThreshold: 0.7})

	activeID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Returns", "Can I return an order?", "Returns are accepted within 7 days.", []string{"returns"}, core.StatusActive)
	if err := f.service.RefreshKnowledgeEmbedding(context.Background(), activeID); err != nil {
		t.Fatal(err)
	}
	active := loadKnowledgeEntryForD4(t, f, f.orgA.orgID, activeID)
	if active.EmbeddingStatus != bot.KnowledgeEmbeddingStatusReady || active.Embedding == nil || *active.Embedding != "[1,0,0]" {
		t.Fatalf("expected active knowledge to have a ready embedding, got %+v", active)
	}
	if active.EmbeddingModel != "mock-v1" || active.EmbeddingContentHash != bot.KnowledgeEmbeddingHash(active) || active.EmbeddedAt == nil {
		t.Fatalf("expected embedding metadata to be stored, got %+v", active)
	}

	if err := f.db.Model(&bot.KnowledgeEntry{}).Where("organization_id = ? AND id = ?", f.orgA.orgID, activeID).Update("answer", "Returns are accepted within 14 days.").Error; err != nil {
		t.Fatal(err)
	}
	processed, err := f.service.RefreshStaleKnowledgeEmbeddings(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected one stale embedding refresh, got %d", processed)
	}
	refreshed := loadKnowledgeEntryForD4(t, f, f.orgA.orgID, activeID)
	if refreshed.EmbeddingContentHash != bot.KnowledgeEmbeddingHash(refreshed) || provider.calls < 2 {
		t.Fatalf("expected stale content to be re-embedded, entry=%+v calls=%d", refreshed, provider.calls)
	}

	draftID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindPolicy, "policy", "Draft policy", "Draft policy?", "Drafts are not AI-facing.", []string{"draft"}, "draft")
	if err := f.service.RefreshKnowledgeEmbedding(context.Background(), draftID); err != nil {
		t.Fatal(err)
	}
	draft := loadKnowledgeEntryForD4(t, f, f.orgA.orgID, draftID)
	if draft.EmbeddingStatus != bot.KnowledgeEmbeddingStatusDisabled || draft.Embedding != nil {
		t.Fatalf("expected draft knowledge embedding to be disabled, got %+v", draft)
	}

	failingProvider := &mockEmbeddingProvider{err: errors.New("provider offline")}
	f.service.ConfigureEmbeddings(failingProvider, EmbeddingOptions{Enabled: true, Dimensions: 3})
	failedID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty", "What warranty do you offer?", "Warranty lasts 30 days.", []string{"warranty"}, core.StatusActive)
	if err := f.service.RefreshKnowledgeEmbedding(context.Background(), failedID); err != nil {
		t.Fatal(err)
	}
	failed := loadKnowledgeEntryForD4(t, f, f.orgA.orgID, failedID)
	if failed.EmbeddingStatus != bot.KnowledgeEmbeddingStatusFailed || !strings.Contains(failed.EmbeddingError, "provider offline") {
		t.Fatalf("expected provider errors to be isolated on the entry, got %+v", failed)
	}
}

func TestPhaseD4HybridRetrievalUsesActiveTenantScopedVectorCandidates(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	provider := &mockEmbeddingProvider{vector: []float32{1, 0, 0}}
	retriever := hybridKnowledgeRetriever{
		db:       f.db,
		lexical:  lexicalFAQRetriever{db: f.db},
		provider: provider,
		options:  normalizeEmbeddingOptions(EmbeddingOptions{Enabled: true, VectorSearchEnabled: true, Dimensions: 3, VectorSearchThreshold: 0.7}),
	}
	returnsID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Returns policy", "What is your returns policy?", "Returns are accepted within 7 days with receipt proof.", []string{"returns"}, core.StatusActive)
	paymentID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindPaymentInfo, "payment", "Payment methods", "Which payment methods are supported?", "Cards and bank transfers are accepted.", []string{"payment"}, core.StatusActive)
	otherTenantID := seedMerchantKnowledgeWithStatus(t, f, f.orgB, bot.KnowledgeKindReturns, "returns", "Other tenant returns", "What is your returns policy?", "Other tenant return policy must never appear.", []string{"returns"}, core.StatusActive)
	setKnowledgeEmbeddingForD4(t, f, f.orgA.orgID, returnsID, []float32{1, 0, 0})
	setKnowledgeEmbeddingForD4(t, f, f.orgA.orgID, paymentID, []float32{0, 1, 0})
	setKnowledgeEmbeddingForD4(t, f, f.orgB.orgID, otherTenantID, []float32{1, 0, 0})

	result, err := retriever.Retrieve(context.Background(), KnowledgeRequest{OrganizationID: f.orgA.orgID, Query: "Can I reverse a purchase?", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) == 0 || result.Entries[0].ID != returnsID {
		t.Fatalf("expected vector retrieval to select the semantically matched returns entry, got %+v", result)
	}
	if len(result.Entries) > 1 && result.Entries[1].ID == paymentID {
		t.Fatalf("expected weak unrelated vector candidate to stay below threshold, got %+v", result.Entries)
	}
	if debugContainsTitleForD4(result.Debug, "Other tenant returns") {
		t.Fatalf("cross-tenant vector candidate leaked into debug/result: %+v", result.Debug)
	}
	if !debugContainsField(result.Debug, "Returns policy", "vector:") {
		t.Fatalf("expected vector debug metadata, got %+v", result.Debug)
	}
}

func TestPhaseD4HybridRetrievalFallsBackWhenVectorProviderFails(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindDelivery, "delivery", "Delivery policy", "How long does delivery take?", "Delivery takes 24 to 48 hours.", []string{"delivery"}, core.StatusActive)
	retriever := hybridKnowledgeRetriever{
		db:       f.db,
		lexical:  lexicalFAQRetriever{db: f.db},
		provider: &mockEmbeddingProvider{err: errors.New("rate limited")},
		options:  normalizeEmbeddingOptions(EmbeddingOptions{Enabled: true, VectorSearchEnabled: true, Dimensions: 3, VectorSearchThreshold: 0.7}),
	}
	result, err := retriever.Retrieve(context.Background(), KnowledgeRequest{OrganizationID: f.orgA.orgID, Query: "How long does delivery take?", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) == 0 || result.Entries[0].Title != "Delivery policy" {
		t.Fatalf("expected lexical fallback to preserve normal retrieval, got %+v", result)
	}
	if !debugHasSkippedReason(result.Debug, "vector_query_failed") {
		t.Fatalf("expected vector failure debug metadata, got %+v", result.Debug)
	}
}

func TestPhaseD4HybridRetrievalIgnoresDraftArchivedAndHandlesConflicts(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	provider := &mockEmbeddingProvider{vector: []float32{1, 0, 0}}
	retriever := hybridKnowledgeRetriever{
		db:       f.db,
		lexical:  lexicalFAQRetriever{db: f.db},
		provider: provider,
		options:  normalizeEmbeddingOptions(EmbeddingOptions{Enabled: true, VectorSearchEnabled: true, Dimensions: 3, VectorSearchThreshold: 0.7}),
	}
	draftID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Draft lifetime warranty", "Do you offer lifetime warranty?", "Draft lifetime warranty should not be used.", []string{"lifetime warranty"}, "draft")
	archivedID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Archived lifetime warranty", "Do you offer lifetime warranty?", "Archived lifetime warranty should not be used.", []string{"lifetime warranty"}, "archived")
	setKnowledgeEmbeddingForD4(t, f, f.orgA.orgID, draftID, []float32{1, 0, 0})
	setKnowledgeEmbeddingForD4(t, f, f.orgA.orgID, archivedID, []float32{1, 0, 0})

	ignored, err := retriever.Retrieve(context.Background(), KnowledgeRequest{OrganizationID: f.orgA.orgID, Query: "Do you offer lifetime warranty?", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(ignored.Entries) != 0 || len(ignored.Matches) != 0 {
		t.Fatalf("expected draft and archived vector candidates to be ignored, got %+v", ignored)
	}

	firstID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Returns 7 days", "What is your returns policy?", "Returns are accepted within 7 days.", []string{"returns"}, core.StatusActive)
	secondID := seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Returns 30 days", "What is your returns policy?", "Returns are accepted within 30 days.", []string{"returns"}, core.StatusActive)
	setKnowledgeEmbeddingForD4(t, f, f.orgA.orgID, firstID, []float32{1, 0, 0})
	setKnowledgeEmbeddingForD4(t, f, f.orgA.orgID, secondID, []float32{1, 0, 0})

	conflicted, err := retriever.Retrieve(context.Background(), KnowledgeRequest{OrganizationID: f.orgA.orgID, Query: "Can I reverse a purchase?", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicted.Conflicts) != 1 || len(conflicted.Entries) != 0 {
		t.Fatalf("expected conflicting vector candidates to be withheld from grounding, got %+v", conflicted)
	}
}

func loadKnowledgeEntryForD4(t *testing.T, f aiRegressionFixture, orgID uuid.UUID, entryID uuid.UUID) bot.KnowledgeEntry {
	t.Helper()
	var entry bot.KnowledgeEntry
	if err := f.db.Where("organization_id = ? AND id = ?", orgID, entryID).First(&entry).Error; err != nil {
		t.Fatal(err)
	}
	return entry
}

func setKnowledgeEmbeddingForD4(t *testing.T, f aiRegressionFixture, orgID uuid.UUID, entryID uuid.UUID, vector []float32) {
	t.Helper()
	serialized := serializeEmbeddingVector(vector)
	if err := f.db.Model(&bot.KnowledgeEntry{}).
		Where("organization_id = ? AND id = ?", orgID, entryID).
		Updates(map[string]any{
			"embedding":        serialized,
			"embedding_status": bot.KnowledgeEmbeddingStatusReady,
			"embedding_model":  "mock",
		}).Error; err != nil {
		t.Fatal(err)
	}
}

func debugContainsTitleForD4(debug []KnowledgeMatchDebug, title string) bool {
	for _, item := range debug {
		if item.Title == title {
			return true
		}
	}
	return false
}
