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
)

func TestPhaseD2ExactMatchBeatsWeakContentMatch(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Exact returns", "Can I return an item?", "Returns are accepted within 7 days with receipt proof.", []string{"returns"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindReturns, "returns", "Generic returns note", "Policy details", "Customers sometimes ask whether they can return an item, but staff must check the receipt.", []string{"policy"}, core.StatusActive)

	result := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "Can I return an item?")
	if len(result.Entries) == 0 || result.Entries[0].Title != "Exact returns" {
		t.Fatalf("expected exact question match first, got %#v", result.Entries)
	}
	if !debugContainsField(result.Debug, "Exact returns", "question_exact") {
		t.Fatalf("expected question_exact debug, got %#v", result.Debug)
	}
}

func TestPhaseD2KeywordMatchBeatsGenericAnswerText(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindPaymentInfo, "payment", "Afterpay", "Do you accept Afterpay?", "Afterpay is accepted for orders above NGN 50000.", []string{"afterpay"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindPaymentInfo, "payment", "Generic payment", "Which payment methods are available?", "Payment questions can include card, cash, transfer, and installment options.", []string{"payment methods"}, core.StatusActive)

	result := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "Can I use afterpay?")
	if len(result.Entries) == 0 || result.Entries[0].Title != "Afterpay" {
		t.Fatalf("expected keyword match first, got %#v", result.Entries)
	}
	if !debugContainsField(result.Debug, "Afterpay", "keyword") {
		t.Fatalf("expected keyword debug, got %#v", result.Debug)
	}
}

func TestPhaseD2CategoryRoutingPrefersDeliveryWarrantyAndPaymentInfo(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindPolicy, "general", "General policy", "General store policy", "General support information mentions delivery, warranty, and payment.", []string{"policy"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindDelivery, "delivery", "Delivery policy", "How long does delivery take?", "Delivery takes 24 to 48 hours after payment confirmation.", []string{"delivery"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty policy", "What is your warranty policy?", "Warranty covers manufacturing defects for 30 days.", []string{"warranty"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindPaymentInfo, "payment", "Payment methods", "Do you accept transfer?", "Bank transfer and cards are accepted.", []string{"transfer", "card"}, core.StatusActive)

	delivery := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "When will delivery arrive?")
	if len(delivery.Entries) == 0 || delivery.Entries[0].Kind != bot.KnowledgeKindDelivery {
		t.Fatalf("expected delivery route, got %#v", delivery.Entries)
	}
	warranty := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "Do you have warranty?")
	if len(warranty.Entries) == 0 || warranty.Entries[0].Kind != bot.KnowledgeKindWarranty {
		t.Fatalf("expected warranty route, got %#v", warranty.Entries)
	}
	payment := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "Can I pay with transfer?")
	if len(payment.Entries) == 0 || payment.Entries[0].Kind != bot.KnowledgeKindPaymentInfo {
		t.Fatalf("expected payment_info route, got %#v", payment.Entries)
	}
}

func TestPhaseD2CompoundQuestionsPreserveUnknownPolicyParts(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindDelivery, "delivery", "Delivery policy", "Do you deliver?", "Delivery depends on the selected store.", []string{"delivery"}, core.StatusActive)

	result := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "What payment methods do you accept? Can I pay on delivery?")
	if len(result.Matches) == 0 || result.Matches[0].Category != "delivery" {
		t.Fatalf("expected the grounded delivery part, got %#v", result.Matches)
	}
	if len(result.Unknown) == 0 || result.Unknown[0].Kind != "payment" {
		t.Fatalf("expected the unanswered payment part to remain unknown, got %#v", result.Unknown)
	}
}

func TestPhaseD2DraftArchivedAndCrossTenantEntriesAreIgnored(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Draft warranty", "Do you offer lifetime warranty?", "Draft lifetime warranty should not be used.", []string{"lifetime warranty"}, "draft")
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Archived warranty", "Do you offer lifetime warranty?", "Archived lifetime warranty should not be used.", []string{"lifetime warranty"}, "archived")
	seedMerchantKnowledgeWithStatus(t, f, f.orgB, bot.KnowledgeKindWarranty, "warranty", "Other tenant warranty", "Do you offer lifetime warranty?", "Other tenant lifetime warranty should not be used.", []string{"lifetime warranty"}, core.StatusActive)

	result := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "Do you offer lifetime warranty?")
	if len(result.Entries) != 0 || len(result.Matches) != 0 {
		t.Fatalf("expected no active same-tenant knowledge, got entries=%#v matches=%#v", result.Entries, result.Matches)
	}
	if len(result.Unknown) == 0 {
		t.Fatalf("expected unknown result, got %#v", result)
	}
}

func TestPhaseD2ConflictingActiveEntriesAreDetected(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty 30 days", "What is your warranty policy?", "Warranty lasts 30 days for manufacturing defects.", []string{"warranty"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty 7 days", "What is your warranty policy?", "Warranty lasts 7 days for manufacturing defects.", []string{"warranty"}, core.StatusActive)

	result := retrieveKnowledgeForTest(t, f, f.orgA.orgID, "What is your warranty policy?")
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %#v", result.Conflicts)
	}
	if len(result.Entries) != 0 || len(result.Matches) != 0 {
		t.Fatalf("conflicted answers should not be selected, got entries=%#v matches=%#v", result.Entries, result.Matches)
	}
	if !debugHasSkippedReason(result.Debug, "conflict") {
		t.Fatalf("expected conflict debug, got %#v", result.Debug)
	}
}

func TestPhaseD2ConflictingGroundingUsesDeterministicUncertainty(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "Warranty lasts 30 days."})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty 30 days", "What is your warranty policy?", "Warranty lasts 30 days for manufacturing defects.", []string{"warranty"}, core.StatusActive)
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindWarranty, "warranty", "Warranty 7 days", "What is your warranty policy?", "Warranty lasts 7 days for manufacturing defects.", []string{"warranty"}, core.StatusActive)
	sessionID := f.startSession(t, f.orgA)

	response := f.send(t, f.orgA, sessionID, "What is your warranty policy?")
	assertContainsAll(t, response.Message.Body, []string{"conflicting information"})
	assertContainsNone(t, response.Message.Body, []string{"30 days", "7 days"})
}

func TestPhaseD2UnknownQuestionStillRefusesInsteadOfGuessing(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "Gift wrapping is free."})
	sessionID := f.startSession(t, f.orgA)

	response := f.send(t, f.orgA, sessionID, "Do you offer gift wrapping?")
	assertContainsAll(t, response.Message.Body, []string{"do not have that information"})
	assertContainsNone(t, response.Message.Body, []string{"free", "Gift wrapping is free"})
}

func TestPhaseD2CommerceFactsRemainUnchanged(t *testing.T) {
	f := newAIRegressionFixture(t, &staticRegressionProvider{response: "Urban Runner Sneakers cost NGN 999999."})
	seedMerchantKnowledgeWithStatus(t, f, f.orgA, bot.KnowledgeKindFAQ, "general", "Old sneaker price note", "How much are Urban Runner Sneakers?", "Urban Runner Sneakers used to cost NGN 999999 in an old note.", []string{"urban runner", "price"}, core.StatusActive)
	sessionID := f.startSession(t, f.orgA)

	price := f.send(t, f.orgA, sessionID, "How much is the black size 42 Urban Runner Sneakers?")
	assertContainsAll(t, price.Message.Body, []string{"Urban Runner Sneakers", "Black Size 42", "NGN 28500"})
	assertContainsNone(t, price.Message.Body, []string{"NGN 999999"})
}

func retrieveKnowledgeForTest(t *testing.T, f aiRegressionFixture, orgID uuid.UUID, query string) KnowledgeResult {
	t.Helper()
	result, err := (lexicalFAQRetriever{db: f.db}).Retrieve(context.Background(), KnowledgeRequest{OrganizationID: orgID, Query: query, MaxResults: 5})
	if err != nil {
		t.Fatalf("retrieve knowledge: %v", err)
	}
	return result
}

func seedMerchantKnowledgeWithStatus(t *testing.T, f aiRegressionFixture, tenant regressionTenant, kind, category, title, question, answer string, keywords []string, status string) uuid.UUID {
	t.Helper()
	body, err := json.Marshal(cleanPhaseDKeywords(keywords))
	if err != nil {
		t.Fatalf("marshal keywords: %v", err)
	}
	now := time.Now().UTC()
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
		Status:         status,
		Metadata:       "{}",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := f.db.Create(&entry).Error; err != nil {
		t.Fatalf("seed merchant knowledge: %v", err)
	}
	return entry.ID
}

func debugContainsField(debug []KnowledgeMatchDebug, title, field string) bool {
	for _, item := range debug {
		if item.Title != title {
			continue
		}
		for _, matched := range item.MatchedFields {
			if matched == field || strings.Contains(matched, field) {
				return true
			}
		}
	}
	return false
}

func debugHasSkippedReason(debug []KnowledgeMatchDebug, reason string) bool {
	for _, item := range debug {
		if item.SkippedReason == reason {
			return true
		}
	}
	return false
}
