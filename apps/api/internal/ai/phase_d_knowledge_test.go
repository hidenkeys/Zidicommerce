package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
)

func TestPhaseDMerchantKnowledgeIsTenantScoped(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledge(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "StrideStreet warranty policy", "What is your warranty policy?", "StrideStreet covers stitching defects for 30 days with receipt proof.", []string{"warranty", "stitching defects"})
	seedMerchantKnowledge(t, f, f.orgB, bot.KnowledgeKindWarranty, "warranty", "GlowNest warranty policy", "What is your warranty policy?", "GlowNest replaces damaged serum pumps within 14 days.", []string{"warranty", "damaged pump"})

	sessionID := f.startSession(t, f.orgA)
	response := f.send(t, f.orgA, sessionID, "What is your warranty policy?")

	assertContainsAll(t, response.Message.Body, []string{"30 days", "stitching defects"})
	assertContainsNone(t, response.Message.Body, []string{"GlowNest", "serum pumps", "14 days"})
}

func TestPhaseDUnknownPolicyQuestionsDoNotHallucinate(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "We offer free gift wrapping and nationwide same-day delivery."})
	sessionID := f.startSession(t, f.orgA)

	response := f.send(t, f.orgA, sessionID, "Do you offer gift wrapping?")

	assertContainsAll(t, response.Message.Body, []string{"do not have that information"})
	assertContainsNone(t, response.Message.Body, []string{"gift wrapping", "free", "same-day"})
}

func TestPhaseDMerchantKnowledgeUpdatesAreImmediate(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	entryID := seedMerchantKnowledge(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty policy", "What is your warranty policy?", "Sneakers have a 14-day limited warranty for manufacturing defects.", []string{"warranty", "manufacturing defects"})
	sessionID := f.startSession(t, f.orgA)

	initial := f.send(t, f.orgA, sessionID, "What is your warranty policy?")
	assertContainsAll(t, initial.Message.Body, []string{"14-day", "manufacturing defects"})

	if err := f.db.Model(&bot.KnowledgeEntry{}).
		Where("organization_id = ? AND id = ?", f.orgA.orgID, entryID).
		Updates(map[string]any{"answer": "Sneakers now have a 45-day warranty for verified manufacturing defects.", "updated_at": time.Now().UTC()}).Error; err != nil {
		t.Fatalf("update knowledge: %v", err)
	}

	updated := f.send(t, f.orgA, sessionID, "What is your warranty policy now?")
	assertContainsAll(t, updated.Message.Body, []string{"45-day", "verified manufacturing defects"})
	assertContainsNone(t, updated.Message.Body, []string{"14-day"})
}

func TestPhaseDCommerceFactsOverrideMerchantKnowledgeText(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "Urban Runner Sneakers cost NGN 999999."})
	seedMerchantKnowledge(t, f, f.orgA, bot.KnowledgeKindFAQ, "general", "Old sneaker price note", "How much are Urban Runner Sneakers?", "Urban Runner Sneakers used to cost NGN 999999 in an old note.", []string{"urban runner", "price"})
	sessionID := f.startSession(t, f.orgA)

	price := f.send(t, f.orgA, sessionID, "How much is the black size 42 Urban Runner Sneakers?")
	assertContainsAll(t, price.Message.Body, []string{"Urban Runner Sneakers", "Black Size 42", "NGN 28500"})
	assertContainsNone(t, price.Message.Body, []string{"NGN 999999"})

	inventory := f.send(t, f.orgA, sessionID, "Do you have 2 black Urban Runner Sneakers at StrideStreet Lekki?")
	assertContainsAll(t, inventory.Message.Body, []string{"Urban Runner Sneakers", "Black Size 42", "StrideStreet Lekki", "7 available"})
	assertContainsNone(t, inventory.Message.Body, []string{"NGN 999999", "GlowNest"})
}

func TestPhaseDGroundingSeparatesKnowledgeFromCommerceFacts(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	entryID := seedMerchantKnowledge(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Return policy", "What is your return policy?", "Returns need the receipt and unused shoes within 10 days.", []string{"returns", "receipt"})
	sessionID := f.startSession(t, f.orgA)
	var session runtime.ConversationSession
	if err := f.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}

	state := AIConversationState{}
	grounding, _, err := newRetrievalEngine(f.db, core.NewService(f.db, core.SafeTestProvider{})).BuildGrounding(context.Background(), session, "What is your return policy?", state, SecurityDecision{})
	if err != nil {
		t.Fatalf("build grounding: %v", err)
	}
	if len(grounding.Knowledge) != 1 || grounding.Knowledge[0].ID != entryID {
		t.Fatalf("expected structured knowledge entry in grounding, got %#v", grounding.Knowledge)
	}
	if len(grounding.Products) != 0 || len(grounding.Inventory) != 0 {
		t.Fatalf("expected policy question not to retrieve commerce facts, got products=%d inventory=%d", len(grounding.Products), len(grounding.Inventory))
	}
}

func seedMerchantKnowledge(t *testing.T, f aiRegressionFixture, tenant regressionTenant, kind, category, title, question, answer string, keywords []string) uuid.UUID {
	t.Helper()
	body, err := json.Marshal(cleanPhaseDKeywords(keywords))
	if err != nil {
		t.Fatalf("marshal keywords: %v", err)
	}
	entry := bot.KnowledgeEntry{
		ID:             uuid.New(),
		OrganizationID: tenant.orgID,
		Kind:           kind,
		Category:       category,
		Title:          title,
		Question:       question,
		Answer:         answer,
		Keywords:       string(body),
		SourceType:     "manual",
		Status:         core.StatusActive,
		Metadata:       "{}",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := f.db.Create(&entry).Error; err != nil {
		t.Fatalf("seed merchant knowledge: %v", err)
	}
	return entry.ID
}

func cleanPhaseDKeywords(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
