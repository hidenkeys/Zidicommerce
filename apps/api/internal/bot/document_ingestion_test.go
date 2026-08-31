package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

func TestDocumentSourceCreatesDeterministicReviewChunks(t *testing.T) {
	fx := newBotFixture(t)
	input := DocumentSourceInput{
		Title:      "Store policies",
		SourceType: DocumentSourcePastedText,
		RawText: `# Delivery
Lagos delivery takes 24 to 48 hours after payment confirmation.

# Returns
Returns are accepted within 7 days with receipt proof.`,
	}

	source, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if source.Status != DocumentStatusReviewRequired {
		t.Fatalf("expected review_required source, got %s", source.Status)
	}
	if len(source.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %+v", source.Chunks)
	}
	if source.Chunks[0].ChunkIndex != 0 || source.Chunks[0].Heading != "Delivery" || source.Chunks[0].Status != DocumentChunkStatusReviewRequired {
		t.Fatalf("unexpected first chunk: %+v", source.Chunks[0])
	}
	if !strings.Contains(source.Chunks[1].Content, "7 days") {
		t.Fatalf("expected return content in second chunk: %+v", source.Chunks[1])
	}
}

func TestDocumentSourceCRUDIsTenantScoped(t *testing.T) {
	fx := newBotFixture(t)
	source, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, DocumentSourceInput{
		Title:      "Warranty notes",
		SourceType: DocumentSourceManual,
		RawText:    "Warranty lasts 30 days for manufacturing defects.",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherActor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	if _, err := fx.service.GetDocumentSource(context.Background(), otherActor, source.ID); err == nil {
		t.Fatal("expected cross-tenant document source lookup to fail")
	}
	if _, err := fx.service.ListDocumentChunks(context.Background(), otherActor, source.ID); err == nil {
		t.Fatal("expected cross-tenant document chunk lookup to fail")
	}
	sources, err := fx.service.ListDocumentSources(context.Background(), otherActor, DocumentSourceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("expected no cross-tenant sources, got %+v", sources)
	}
}

func TestDocumentPermissionsUseKnowledgePermissions(t *testing.T) {
	fx := newBotFixture(t)
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.StoreManager}
	support := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.SupportAgent}

	if _, err := fx.service.ListDocumentSources(context.Background(), viewer, DocumentSourceFilter{}); err != nil {
		t.Fatalf("expected merchant staff to view documents: %v", err)
	}
	if _, err := fx.service.CreateDocumentSource(context.Background(), viewer, DocumentSourceInput{Title: "Policy", RawText: "Returns take 7 days."}); err == nil {
		t.Fatal("expected merchant staff create to be forbidden")
	}
	if _, err := fx.service.ListDocumentSources(context.Background(), support, DocumentSourceFilter{}); err != nil {
		t.Fatalf("expected support agent to view documents: %v", err)
	}
	if _, err := fx.service.CreateDocumentSource(context.Background(), support, DocumentSourceInput{Title: "Policy", RawText: "Returns take 7 days."}); err == nil {
		t.Fatal("expected support agent create to be forbidden")
	}
}

func TestDocumentChunkApprovalCreatesActiveStructuredKnowledge(t *testing.T) {
	fx := newBotFixture(t)
	fx.service.ConfigureKnowledgeEmbeddings(true)
	source, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, DocumentSourceInput{
		Title:       "Payment policy",
		SourceType:  DocumentSourcePastedText,
		SourceLabel: "Payment handbook",
		RawText:     "Customers can pay with bank transfer or card on delivery.",
	})
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := fx.service.ApproveDocumentChunk(context.Background(), fx.actor, source.Chunks[0].ID, DocumentChunkApprovalInput{
		Kind:     KnowledgeKindPaymentInfo,
		Category: "payment_info",
		Question: "What payment methods do you accept?",
		Keywords: []string{"payment", "bank transfer", "card"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Status != DocumentChunkStatusApproved || chunk.KnowledgeEntryID == nil {
		t.Fatalf("expected approved chunk with knowledge entry, got %+v", chunk)
	}
	var entry KnowledgeEntry
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, *chunk.KnowledgeEntryID).First(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if entry.Status != core.StatusActive || entry.SourceType != "document_chunk" || entry.Kind != KnowledgeKindPaymentInfo {
		t.Fatalf("unexpected knowledge entry: %+v", entry)
	}
	if entry.EmbeddingStatus != KnowledgeEmbeddingStatusPending || entry.EmbeddingContentHash == "" {
		t.Fatalf("expected approved knowledge to be pending embedding, got %+v", entry)
	}
	if !strings.Contains(entry.Answer, "bank transfer") {
		t.Fatalf("expected chunk content copied to knowledge answer, got %q", entry.Answer)
	}
	metadata := metadataForTest(t, entry.Metadata)
	if metadata["document_source_id"] != source.ID.String() || metadata["document_chunk_id"] != source.Chunks[0].ID.String() {
		t.Fatalf("expected source/chunk metadata, got %+v", metadata)
	}
	if metadata["document_source_title"] != "Payment policy" || metadata["document_source_label"] != "Payment handbook" {
		t.Fatalf("expected source title/label metadata, got %+v", metadata)
	}
	if metadata["approved_by"] != fx.actor.ID.String() || metadata["approved_at"] == "" {
		t.Fatalf("expected approval metadata, got %+v", metadata)
	}

	chunk, err = fx.service.ApproveDocumentChunk(context.Background(), fx.actor, source.Chunks[0].ID, DocumentChunkApprovalInput{
		Kind:     KnowledgeKindPaymentInfo,
		Category: "payment_info",
		Question: "Which payment methods are supported?",
		Keywords: []string{"payment", "transfer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := fx.db.Model(&KnowledgeEntry{}).Where("organization_id = ? AND source_type = ?", fx.actor.OrganizationID, "document_chunk").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || chunk.KnowledgeEntryID == nil || *chunk.KnowledgeEntryID != entry.ID {
		t.Fatalf("expected idempotent approval to update one linked knowledge entry, count=%d chunk=%+v entry=%s", count, chunk, entry.ID)
	}
}

func TestDocumentArchiveAndUpdateFreshness(t *testing.T) {
	fx := newBotFixture(t)
	source, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, DocumentSourceInput{
		Title:   "Delivery policy",
		RawText: "Delivery takes 48 hours.",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := fx.service.UpdateDocumentSource(context.Background(), fx.actor, source.ID, DocumentSourceInput{
		Title:   "Updated delivery policy",
		RawText: "Delivery now takes 24 hours in Lagos.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Chunks) != 1 || !strings.Contains(updated.Chunks[0].Content, "24 hours") {
		t.Fatalf("expected refreshed chunk, got %+v", updated.Chunks)
	}
	archived, err := fx.service.ArchiveDocumentSource(context.Background(), fx.actor, source.ID, DocumentSourceArchiveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != DocumentStatusArchived || len(archived.Chunks) != 1 || archived.Chunks[0].Status != DocumentChunkStatusArchived {
		t.Fatalf("expected archived source and chunk, got %+v", archived)
	}
}

func TestDocumentSourceArchiveLinkedKnowledgeIsExplicit(t *testing.T) {
	fx := newBotFixture(t)
	source, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, DocumentSourceInput{
		Title:   "Returns policy",
		RawText: "Returns are accepted within 10 days with receipt proof.",
	})
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := fx.service.ApproveDocumentChunk(context.Background(), fx.actor, source.Chunks[0].ID, DocumentChunkApprovalInput{
		Kind:     KnowledgeKindReturns,
		Category: "returns",
		Question: "What is your returns policy?",
		Keywords: []string{"returns"},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := fx.service.GetDocumentSource(context.Background(), fx.actor, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.LinkedKnowledgeCount != 1 || detail.ActiveLinkedKnowledgeCount != 1 {
		t.Fatalf("expected linked knowledge counts, got %+v", detail)
	}
	if _, err := fx.service.ArchiveDocumentSource(context.Background(), fx.actor, source.ID, DocumentSourceArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	var entry KnowledgeEntry
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, *chunk.KnowledgeEntryID).First(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if entry.Status != core.StatusActive {
		t.Fatalf("expected source-only archive to keep linked knowledge active, got %+v", entry)
	}

	source2, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, DocumentSourceInput{
		Title:   "Warranty policy",
		RawText: "Warranty lasts 21 days for manufacturing defects.",
	})
	if err != nil {
		t.Fatal(err)
	}
	chunk2, err := fx.service.ApproveDocumentChunk(context.Background(), fx.actor, source2.Chunks[0].ID, DocumentChunkApprovalInput{
		Kind:     KnowledgeKindWarranty,
		Category: "warranty",
		Question: "What is your warranty?",
		Keywords: []string{"warranty"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ArchiveDocumentSource(context.Background(), fx.actor, source2.ID, DocumentSourceArchiveOptions{ArchiveLinkedKnowledge: true}); err != nil {
		t.Fatal(err)
	}
	var archivedEntry KnowledgeEntry
	if err := fx.db.Where("organization_id = ? AND id = ?", fx.actor.OrganizationID, *chunk2.KnowledgeEntryID).First(&archivedEntry).Error; err != nil {
		t.Fatal(err)
	}
	if archivedEntry.Status != "archived" {
		t.Fatalf("expected explicit linked archive to archive knowledge, got %+v", archivedEntry)
	}
}

func TestDocumentSourceArchiveLinkedKnowledgeIsTenantScoped(t *testing.T) {
	fx := newBotFixture(t)
	source, err := fx.service.CreateDocumentSource(context.Background(), fx.actor, DocumentSourceInput{
		Title:   "Delivery policy",
		RawText: "Delivery takes 24 hours.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ApproveDocumentChunk(context.Background(), fx.actor, source.Chunks[0].ID, DocumentChunkApprovalInput{Kind: KnowledgeKindDelivery, Category: "delivery"}); err != nil {
		t.Fatal(err)
	}

	otherActor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	otherSource, err := fx.service.CreateDocumentSource(context.Background(), otherActor, DocumentSourceInput{
		Title:   "Other delivery policy",
		RawText: "Other tenant delivery takes 3 days.",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherChunk, err := fx.service.ApproveDocumentChunk(context.Background(), otherActor, otherSource.Chunks[0].ID, DocumentChunkApprovalInput{Kind: KnowledgeKindDelivery, Category: "delivery"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fx.service.ArchiveDocumentSource(context.Background(), fx.actor, source.ID, DocumentSourceArchiveOptions{ArchiveLinkedKnowledge: true}); err != nil {
		t.Fatal(err)
	}
	var otherEntry KnowledgeEntry
	if err := fx.db.Where("organization_id = ? AND id = ?", otherActor.OrganizationID, *otherChunk.KnowledgeEntryID).First(&otherEntry).Error; err != nil {
		t.Fatal(err)
	}
	if otherEntry.Status != core.StatusActive {
		t.Fatalf("expected other tenant linked knowledge to remain active, got %+v", otherEntry)
	}
}

func metadataForTest(t *testing.T, raw string) map[string]string {
	t.Helper()
	values := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	out := map[string]string{}
	for key, value := range values {
		out[key] = strings.TrimSpace(strings.Trim(valueToString(value), "\""))
	}
	return out
}

func valueToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
