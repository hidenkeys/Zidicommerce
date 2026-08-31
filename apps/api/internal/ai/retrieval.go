package ai

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"gorm.io/gorm"
)

type KnowledgeRetriever interface {
	Retrieve(ctx context.Context, req KnowledgeRequest) (KnowledgeResult, error)
}

type KnowledgeRequest struct {
	OrganizationID uuid.UUID
	Query          string
	MaxResults     int
}

type KnowledgeResult struct {
	Matches   []GroundedFAQ         `json:"matches"`
	Entries   []GroundedKnowledge   `json:"entries,omitempty"`
	Unknown   []UnknownFact         `json:"unknown,omitempty"`
	Debug     []KnowledgeMatchDebug `json:"debug,omitempty"`
	Conflicts []KnowledgeConflict   `json:"conflicts,omitempty"`
}

type lexicalFAQRetriever struct {
	db *gorm.DB
}

func (r lexicalFAQRetriever) Retrieve(ctx context.Context, req KnowledgeRequest) (KnowledgeResult, error) {
	queryParts := splitKnowledgeQuery(req.Query)
	if len(queryParts) == 0 {
		return KnowledgeResult{}, nil
	}
	var entries []bot.KnowledgeEntry
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND status = ?", req.OrganizationID, core.StatusActive).Find(&entries).Error; err != nil {
		return KnowledgeResult{}, err
	}
	var faqs []bot.FAQ
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND status = ?", req.OrganizationID, core.StatusActive).Find(&faqs).Error; err != nil {
		return KnowledgeResult{}, err
	}
	return rankStructuredKnowledge(req, queryParts, entries, faqs), nil
}

func legacyRankStructuredKnowledge(req KnowledgeRequest, queryParts []string, entries []bot.KnowledgeEntry, faqs []bot.FAQ) KnowledgeResult {
	entriesByID := map[uuid.UUID]GroundedKnowledge{}
	matchesByID := map[uuid.UUID]GroundedFAQ{}
	for _, part := range queryParts {
		bestScore := 0.0
		for _, entry := range entries {
			score, matchedOn := scoreKnowledgeEntry(part, entry)
			if score > bestScore {
				bestScore = score
			}
			if score >= 0.42 {
				existing := entriesByID[entry.ID]
				if score > existing.Score {
					grounded := groundedKnowledgeEntry(entry, score, matchedOn)
					entriesByID[entry.ID] = grounded
					matchesByID[entry.ID] = faqFromKnowledgeEntry(grounded)
				}
			}
		}
		for _, faq := range faqs {
			score, matchedOn := scoreKnowledgeFAQ(part, faq)
			if score > bestScore {
				bestScore = score
			}
			if score >= 0.42 {
				existing := matchesByID[faq.ID]
				if score > existing.Score {
					matchesByID[faq.ID] = GroundedFAQ{ID: faq.ID, Question: faq.Question, Answer: faq.Answer, Category: classifyFAQCategory(faq.Question + " " + strings.Join(parseFAQKeywords(faq.Keywords), " ")), Score: score, MatchedOn: matchedOn}
				}
			}
		}
		if bestScore < 0.42 {
			kind := classifyFAQCategory(part)
			if kind == "" {
				kind = "policy"
			}
			returnUnknown := UnknownFact{Kind: kind, Detail: "No configured merchant knowledge matched: " + part}
			matches := knowledgeMatches(matchesByID, req.MaxResults)
			entries := knowledgeEntryMatches(entriesByID, req.MaxResults)
			return KnowledgeResult{Matches: matches, Entries: entries, Unknown: []UnknownFact{returnUnknown}}
		}
	}
	return KnowledgeResult{Matches: knowledgeMatches(matchesByID, req.MaxResults), Entries: knowledgeEntryMatches(entriesByID, req.MaxResults)}
}

func knowledgeMatches(matchesByID map[uuid.UUID]GroundedFAQ, maxResults int) []GroundedFAQ {
	if maxResults <= 0 {
		maxResults = 5
	}
	matches := make([]GroundedFAQ, 0, len(matchesByID))
	for _, match := range matchesByID {
		matches = append(matches, match)
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	return matches
}

func knowledgeEntryMatches(matchesByID map[uuid.UUID]GroundedKnowledge, maxResults int) []GroundedKnowledge {
	if maxResults <= 0 {
		maxResults = 5
	}
	matches := make([]GroundedKnowledge, 0, len(matchesByID))
	for _, match := range matchesByID {
		matches = append(matches, match)
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	return matches
}

type retrievalPlan struct {
	Intent     AIIntent
	Products   bool
	Stores     bool
	Inventory  bool
	FAQ        bool
	Orders     bool
	OutOfScope bool
}

func planRetrieval(text string, state AIConversationState, security SecurityDecision) retrievalPlan {
	if security.Blocked {
		return retrievalPlan{Intent: IntentSecurity}
	}
	normalized := normalizeAIText(text)
	if isOutOfScope(normalized) {
		return retrievalPlan{Intent: IntentOutOfScope, OutOfScope: true}
	}
	if isGenericStoreListing(normalized) && !strings.Contains(normalized, "deliver") {
		return retrievalPlan{Intent: IntentCommerce, Stores: true}
	}
	plan := retrievalPlan{}
	if needsOrder(normalized) {
		plan.Intent = IntentOrderStatus
		plan.Orders = true
	}
	if needsFAQ(normalized) {
		plan.Intent = IntentFAQ
		plan.FAQ = true
	}
	if needsCommerce(normalized) || refersToPrevious(normalized) || state.CurrentProductID != "" {
		if plan.Intent == "" {
			plan.Intent = IntentCommerce
		}
		plan.Products = true
	}
	if needsStore(normalized) {
		plan.Stores = true
	}
	if needsInventory(normalized) || (isStoreFocused(normalized) && (seemsProductSpecific(normalized) || state.CurrentProductID != "")) {
		plan.Inventory = true
		plan.Products = true
		plan.Stores = true
		if plan.Intent == "" || plan.Intent == IntentCommerce {
			plan.Intent = IntentInventory
		}
	}
	if plan.Intent == "" {
		plan.Intent = IntentGeneral
		plan.Products = isProductListing(normalized) || couldBeCatalogueSearch(normalized)
		plan.Stores = needsStore(normalized)
		plan.FAQ = needsFAQ(normalized)
	}
	return plan
}

type retrievalEngine struct {
	db        *gorm.DB
	commerce  *core.Service
	knowledge KnowledgeRetriever
}

func newRetrievalEngine(db *gorm.DB, commerce *core.Service) retrievalEngine {
	return retrievalEngine{db: db, commerce: commerce, knowledge: lexicalFAQRetriever{db: db}}
}

func newRetrievalEngineWithKnowledge(db *gorm.DB, commerce *core.Service, knowledge KnowledgeRetriever) retrievalEngine {
	if knowledge == nil {
		knowledge = lexicalFAQRetriever{db: db}
	}
	return retrievalEngine{db: db, commerce: commerce, knowledge: knowledge}
}

func (r retrievalEngine) BuildGrounding(ctx context.Context, session runtime.ConversationSession, text string, state AIConversationState, security SecurityDecision) (GroundingContext, []UnknownFact, error) {
	merchant, err := r.merchantIdentity(ctx, session.OrganizationID)
	if err != nil {
		return GroundingContext{}, nil, err
	}
	plan := planRetrieval(text, state, security)
	grounding := GroundingContext{
		Organization: merchant,
		Intent:       plan.Intent,
		Security:     security,
		Requested:    RequestedFacts{RawText: strings.TrimSpace(text), Quantity: 1},
		Constraints:  AnswerConstraints{Forbidden: []string{"raw_json", "tool_names", "uuids", "internal_ids", "other_merchants"}},
	}
	if security.Blocked {
		grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "security", Detail: security.Reason})
		return grounding, grounding.Unknowns, nil
	}
	if plan.OutOfScope {
		grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "out_of_scope", Detail: "Question is outside merchant customer-service scope"})
		return grounding, grounding.Unknowns, nil
	}
	actor := auth.CurrentUser{ID: uuid.Nil, OrganizationID: session.OrganizationID, Role: authz.MerchantAdmin}
	var products []core.Product
	var stores []core.Store
	var categories []core.Category
	var inventory []core.InventoryLevel
	if r.commerce == nil {
		grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "commerce", Detail: "Commerce retrieval is not configured"})
		return grounding, grounding.Unknowns, nil
	}
	if plan.Products || plan.Inventory {
		products, err = r.commerce.ListProducts(ctx, actor)
		if err != nil {
			return GroundingContext{}, nil, err
		}
		grounding.Sources = append(grounding.Sources, GroundingSource{Name: "products", Authority: "postgresql", Confidence: 1})
	}
	if plan.Stores || plan.Inventory {
		stores, err = r.commerce.ListStores(ctx, actor)
		if err != nil {
			return GroundingContext{}, nil, err
		}
		grounding.Sources = append(grounding.Sources, GroundingSource{Name: "stores", Authority: "postgresql", Confidence: 1})
	}
	if plan.Products {
		categories, err = r.commerce.ListCategories(ctx, actor)
		if err != nil {
			return GroundingContext{}, nil, err
		}
	}
	resolved := resolveEntities(ResolverInput{Text: text, State: state, Products: products, Categories: categories, Stores: stores})
	grounding.Requested = resolved.Requested
	grounding.Requested.NeedsProducts = plan.Products
	grounding.Requested.NeedsStores = plan.Stores
	grounding.Requested.NeedsInventory = plan.Inventory
	grounding.Requested.NeedsFAQ = plan.FAQ
	grounding.Requested.NeedsOrders = plan.Orders
	grounding.Resolved = resolved.Resolved
	grounding.Products = resolved.Products
	normalizedText := normalizeAIText(text)
	if plan.Products && len(grounding.Products) == 0 && seemsProductSpecific(normalizedText) && !isStoreFocused(normalizedText) {
		grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "product", Detail: "No matching product was found for the request"})
	}
	if (plan.Products || plan.Inventory) && resolved.Resolved.ProductStatus == ResolutionNoMatch {
		grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "product", Detail: "No matching product was found for the request"})
	}
	if len(resolved.Stores) > 0 {
		grounding.Stores = resolved.Stores
	} else if resolved.Resolved.StoreStatus == ResolutionNoMatch {
		grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "location", Detail: "No matching store or branch was found for the request"})
	} else if plan.Stores {
		for _, store := range stores {
			if store.Status == core.StatusActive {
				grounding.Stores = append(grounding.Stores, GroundedStore{ID: store.ID, Name: store.Name, Code: store.Code, Address: store.Address, City: store.City})
			}
		}
	}
	if plan.Inventory {
		if grounding.Resolved.StoreStatus == ResolutionNoMatch {
			grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "inventory", Detail: "Inventory was not checked because the requested store or branch was not found"})
		} else {
			var storeID *uuid.UUID
			if grounding.Resolved.StoreID != uuid.Nil {
				storeID = &grounding.Resolved.StoreID
			}
			inventory, err = r.commerce.ListInventory(ctx, actor, storeID)
			if err != nil {
				return GroundingContext{}, nil, err
			}
			grounding.Inventory = groundedInventory(inventory, grounding, stores)
		}
		grounding.Sources = append(grounding.Sources, GroundingSource{Name: "inventory", Authority: "postgresql", Confidence: 1})
		if len(grounding.Inventory) == 0 {
			grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "inventory", Detail: "No inventory record matched the requested item/store"})
		}
	}
	if plan.FAQ {
		result, err := r.knowledge.Retrieve(ctx, KnowledgeRequest{OrganizationID: session.OrganizationID, Query: text, MaxResults: 5})
		if err != nil {
			return GroundingContext{}, nil, err
		}
		grounding.FAQs = result.Matches
		grounding.Knowledge = result.Entries
		grounding.Unknowns = append(grounding.Unknowns, result.Unknown...)
		grounding.Sources = append(grounding.Sources, GroundingSource{Name: "merchant_knowledge_entries,bot_faqs", Authority: "merchant_knowledge", Confidence: 1})
		if len(result.Matches) == 0 && len(result.Unknown) == 0 {
			grounding.Unknowns = append(grounding.Unknowns, UnknownFact{Kind: "policy", Detail: "No configured merchant knowledge matched the request"})
		}
	}
	if plan.Orders && session.CustomerID != nil {
		orders, err := r.commerce.ListCustomerOrders(ctx, actor, *session.CustomerID)
		if err != nil {
			return GroundingContext{}, nil, err
		}
		for _, order := range orders {
			grounding.Orders = append(grounding.Orders, GroundedOrder{ID: order.ID, OrderNumber: order.OrderNumber, Status: order.Status, TotalMinor: order.TotalMinor, Currency: order.Currency})
			if len(grounding.Orders) >= 5 {
				break
			}
		}
		grounding.Sources = append(grounding.Sources, GroundingSource{Name: "customer_orders", Authority: "postgresql", Confidence: 1})
	}
	setConstraints(&grounding)
	return grounding, grounding.Unknowns, nil
}

func (r retrievalEngine) merchantIdentity(ctx context.Context, orgID uuid.UUID) (MerchantIdentity, error) {
	var org organization.Organization
	if err := r.db.WithContext(ctx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return MerchantIdentity{ID: orgID, Name: "current merchant"}, nil
	}
	return MerchantIdentity{ID: org.ID, Name: org.Name, Description: org.Description, Currency: org.Currency, Country: org.Country}, nil
}

func groundedInventory(levels []core.InventoryLevel, grounding GroundingContext, stores []core.Store) []GroundedInventory {
	rows := []GroundedInventory{}
	if grounding.Resolved.ProductStatus == ResolutionNoMatch {
		return rows
	}
	storeNames := map[uuid.UUID]string{}
	for _, store := range stores {
		storeNames[store.ID] = store.Name
	}
	for _, level := range levels {
		if grounding.Resolved.VariantID != uuid.Nil && level.VariantID != grounding.Resolved.VariantID {
			continue
		}
		productName := level.Variant.Product.Name
		if productName == "" {
			productName = grounding.Resolved.ProductName
		}
		if grounding.Resolved.ProductID != uuid.Nil && level.Variant.ProductID != grounding.Resolved.ProductID {
			continue
		}
		rows = append(rows, GroundedInventory{
			StoreID:     level.StoreID,
			StoreName:   storeNames[level.StoreID],
			ProductID:   level.Variant.ProductID,
			ProductName: productName,
			VariantID:   level.VariantID,
			VariantName: level.Variant.Name,
			Available:   level.Available(),
			Requested:   grounding.Requested.Quantity,
			InStock:     level.Available() >= grounding.Requested.Quantity,
		})
		if len(rows) >= 8 {
			break
		}
	}
	return rows
}

func setConstraints(grounding *GroundingContext) {
	grounding.Constraints.AllowProducts = len(grounding.Products) > 0
	grounding.Constraints.AllowPrices = len(grounding.Products) > 0
	grounding.Constraints.AllowInventory = len(grounding.Inventory) > 0
	grounding.Constraints.AllowStores = len(grounding.Stores) > 0
	grounding.Constraints.AllowPolicies = len(grounding.FAQs) > 0 || len(grounding.Knowledge) > 0
	grounding.Constraints.AllowOrders = len(grounding.Orders) > 0
}

func splitKnowledgeQuery(query string) []string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	replacer := strings.NewReplacer(
		" and ", "|",
		" also ", "|",
		" plus ", "|",
		"? ", "|",
		". ", "|",
		"; ", "|",
	)
	parts := strings.Split(replacer.Replace(normalized), "|")
	out := []string{}
	for _, part := range parts {
		part = normalizeAIText(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func scoreKnowledgeFAQ(query string, faq bot.FAQ) (float64, string) {
	question := normalizeAIText(faq.Question)
	answer := normalizeAIText(faq.Answer)
	keywords := parseFAQKeywords(faq.Keywords)
	queryCategory := classifyFAQCategory(query)
	faqCategory := classifyFAQCategory(faq.Question + " " + strings.Join(keywords, " "))
	joinedFAQ := question + " " + answer + " " + normalizeAIText(strings.Join(keywords, " "))
	if strings.Contains(query, "spf") && !strings.Contains(joinedFAQ, "spf") {
		return 0, ""
	}
	if question == query {
		return 1, "question"
	}
	if strings.Contains(question, query) || strings.Contains(query, question) {
		return 0.86, "question"
	}
	best := scoreText(query, question)
	matchedOn := "question"
	if score := scoreText(query, answer) * 0.45; score > best {
		best = score
		matchedOn = "answer"
	}
	for _, keyword := range keywords {
		keyword = normalizeAIText(keyword)
		if keyword == "" {
			continue
		}
		score := scoreText(query, keyword) * 0.9
		if strings.Contains(query, keyword) || strings.Contains(keyword, query) {
			score = 0.92
		}
		if score > best {
			best = score
			matchedOn = "keyword"
		}
	}
	if queryCategory != "" && queryCategory == faqCategory && best < 0.55 {
		best = 0.55
		matchedOn = "category"
	}
	return best, matchedOn
}

func scoreKnowledgeEntry(query string, entry bot.KnowledgeEntry) (float64, string) {
	title := normalizeAIText(entry.Title)
	question := normalizeAIText(entry.Question)
	answer := normalizeAIText(entry.Answer)
	kind := normalizeAIText(entry.Kind)
	category := normalizeAIText(entry.Category)
	keywords := parseFAQKeywords(entry.Keywords)
	queryCategory := classifyFAQCategory(query)
	entryCategory := firstNonEmpty(category, classifyFAQCategory(entry.Kind+" "+entry.Category+" "+entry.Title+" "+entry.Question+" "+strings.Join(keywords, " ")))
	joined := strings.Join([]string{title, question, answer, kind, category, normalizeAIText(strings.Join(keywords, " "))}, " ")
	if strings.Contains(query, "spf") && !strings.Contains(joined, "spf") {
		return 0, ""
	}
	if question != "" && question == query {
		return 1, "question"
	}
	if title != "" && title == query {
		return 0.96, "title"
	}
	if question != "" && (strings.Contains(question, query) || strings.Contains(query, question)) {
		return 0.86, "question"
	}
	if title != "" && (strings.Contains(title, query) || strings.Contains(query, title)) {
		return 0.82, "title"
	}
	best := scoreText(query, question)
	matchedOn := "question"
	if score := scoreText(query, title) * 0.9; score > best {
		best = score
		matchedOn = "title"
	}
	if score := scoreText(query, answer) * 0.45; score > best {
		best = score
		matchedOn = "answer"
	}
	if score := scoreText(query, kind+" "+category) * 0.75; score > best {
		best = score
		matchedOn = "category"
	}
	for _, keyword := range keywords {
		keyword = normalizeAIText(keyword)
		if keyword == "" {
			continue
		}
		score := scoreText(query, keyword) * 0.9
		if strings.Contains(query, keyword) || strings.Contains(keyword, query) {
			score = 0.92
		}
		if score > best {
			best = score
			matchedOn = "keyword"
		}
	}
	if queryCategory != "" && queryCategory == entryCategory && best < 0.55 {
		best = 0.55
		matchedOn = "category"
	}
	return best, matchedOn
}

func groundedKnowledgeEntry(entry bot.KnowledgeEntry, score float64, matchedOn string) GroundedKnowledge {
	category := strings.TrimSpace(entry.Category)
	if category == "" {
		category = classifyFAQCategory(entry.Kind + " " + entry.Title + " " + entry.Question + " " + strings.Join(parseFAQKeywords(entry.Keywords), " "))
	}
	if category == "" {
		category = "general"
	}
	kind := strings.TrimSpace(entry.Kind)
	if kind == "" {
		kind = bot.KnowledgeKindFAQ
	}
	return GroundedKnowledge{
		ID:         entry.ID,
		Kind:       kind,
		Category:   category,
		Title:      entry.Title,
		Question:   entry.Question,
		Answer:     entry.Answer,
		SourceType: entry.SourceType,
		Score:      score,
		MatchedOn:  matchedOn,
	}
}

func faqFromKnowledgeEntry(entry GroundedKnowledge) GroundedFAQ {
	question := strings.TrimSpace(entry.Question)
	if question == "" {
		question = entry.Title
	}
	return GroundedFAQ{
		ID:        entry.ID,
		Question:  question,
		Answer:    entry.Answer,
		Category:  firstNonEmpty(entry.Category, entry.Kind),
		Score:     entry.Score,
		MatchedOn: entry.MatchedOn,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseFAQKeywords(raw string) []string {
	var keywords []string
	_ = json.Unmarshal([]byte(defaultJSONArray(raw)), &keywords)
	return keywords
}

func defaultJSONArray(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	return value
}

func classifyFAQCategory(text string) string {
	text = normalizeAIText(text)
	categories := []struct {
		name  string
		terms []string
	}{
		{"delivery", []string{"delivery", "deliver", "shipping", "ship", "rider", "dispatch", "bring"}},
		{"returns", []string{"return", "refund", "exchange"}},
		{"warranty", []string{"warranty", "guarantee"}},
		{"opening_hours", []string{"opening", "hours", "open", "close"}},
		{"payment", []string{"payment", "pay", "card", "cash", "transfer", "pos"}},
		{"usage", []string{"sensitive skin", "patch test", "how to use", "safe for", "spf"}},
		{"unsupported_claim", []string{"cure", "treat pain", "knee pain", "medical"}},
		{"memory_claim", []string{"you told me", "yesterday"}},
		{"promotion", []string{"discount", "coupon", "promo code", "voucher"}},
		{"bulk_orders", []string{"bulk", "large order", "notice"}},
		{"custom_orders", []string{"custom", "engraving", "customize", "alteration", "reserve", "subscribe", "subscription", "gift wrap"}},
		{"locations", []string{"location", "branch", "store", "where"}},
		{"support", []string{"human", "agent", "support", "complaint", "speak to someone"}},
	}
	for _, category := range categories {
		for _, term := range category.terms {
			if strings.Contains(text, term) {
				return category.name
			}
		}
	}
	return ""
}

func needsFAQ(text string) bool {
	return classifyFAQCategory(text) != "" || strings.Contains(text, "policy")
}

func needsCommerce(text string) bool {
	for _, term := range []string{"product", "products", "menu", "sell", "have", "price", "how much", "cost", "ngn", "buy", "order", "available", "stock", "can i get", "pickup", "pick up", "size", "shade", "color", "colour", "cheaper", "cheapest", "compare", "phone", "shoe", "sneaker", "sandals", "kit", "dress", "cream", "serum", "food", "perfume", "jewelry", "jewellery", "you get", "una get", "do una get"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func needsInventory(text string) bool {
	if category := classifyFAQCategory(text); category != "" && category != "locations" && !strings.Contains(text, "stock") && !strings.Contains(text, "available") && !strings.Contains(text, "do you have") && !strings.Contains(text, "how many") && !strings.Contains(text, "i need") && !strings.Contains(text, "i want") {
		return false
	}
	for _, term := range []string{"available", "stock", "in stock", "do you have", "can i get", "quantity", "how many", "i need", "i want", "ill take", "i will take", "pick", "pickup", "what about", "you get", "una get", "do una get"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func needsStore(text string) bool {
	return isStoreFocused(text)
}

func isStoreFocused(text string) bool {
	for _, term := range []string{"branch", "store", "where", "located", "location"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	if strings.Contains(text, "in stock") {
		return false
	}
	padded := " " + text + " "
	return strings.Contains(padded, " in ") || strings.Contains(padded, " at ")
}

func needsOrder(text string) bool {
	for _, term := range []string{"my order", "order status", "track", "receipt", "past purchase", "recent order"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func isOutOfScope(text string) bool {
	for _, term := range []string{"weather", "president", "bitcoin", "capital of france", "capital city"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func couldBeCatalogueSearch(text string) bool {
	if text == "" {
		return false
	}
	for _, greeting := range []string{"hi", "hello", "hey", "good morning", "good afternoon", "good evening"} {
		if text == greeting {
			return false
		}
	}
	return len(strings.Fields(text)) <= 5
}
