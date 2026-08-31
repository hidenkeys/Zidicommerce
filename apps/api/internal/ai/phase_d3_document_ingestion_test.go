package ai

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

func TestPhaseD3UnapprovedDocumentChunksAreNotGrounded(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "Gift wrapping is free on every order."})
	sourceID := uuid.New()
	chunkID := uuid.New()
	now := time.Now().UTC()
	mustCreate(t, f.db,
		&bot.DocumentSource{ID: sourceID, OrganizationID: f.orgA.orgID, Title: "Draft gift policy", SourceType: bot.DocumentSourcePastedText, Status: bot.DocumentStatusReviewRequired, MimeType: "text/plain", RawText: "Gift wrapping is available for NGN 1500.", Metadata: "{}", CreatedAt: now, UpdatedAt: now},
		&bot.DocumentChunk{ID: chunkID, OrganizationID: f.orgA.orgID, DocumentSourceID: sourceID, ChunkIndex: 0, Title: "Gift policy", Content: "Gift wrapping is available for NGN 1500.", Status: bot.DocumentChunkStatusReviewRequired, Metadata: "{}", CreatedAt: now, UpdatedAt: now},
	)

	sessionID := f.startSession(t, f.orgA)
	response := f.send(t, f.orgA, sessionID, "Do you offer gift wrapping?")

	assertContainsAll(t, response.Message.Body, []string{"do not have that information"})
	assertContainsNone(t, response.Message.Body, []string{"Gift wrapping is available", "NGN 1500", "free"})
}

func TestPhaseD3ApprovedDocumentKnowledgeIsGrounded(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	sourceID := uuid.New()
	chunkID := uuid.New()
	entryID := uuid.New()
	now := time.Now().UTC()
	mustCreate(t, f.db,
		&bot.DocumentSource{ID: sourceID, OrganizationID: f.orgA.orgID, Title: "Warranty document", SourceType: bot.DocumentSourceManual, Status: bot.DocumentStatusReviewRequired, MimeType: "text/plain", RawText: "Sneaker warranty lasts 60 days for sole separation.", Metadata: "{}", CreatedAt: now, UpdatedAt: now},
		&bot.KnowledgeEntry{ID: entryID, OrganizationID: f.orgA.orgID, Kind: bot.KnowledgeKindWarranty, Category: "warranty", Title: "Sneaker warranty", Question: "What is your sneaker warranty?", Answer: "Sneaker warranty lasts 60 days for sole separation.", Keywords: `["warranty","sole separation"]`, SourceType: "document_chunk", Status: core.StatusActive, Metadata: "{}", CreatedAt: now, UpdatedAt: now},
		&bot.DocumentChunk{ID: chunkID, OrganizationID: f.orgA.orgID, DocumentSourceID: sourceID, KnowledgeEntryID: &entryID, ChunkIndex: 0, Title: "Sneaker warranty", Content: "Sneaker warranty lasts 60 days for sole separation.", Status: bot.DocumentChunkStatusApproved, Metadata: "{}", CreatedAt: now, UpdatedAt: now},
	)

	sessionID := f.startSession(t, f.orgA)
	response := f.send(t, f.orgA, sessionID, "What is your sneaker warranty?")

	assertContainsAll(t, response.Message.Body, []string{"60 days", "sole separation"})
}

func TestPhaseD3ArchivedDocumentChunkDoesNotOverrideActiveKnowledge(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	sourceID := uuid.New()
	now := time.Now().UTC()
	seedMerchantKnowledge(t, f, f.orgA, bot.KnowledgeKindDelivery, "delivery", "Delivery policy", "How long does delivery take?", "Delivery takes 24 hours within Lagos.", []string{"delivery"})
	mustCreate(t, f.db,
		&bot.DocumentSource{ID: sourceID, OrganizationID: f.orgA.orgID, Title: "Old delivery document", SourceType: bot.DocumentSourceManual, Status: bot.DocumentStatusArchived, MimeType: "text/plain", RawText: "Delivery takes 10 days.", Metadata: "{}", CreatedAt: now, UpdatedAt: now},
		&bot.DocumentChunk{ID: uuid.New(), OrganizationID: f.orgA.orgID, DocumentSourceID: sourceID, ChunkIndex: 0, Title: "Old delivery", Content: "Delivery takes 10 days.", Status: bot.DocumentChunkStatusArchived, Metadata: "{}", CreatedAt: now, UpdatedAt: now},
	)

	sessionID := f.startSession(t, f.orgA)
	response := f.send(t, f.orgA, sessionID, "How long does delivery take?")

	if strings.Contains(response.Message.Body, "10 days") {
		t.Fatalf("archived document chunk leaked into response: %q", response.Message.Body)
	}
	assertContainsAll(t, response.Message.Body, []string{"24 hours", "Lagos"})
}

func TestPhaseD35ArchivedLinkedKnowledgeIsIgnoredByAI(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "Gift wrapping is available for NGN 1500."})
	sourceID := uuid.New()
	chunkID := uuid.New()
	entryID := uuid.New()
	now := time.Now().UTC()
	mustCreate(t, f.db,
		&bot.DocumentSource{ID: sourceID, OrganizationID: f.orgA.orgID, Title: "Gift policy document", SourceType: bot.DocumentSourceManual, Status: bot.DocumentStatusArchived, MimeType: "text/plain", RawText: "Gift wrapping is available for NGN 1500.", Metadata: "{}", CreatedAt: now, UpdatedAt: now},
		&bot.KnowledgeEntry{ID: entryID, OrganizationID: f.orgA.orgID, Kind: bot.KnowledgeKindPolicy, Category: "policy", Title: "Gift wrapping", Question: "Do you offer gift wrapping?", Answer: "Gift wrapping is available for NGN 1500.", Keywords: `["gift wrapping"]`, SourceType: "document_chunk", Status: "archived", Metadata: "{}", CreatedAt: now, UpdatedAt: now},
		&bot.DocumentChunk{ID: chunkID, OrganizationID: f.orgA.orgID, DocumentSourceID: sourceID, KnowledgeEntryID: &entryID, ChunkIndex: 0, Title: "Gift wrapping", Content: "Gift wrapping is available for NGN 1500.", Status: bot.DocumentChunkStatusArchived, Metadata: "{}", CreatedAt: now, UpdatedAt: now},
	)

	sessionID := f.startSession(t, f.orgA)
	response := f.send(t, f.orgA, sessionID, "Do you offer gift wrapping?")

	assertContainsAll(t, response.Message.Body, []string{"do not have that information"})
	assertContainsNone(t, response.Message.Body, []string{"NGN 1500", "available"})
}
