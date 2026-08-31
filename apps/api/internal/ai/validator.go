package ai

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type ValidationResult struct {
	Valid   bool
	Reasons []string
}

var uuidPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

func validateCustomerResponse(answer string, grounding GroundingContext) ValidationResult {
	answer = strings.TrimSpace(answer)
	result := ValidationResult{Valid: true}
	if answer == "" {
		return result.add("empty_response")
	}
	normalized := normalizeAIText(answer)
	rawMarkers := []string{"tool_calls", "parameters", "get_products", "get_inventory", "check_inventory", "match_faq", "\"name\"", "\"arguments\"", "{\""}
	for _, marker := range rawMarkers {
		if strings.Contains(strings.ToLower(answer), marker) {
			result = result.add("raw_tool_or_json_leakage")
			break
		}
	}
	if uuidPattern.MatchString(answer) {
		result = result.add("internal_id_leakage")
	}
	for _, forbidden := range []string{"system prompt", "developer message", "database schema", "sql", "api key", "access token"} {
		if strings.Contains(normalized, forbidden) {
			result = result.add("internal_detail_leakage")
		}
	}
	if grounding.Organization.Name != "" {
		for _, name := range knownSeedMerchantNames() {
			if !strings.EqualFold(name, grounding.Organization.Name) && strings.Contains(strings.ToLower(answer), strings.ToLower(name)) {
				result = result.add("merchant_identity_confusion")
			}
		}
	}
	result = validatePrices(answer, grounding, result)
	result = validateStores(answer, grounding, result)
	result = validateUnsupportedPolicyClaims(normalized, grounding, result)
	result = validateKnownUnknowns(answer, grounding, result)
	result = validateGroundedComparison(answer, grounding, result)
	result = validateGroundedFAQs(answer, grounding, result)
	result = validateRequiredGroundedFacts(answer, grounding, result)
	return result
}

func (r ValidationResult) add(reason string) ValidationResult {
	r.Valid = false
	r.Reasons = append(r.Reasons, reason)
	return r
}

func validatePrices(answer string, grounding GroundingContext, result ValidationResult) ValidationResult {
	if !strings.Contains(strings.ToLower(answer), "ngn") && !strings.Contains(answer, "₦") {
		return result
	}
	allowed := map[string]struct{}{}
	for _, product := range grounding.Products {
		for _, variant := range product.Variants {
			allowed[normalizeMoneyText(formatGroundedMoney(variant.PriceMinor, variant.Currency))] = struct{}{}
			allowed[fmt.Sprintf("%d", variant.PriceMinor/100)] = struct{}{}
		}
	}
	for _, order := range grounding.Orders {
		allowed[normalizeMoneyText(formatGroundedMoney(order.TotalMinor, order.Currency))] = struct{}{}
	}
	pricePattern := regexp.MustCompile(`(?i)(?:ngn|₦)\s*([0-9][0-9,\.]*)`)
	for _, match := range pricePattern.FindAllString(answer, -1) {
		normalized := normalizeMoneyText(match)
		if _, ok := allowed[normalized]; !ok {
			return result.add("unsupported_or_contradictory_price")
		}
	}
	return result
}

func normalizeMoneyText(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "₦", "ngn")
	value = strings.ReplaceAll(value, ",", "")
	value = strings.ReplaceAll(value, ".00", "")
	value = strings.TrimRight(value, ".")
	return strings.Join(strings.Fields(value), " ")
}

func validateStores(answer string, grounding GroundingContext, result ValidationResult) ValidationResult {
	normalized := normalizeAIText(answer)
	for _, city := range []string{"abuja", "lagos", "port harcourt", "lekki", "ikeja", "yaba", "surulere", "ikoyi", "maitama", "wuse", "ibadan", "jabi", "maryland", "ilupeju", "computer village", "gwarinpa"} {
		if !strings.Contains(normalized, city) {
			continue
		}
		allowed := false
		for _, store := range grounding.Stores {
			if strings.Contains(normalizeAIText(store.Name+" "+store.Address+" "+store.City), city) {
				allowed = true
				break
			}
		}
		for _, faq := range grounding.FAQs {
			if strings.Contains(normalizeAIText(faq.Answer+" "+faq.Question), city) {
				allowed = true
				break
			}
		}
		if !allowed && (strings.Contains(normalized, "deliver") || strings.Contains(normalized, "branch") || strings.Contains(normalized, "store") || strings.Contains(normalized, "available")) {
			return result.add("unsupported_branch_or_location_claim")
		}
	}
	return result
}

func validateUnsupportedPolicyClaims(normalized string, grounding GroundingContext, result ValidationResult) ValidationResult {
	policyTerms := map[string][]string{
		"delivery": {"deliver", "delivery", "shipping"},
		"warranty": {"warranty", "guarantee"},
		"returns":  {"return", "refund", "exchange"},
	}
	for kind, terms := range policyTerms {
		mentioned := false
		for _, term := range terms {
			if strings.Contains(normalized, term) {
				mentioned = true
				break
			}
		}
		if !mentioned {
			continue
		}
		grounded := false
		for _, faq := range grounding.FAQs {
			if faq.Category == kind || strings.Contains(normalizeAIText(faq.Question+" "+faq.Answer), kind) {
				grounded = true
				break
			}
		}
		for _, unknown := range grounding.Unknowns {
			if unknown.Kind == kind && strings.Contains(normalized, "dont have") {
				grounded = true
			}
		}
		if !grounded && (strings.Contains(normalized, "we ") || strings.Contains(normalized, "our ")) {
			return result.add("unsupported_policy_claim")
		}
	}
	return result
}

func validateKnownUnknowns(answer string, grounding GroundingContext, result ValidationResult) ValidationResult {
	if !unknownRequiresRefusal(grounding) && !(grounding.Requested.NeedsFAQ && len(grounding.FAQs) > 0 && !groundedFAQsAnswerRequest(grounding)) {
		return result
	}
	if !meansGroundedUnknownAnswer(answer) {
		return result.add("missing_grounded_unknown")
	}
	return result
}

func validateGroundedComparison(answer string, grounding GroundingContext, result ValidationResult) ValidationResult {
	if grounding.Requested.PriceConstraint != "cheapest" && grounding.Requested.PriceConstraint != "cheaper" {
		return result
	}
	product, variant, ok := cheapestGroundedVariant(grounding.Products)
	if !ok {
		return result
	}
	normalized := normalizeAnswerFactText(answer)
	if product.Name != "" && !containsGroundedPhrase(normalized, product.Name) {
		result = result.add("missing_grounded_comparison_product")
	}
	if variant.Name != "" && !containsGroundedPhrase(normalized, variant.Name) {
		result = result.add("missing_grounded_comparison_variant")
	}
	if !containsGroundedPhrase(normalized, formatGroundedMoney(variant.PriceMinor, variant.Currency)) && !strings.Contains(normalized, fmt.Sprintf("%d", variant.PriceMinor/100)) {
		result = result.add("missing_grounded_comparison_price")
	}
	return result
}

func validateGroundedFAQs(answer string, grounding GroundingContext, result ValidationResult) ValidationResult {
	if !grounding.Requested.NeedsFAQ || len(grounding.FAQs) == 0 {
		return result
	}
	if !groundedFAQsAnswerRequest(grounding) {
		return result
	}
	normalized := normalizeAnswerFactText(answer)
	for _, faq := range grounding.FAQs {
		if !faqAnswerCovered(normalized, faq.Answer) {
			return result.add("missing_grounded_faq")
		}
	}
	return result
}

func faqAnswerCovered(normalizedAnswer, faqAnswer string) bool {
	answerTokens := tokenSetAI(normalizedAnswer)
	faqTokens := tokenSetAI(faqAnswer)
	meaningful := 0
	hits := 0
	for token := range faqTokens {
		if len(token) < 3 || commonFAQToken(token) {
			continue
		}
		meaningful++
		if _, ok := answerTokens[token]; ok {
			hits++
		}
	}
	if meaningful == 0 {
		return true
	}
	return hits >= minInt(meaningful, 3) || float64(hits)/float64(meaningful) >= 0.55
}

func commonFAQToken(token string) bool {
	switch token {
	case "the", "and", "for", "are", "you", "your", "our", "with", "after", "this", "that", "they", "from", "takes":
		return true
	default:
		return false
	}
}

func validateRequiredGroundedFacts(answer string, grounding GroundingContext, result ValidationResult) ValidationResult {
	normalized := normalizeAnswerFactText(answer)
	if grounding.Requested.NeedsInventory && len(grounding.Inventory) > 0 {
		inv := grounding.Inventory[0]
		if inv.ProductName != "" && !containsGroundedPhrase(normalized, inv.ProductName) {
			result = result.add("missing_grounded_product")
		}
		if inv.StoreName != "" && !mentionsGroundedStore(normalized, inv.StoreName) {
			result = result.add("missing_grounded_store")
		}
		if inv.InStock {
			if !mentionsAvailableStock(normalized) || !strings.Contains(normalized, fmt.Sprintf("%d", inv.Available)) {
				result = result.add("missing_grounded_inventory")
			}
		} else if !mentionsUnavailableStock(normalized, inv.Requested, inv.Available) {
			result = result.add("missing_grounded_inventory")
		}
	}
	if requiresGroundedPrice(grounding.Requested.RawText) && len(grounding.Products) > 0 {
		if grounding.Resolved.ProductName != "" && !containsGroundedPhrase(normalized, grounding.Resolved.ProductName) {
			result = result.add("missing_grounded_product")
		}
		if !mentionsRequiredPrice(answer, grounding) {
			result = result.add("missing_grounded_price")
		}
	}
	return result
}

func canAnswerDeterministically(grounding GroundingContext) bool {
	if grounding.Security.Blocked || grounding.Intent == IntentOutOfScope {
		return true
	}
	if unknownRequiresRefusal(grounding) {
		return true
	}
	if grounding.Requested.NeedsFAQ && len(grounding.FAQs) > 0 && !groundedFAQsAnswerRequest(grounding) {
		return true
	}
	if len(grounding.Inventory) > 0 {
		return true
	}
	if grounding.Requested.NeedsFAQ && len(grounding.FAQs) > 0 {
		return true
	}
	if grounding.Requested.PriceConstraint == "cheapest" || grounding.Requested.PriceConstraint == "cheaper" {
		return len(grounding.Products) > 0
	}
	if asksGroundedPrice(grounding.Requested.RawText) && len(grounding.Products) > 0 {
		return true
	}
	if isProductListing(normalizeAIText(grounding.Requested.RawText)) && len(grounding.Products) > 0 {
		return true
	}
	if len(grounding.Products) > 0 && !grounding.Requested.NeedsFAQ && !grounding.Requested.NeedsInventory && !grounding.Requested.NeedsOrders {
		return true
	}
	if grounding.Requested.NeedsStores && !grounding.Requested.NeedsInventory && len(grounding.Stores) > 0 {
		return true
	}
	if len(grounding.Orders) > 0 {
		return true
	}
	return false
}

func unknownRequiresRefusal(grounding GroundingContext) bool {
	if len(grounding.Unknowns) == 0 {
		return false
	}
	hasFacts := len(grounding.Products) > 0 || len(grounding.FAQs) > 0 || len(grounding.Inventory) > 0 || len(grounding.Stores) > 0 || len(grounding.Orders) > 0
	for _, unknown := range grounding.Unknowns {
		switch unknown.Kind {
		case "product", "payment", "promotion", "opening_hours", "warranty", "bulk_orders", "custom_orders", "unsupported_claim", "usage", "memory_claim", "knowledge_conflict":
			return true
		case "location":
			if len(grounding.Stores) == 0 && !faqCoversRequestedLocation(grounding) {
				return true
			}
		case "policy":
			if grounding.Requested.NeedsFAQ && len(grounding.FAQs) == 0 {
				return true
			}
		case "inventory":
			if !hasFacts || grounding.Resolved.ProductStatus == ResolutionNoMatch || grounding.Resolved.StoreStatus == ResolutionNoMatch {
				return true
			}
		}
	}
	return false
}

func hasUnknownKind(grounding GroundingContext, kind string) bool {
	for _, unknown := range grounding.Unknowns {
		if unknown.Kind == kind {
			return true
		}
	}
	return false
}

func faqCoversRequestedLocation(grounding GroundingContext) bool {
	raw := normalizeAIText(grounding.Requested.RawText)
	requestedLocations := []string{}
	for _, location := range []string{"lagos", "abuja", "port harcourt", "lekki", "ikeja", "yaba", "surulere", "ibadan"} {
		if strings.Contains(raw, location) {
			requestedLocations = append(requestedLocations, location)
		}
	}
	if len(requestedLocations) == 0 {
		return len(grounding.FAQs) > 0
	}
	for _, faq := range grounding.FAQs {
		faqText := normalizeAIText(faq.Question + " " + faq.Answer)
		for _, location := range requestedLocations {
			if strings.Contains(faqText, location) {
				return true
			}
		}
	}
	return false
}

func groundedFAQsAnswerRequest(grounding GroundingContext) bool {
	raw := normalizeAIText(grounding.Requested.RawText)
	faqText := normalizeAIText(joinFAQText(grounding.FAQs))
	if faqText == "" {
		return false
	}
	if strings.Contains(raw, "warranty") || strings.Contains(raw, "guarantee") {
		return strings.Contains(faqText, "warranty") || strings.Contains(faqText, "guarantee")
	}
	if strings.Contains(raw, "crypto") || strings.Contains(raw, "bitcoin") || strings.Contains(raw, "card") || strings.Contains(raw, "transfer") {
		return strings.Contains(faqText, "crypto") || strings.Contains(faqText, "bitcoin") || strings.Contains(faqText, "card") || strings.Contains(faqText, "transfer") || strings.Contains(faqText, "payment")
	}
	if strings.Contains(raw, "opening") || strings.Contains(raw, "open 24") || strings.Contains(raw, "hours") {
		if strings.Contains(raw, "delivery") || strings.Contains(raw, "deliver") {
			return true
		}
		return strings.Contains(faqText, "opening") || strings.Contains(faqText, "open") || strings.Contains(faqText, "hours")
	}
	if strings.Contains(raw, "deliver") || strings.Contains(raw, "delivery") || strings.Contains(raw, "shipping") {
		return faqCoversRequestedLocation(grounding)
	}
	return true
}

func joinFAQText(faqs []GroundedFAQ) string {
	parts := make([]string, 0, len(faqs)*2)
	for _, faq := range faqs {
		parts = append(parts, faq.Question, faq.Answer, faq.Category)
	}
	return strings.Join(parts, " ")
}

func meansGroundedUnknownAnswer(answer string) bool {
	normalized := normalizeAIText(answer)
	for _, speculative := range []string{"probably", "usually", "should have", "likely", "i believe", "they should"} {
		if strings.Contains(normalized, speculative) {
			return false
		}
	}
	for _, phrase := range []string{
		"do not have that information",
		"dont have that information",
		"i do not have information",
		"i dont have information",
		"conflicting information",
		"conflicting merchant information",
		"merchant has conflicting",
		"could not find",
		"couldnt find",
		"has not provided",
		"hasnt provided",
		"not provided",
		"not found",
		"not listed",
		"not in this merchants",
		"not in the merchants",
		"no matching",
	} {
		if strings.Contains(normalized, phrase) {
			return !mentionsAvailableStock(normalized)
		}
	}
	return false
}

func requiresGroundedPrice(text string) bool {
	return asksGroundedPrice(text) || isProductListing(normalizeAIText(text))
}

func asksGroundedPrice(text string) bool {
	normalized := normalizeAIText(text)
	for _, term := range []string{"how much", "price", "cost", "sell for", "amount"} {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

func mentionsRequiredPrice(answer string, grounding GroundingContext) bool {
	answerText := normalizeAnswerFactText(answer)
	for _, product := range grounding.Products {
		for _, variant := range product.Variants {
			if grounding.Resolved.VariantID != uuid.Nil && variant.ID != grounding.Resolved.VariantID {
				continue
			}
			if containsGroundedPhrase(answerText, formatGroundedMoney(variant.PriceMinor, variant.Currency)) {
				return true
			}
			if strings.Contains(answerText, fmt.Sprintf("%d", variant.PriceMinor/100)) {
				return true
			}
		}
	}
	return false
}

func normalizeAnswerFactText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.ReplaceAll(value, "\u202f", " ")
	value = strings.ReplaceAll(value, "₦", "NGN ")
	return normalizeAIText(value)
}

func containsGroundedPhrase(normalizedAnswer, phrase string) bool {
	return strings.Contains(normalizedAnswer, normalizeAnswerFactText(phrase))
}

func mentionsGroundedStore(normalizedAnswer, storeName string) bool {
	if containsGroundedPhrase(normalizedAnswer, storeName) {
		return true
	}
	tokens := strings.Fields(normalizeAnswerFactText(storeName))
	if len(tokens) == 0 {
		return false
	}
	cityOrBranch := tokens[len(tokens)-1]
	return strings.Contains(normalizedAnswer, cityOrBranch)
}

func mentionsAvailableStock(normalizedAnswer string) bool {
	if mentionsUnavailableStock(normalizedAnswer, -1, -1) {
		return false
	}
	return strings.Contains(normalizedAnswer, "available") || strings.Contains(normalizedAnswer, "in stock") || strings.Contains(normalizedAnswer, "we have")
}

func mentionsUnavailableStock(normalizedAnswer string, requestedQuantity, availableQuantity int) bool {
	for _, phrase := range []string{
		"not available",
		"not enough",
		"dont have enough",
		"do not have enough",
		"doesnt have enough",
		"insufficient stock",
		"not in stock",
		"out of stock",
		"cannot fulfill",
		"cant fulfill",
		"cannot provide",
		"cant provide",
		"unable to fulfill",
		"not available in the requested quantity",
		"cant meet",
		"cannot meet",
	} {
		if strings.Contains(normalizedAnswer, phrase) {
			return true
		}
	}
	if availableQuantity == 0 && strings.Contains(normalizedAnswer, "0") && (strings.Contains(normalizedAnswer, "available") || strings.Contains(normalizedAnswer, "in stock")) {
		return true
	}
	if strings.Contains(normalizedAnswer, "no ") && strings.Contains(normalizedAnswer, " available") {
		return true
	}
	if requestedQuantity > availableQuantity && requestedQuantity > 0 {
		requested := fmt.Sprintf("%d", requestedQuantity)
		if strings.Contains(normalizedAnswer, "short of "+requested) || strings.Contains(normalizedAnswer, "dont have "+requested+" in stock") || strings.Contains(normalizedAnswer, "do not have "+requested+" in stock") {
			return true
		}
	}
	return false
}

func knownSeedMerchantNames() []string {
	return []string{
		"AuraLane Perfumes",
		"CurlCraft Hair Studio",
		"FitFuel Market",
		"GlowNest Cosmetics",
		"HomeStyle Decor",
		"Loom & Label Clothing",
		"Mama Tola Kitchen",
		"StrideStreet Shoes",
		"VoltHub Electronics",
		"Zuri Gems Jewelry",
		"Bing Chun AI Smoke",
	}
}

func deterministicFallback(grounding GroundingContext) string {
	if grounding.Security.Blocked {
		return grounding.Security.Message
	}
	if grounding.Intent == IntentOutOfScope {
		return "I can help with this merchant's products, stores, policies, inventory, and your orders, but I do not have information for that request."
	}
	if hasUnknownKind(grounding, "knowledge_conflict") {
		return "This merchant has conflicting information for that question, so I cannot answer it with certainty yet. Please ask the merchant to review their knowledge entries."
	}
	if unknownRequiresRefusal(grounding) {
		return "I do not have that information from this merchant yet. I can help with the products, stores, policies, inventory, or order details this merchant has provided."
	}
	if grounding.Requested.NeedsFAQ && len(grounding.FAQs) > 0 && !groundedFAQsAnswerRequest(grounding) {
		return "I do not have that information from this merchant yet. I can help with the products, stores, policies, inventory, or order details this merchant has provided."
	}
	if len(grounding.Inventory) > 0 {
		return deterministicInventoryAnswer(grounding)
	}
	if grounding.Requested.NeedsStores && len(grounding.Stores) > 0 && !grounding.Requested.NeedsInventory {
		lines := []string{"The available stores are:"}
		for _, store := range grounding.Stores {
			lines = append(lines, fmt.Sprintf("- %s, %s", store.Name, strings.TrimSpace(strings.Join([]string{store.Address, store.City}, ", "))))
		}
		return strings.Join(lines, "\n")
	}
	if grounding.Requested.NeedsFAQ && len(grounding.FAQs) > 0 {
		return strings.Join(uniqueFAQAnswers(grounding.FAQs), "\n")
	}
	for _, unknown := range grounding.Unknowns {
		if unknown.Kind == "product" || (grounding.Requested.NeedsFAQ && len(grounding.FAQs) == 0 && len(grounding.Stores) == 0) {
			return "I do not have that information from this merchant yet. I can help with the products, stores, policies, or order details this merchant has provided."
		}
	}
	if len(grounding.Unknowns) > 0 && len(grounding.Products) == 0 && len(grounding.FAQs) == 0 && len(grounding.Inventory) == 0 && len(grounding.Stores) == 0 && len(grounding.Orders) == 0 {
		return "I do not have that information from this merchant yet. I can help with the products, stores, policies, or order details this merchant has provided."
	}
	if grounding.Resolved.ProductStatus == ResolutionAmbiguous {
		return "I found more than one possible item. Please tell me which one you mean."
	}
	if (grounding.Requested.PriceConstraint == "cheapest" || grounding.Requested.PriceConstraint == "cheaper") && len(grounding.Products) > 0 {
		product, variant, ok := cheapestGroundedVariant(grounding.Products)
		if ok {
			return fmt.Sprintf("The cheapest matching item is %s - %s for %s.", product.Name, variant.Name, formatGroundedMoney(variant.PriceMinor, variant.Currency))
		}
	}
	if len(grounding.Products) > 0 {
		lines := []string{"Here are the matching items I found:"}
		for _, product := range grounding.Products {
			for _, variant := range product.Variants {
				lines = append(lines, fmt.Sprintf("- %s - %s - %s", product.Name, variant.Name, formatGroundedMoney(variant.PriceMinor, variant.Currency)))
			}
		}
		return strings.Join(lines, "\n")
	}
	if len(grounding.FAQs) > 0 {
		return strings.Join(uniqueFAQAnswers(grounding.FAQs), "\n")
	}
	if len(grounding.Stores) > 0 {
		lines := []string{"The available stores are:"}
		for _, store := range grounding.Stores {
			lines = append(lines, fmt.Sprintf("- %s, %s", store.Name, strings.TrimSpace(strings.Join([]string{store.Address, store.City}, ", "))))
		}
		return strings.Join(lines, "\n")
	}
	return "I need a little more information to answer that accurately."
}

func uniqueFAQAnswers(faqs []GroundedFAQ) []string {
	answers := make([]string, 0, len(faqs))
	seen := make(map[string]struct{}, len(faqs))
	for _, faq := range faqs {
		answer := strings.TrimSpace(faq.Answer)
		if answer == "" {
			continue
		}
		key := strings.ToLower(strings.Join(strings.Fields(answer), " "))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		answers = append(answers, answer)
	}
	return answers
}

func deterministicInventoryAnswer(grounding GroundingContext) string {
	rows := selectedInventoryRows(grounding)
	if len(rows) == 0 {
		return "I do not have inventory information from this merchant for that request yet."
	}
	if len(rows) == 1 {
		return formatInventoryRow(rows[0], grounding)
	}
	lines := []string{"Here is the current inventory I found:"}
	for _, inv := range rows {
		lines = append(lines, "- "+formatInventoryRow(inv, grounding))
	}
	return strings.Join(lines, "\n")
}

func selectedInventoryRows(grounding GroundingContext) []GroundedInventory {
	if len(grounding.Inventory) <= 1 {
		return grounding.Inventory
	}
	if grounding.Resolved.StoreID != uuid.Nil {
		rows := []GroundedInventory{}
		for _, inv := range grounding.Inventory {
			if inv.StoreID == grounding.Resolved.StoreID {
				rows = append(rows, inv)
			}
		}
		if len(rows) > 0 {
			return rows
		}
	}
	requestedStoreRows := []GroundedInventory{}
	raw := normalizeAIText(grounding.Requested.RawText)
	for _, inv := range grounding.Inventory {
		storeText := normalizeAIText(inv.StoreName)
		parts := strings.Fields(storeText)
		branch := ""
		if len(parts) > 0 {
			branch = parts[len(parts)-1]
		}
		if storeText != "" && (strings.Contains(raw, storeText) || (branch != "" && strings.Contains(raw, branch))) {
			requestedStoreRows = append(requestedStoreRows, inv)
		}
	}
	if len(requestedStoreRows) > 0 {
		return requestedStoreRows
	}
	inStockRows := []GroundedInventory{}
	for _, inv := range grounding.Inventory {
		if inv.InStock {
			inStockRows = append(inStockRows, inv)
		}
	}
	if len(inStockRows) > 0 {
		return inStockRows
	}
	return grounding.Inventory
}

func formatInventoryRow(inv GroundedInventory, grounding GroundingContext) string {
	price := ""
	for _, product := range grounding.Products {
		for _, variant := range product.Variants {
			if variant.ID == inv.VariantID {
				price = " for " + formatGroundedMoney(variant.PriceMinor, variant.Currency)
				break
			}
		}
		if price != "" {
			break
		}
	}
	if inventoryQuestionIsSizeOnly(grounding) {
		return fmt.Sprintf("%s - %s has %d available at %s%s.", inv.ProductName, inv.VariantName, inv.Available, inv.StoreName, price)
	}
	if inv.InStock {
		return fmt.Sprintf("%s - %s is available at %s%s. There are %d available.", inv.ProductName, inv.VariantName, inv.StoreName, price, inv.Available)
	}
	return fmt.Sprintf("%s - %s is not available in the requested quantity at %s%s. Requested: %d; available: %d.", inv.ProductName, inv.VariantName, inv.StoreName, price, inv.Requested, inv.Available)
}

func inventoryQuestionIsSizeOnly(grounding GroundingContext) bool {
	size, ok := grounding.Resolved.Filters["size"]
	if !ok {
		return false
	}
	if sizeValue, err := strconv.Atoi(size); err == nil && sizeValue < 30 {
		return false
	}
	normalized := normalizeAIText(grounding.Requested.RawText)
	for _, term := range []string{"buy", "order", "purchase", "take", "get", "need", "want"} {
		if strings.Contains(normalized, term+" ") && !strings.Contains(normalized, "size") {
			return false
		}
	}
	return true
}

func cheapestGroundedVariant(products []GroundedProduct) (GroundedProduct, GroundedVariant, bool) {
	var bestProduct GroundedProduct
	var bestVariant GroundedVariant
	found := false
	for _, product := range products {
		for _, variant := range product.Variants {
			if !found || variant.PriceMinor < bestVariant.PriceMinor {
				bestProduct = product
				bestVariant = variant
				found = true
			}
		}
	}
	return bestProduct, bestVariant, found
}
