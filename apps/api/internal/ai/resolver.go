package ai

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

type ResolverInput struct {
	Text       string
	State      AIConversationState
	Products   []core.Product
	Categories []core.Category
	Stores     []core.Store
}

type ResolverResult struct {
	Requested RequestedFacts
	Resolved  ResolvedEntities
	Products  []GroundedProduct
	Stores    []GroundedStore
	Unknowns  []UnknownFact
}

func resolveEntities(input ResolverInput) ResolverResult {
	text := normalizeAIText(input.Text)
	filters := extractFilters(text)
	requested := RequestedFacts{
		RawText:         strings.TrimSpace(input.Text),
		Terms:           strings.Fields(text),
		Quantity:        extractQuantity(text),
		PriceConstraint: extractPriceConstraint(text),
	}
	resolved := ResolvedEntities{Filters: filters}
	storeMatches := rankStores(text, input.Stores)
	if len(storeMatches) == 1 || (len(storeMatches) > 1 && storeMatches[0].Score-storeMatches[1].Score >= 0.25) {
		resolved.StoreID = storeMatches[0].ID
		resolved.StoreName = storeMatches[0].Name
		resolved.StoreStatus = ResolutionExact
	}
	if len(storeMatches) > 1 && resolved.StoreID == uuid.Nil && storeMatches[0].Score >= 0.35 {
		resolved.StoreStatus = ResolutionAmbiguous
		resolved.AmbiguousReason = "multiple stores match the request"
	} else if len(storeMatches) == 0 && isStoreFocused(text) && !isGenericStoreListing(text) {
		resolved.StoreStatus = ResolutionNoMatch
		resolved.AmbiguousReason = "no store matched the request"
	}
	productMatches := rankProducts(text, filters, input)
	productMatches = filterWeakProductMatches(text, productMatches)
	if len(productMatches) == 0 {
		if seemsProductSpecific(text) {
			resolved.ProductStatus = ResolutionNoMatch
			resolved.AmbiguousReason = "no product matched the request"
		}
	} else if isProductListing(text) && len(productMatches) > 1 {
		// Listing requests should return the scoped catalogue instead of forcing
		// the customer to disambiguate between every matching product.
	} else if requested.PriceConstraint == "cheapest" || requested.PriceConstraint == "cheaper" {
		resolved.ProductID = productMatches[0].ID
		resolved.ProductName = productMatches[0].Name
		resolved.ProductStatus = ResolutionExact
		if len(productMatches[0].Variants) == 1 {
			resolved.VariantID = productMatches[0].Variants[0].ID
			resolved.VariantName = productMatches[0].Variants[0].Name
		}
	} else if len(productMatches) == 1 || productMatches[0].Score-productMatches[minInt(1, len(productMatches)-1)].Score >= 0.22 || productMatches[0].Score >= 0.78 {
		resolved.ProductID = productMatches[0].ID
		resolved.ProductName = productMatches[0].Name
		resolved.ProductStatus = ResolutionExact
		if len(productMatches[0].Variants) == 1 {
			resolved.VariantID = productMatches[0].Variants[0].ID
			resolved.VariantName = productMatches[0].Variants[0].Name
		} else if variant := bestVariant(productMatches[0], text, filters, input.State); variant.ID != uuid.Nil {
			resolved.VariantID = variant.ID
			resolved.VariantName = variant.Name
		}
	} else {
		resolved.ProductStatus = ResolutionAmbiguous
		resolved.AmbiguousReason = "multiple products match the request"
	}
	if resolved.ProductID == uuid.Nil {
		if fromState := productFromState(input.State, input.Products); fromState.ID != uuid.Nil && refersToPrevious(text) {
			resolved.ProductID = fromState.ID
			resolved.ProductName = fromState.Name
			resolved.ProductStatus = ResolutionCandidate
			if variant := bestVariant(fromState, text, filters, input.State); variant.ID != uuid.Nil {
				resolved.VariantID = variant.ID
				resolved.VariantName = variant.Name
			}
			productMatches = append([]GroundedProduct{fromState}, productMatches...)
		}
	}
	if category := resolveCategory(text, input.Categories, input.State); category.ID != uuid.Nil {
		resolved.CategoryID = category.ID
		resolved.CategoryName = category.Name
	}
	requested.NeedsClarification = resolved.ProductStatus == ResolutionAmbiguous || resolved.StoreStatus == ResolutionAmbiguous
	return ResolverResult{Requested: requested, Resolved: resolved, Products: limitProducts(productMatches, 6), Stores: limitStores(storeMatches, 4)}
}

func filterWeakProductMatches(text string, matches []GroundedProduct) []GroundedProduct {
	if len(matches) == 0 || isProductListing(text) || extractPriceConstraint(text) != "" {
		return matches
	}
	if !seemsProductSpecific(text) && !needsInventory(text) {
		return matches
	}
	specificTokens := discriminativeProductTokens(text)
	filtered := matches[:0]
	for _, match := range matches {
		if len(specificTokens) > 0 && !productMatchesAnyToken(match, specificTokens) {
			continue
		}
		if match.Score >= 0.35 || productNameReferenced(text, match.Name) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func discriminativeProductTokens(text string) map[string]struct{} {
	tokens := tokenSetAI(text)
	for _, token := range []string{
		"a", "an", "and", "are", "available", "buy", "can", "do", "for", "get", "have", "how", "i", "in", "is", "item", "me", "much", "need", "of", "order", "please", "price", "product", "stock", "the", "to", "want", "you",
		"beverage", "drink", "milkshake", "smoothie", "sundae", "tea", "topping",
	} {
		delete(tokens, singularAI(token))
	}
	return tokens
}

func productMatchesAnyToken(product GroundedProduct, tokens map[string]struct{}) bool {
	parts := []string{product.Name, product.Description}
	for _, variant := range product.Variants {
		parts = append(parts, variant.Name)
		for _, value := range variant.Attributes {
			parts = append(parts, value)
		}
	}
	productTokens := tokenSetAI(strings.Join(parts, " "))
	for token := range tokens {
		if _, ok := productTokens[token]; ok {
			return true
		}
	}
	return false
}

func rankProducts(text string, filters map[string]string, input ResolverInput) []GroundedProduct {
	matches := []GroundedProduct{}
	categoryHint := resolveCategory(text, input.Categories, input.State)
	for _, product := range input.Products {
		if product.Status != core.StatusActive {
			continue
		}
		score := scoreText(text, product.Name+" "+product.Description+" "+product.Slug)
		if productNameReferenced(text, product.Name) {
			score = maxFloat(score, 0.95)
		}
		if categoryHint.ID != uuid.Nil && product.CategoryID != nil && *product.CategoryID == categoryHint.ID {
			score += 0.25
		}
		var variants []GroundedVariant
		for _, variant := range product.Variants {
			if variant.Status != core.StatusActive {
				continue
			}
			attrs := variantAttributes(product, variant)
			variantScore := scoreText(text, variant.Name+" "+variant.SKU)
			for key, value := range filters {
				if attrs[key] == value {
					variantScore += 0.35
					score += 0.15
				}
			}
			if variantScore > 0 || score > 0 || len(filters) == 0 {
				variants = append(variants, GroundedVariant{ID: variant.ID, ProductID: product.ID, Name: variant.Name, PriceMinor: variant.PriceMinor, Currency: variant.Currency, Attributes: attrs, Score: variantScore})
			}
		}
		if score > 0 || anyVariantScore(variants) > 0 || (categoryHint.ID != uuid.Nil && product.CategoryID != nil && *product.CategoryID == categoryHint.ID) {
			sort.SliceStable(variants, func(i, j int) bool { return variants[i].Score > variants[j].Score })
			matches = append(matches, GroundedProduct{ID: product.ID, Name: product.Name, Description: product.Description, CategoryID: product.CategoryID, Variants: variants, Score: score + anyVariantScore(variants)})
		}
	}
	priceConstraint := extractPriceConstraint(text)
	if len(matches) == 0 && (isProductListing(text) || priceConstraint == "cheapest" || priceConstraint == "cheaper") {
		for _, product := range input.Products {
			if product.Status != core.StatusActive {
				continue
			}
			matches = append(matches, groundedProduct(product, 0.1))
		}
	}
	if priceConstraint == "cheapest" || priceConstraint == "cheaper" {
		sortByLowestPrice(matches)
		return matches
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	return matches
}

func groundedProduct(product core.Product, score float64) GroundedProduct {
	variants := []GroundedVariant{}
	for _, variant := range product.Variants {
		if variant.Status != core.StatusActive {
			continue
		}
		variants = append(variants, GroundedVariant{ID: variant.ID, ProductID: product.ID, Name: variant.Name, PriceMinor: variant.PriceMinor, Currency: variant.Currency, Attributes: variantAttributes(product, variant)})
	}
	return GroundedProduct{ID: product.ID, Name: product.Name, Description: product.Description, CategoryID: product.CategoryID, Variants: variants, Score: score}
}

func rankStores(text string, stores []core.Store) []GroundedStore {
	matches := []GroundedStore{}
	for _, store := range stores {
		if store.Status != core.StatusActive {
			continue
		}
		score := scoreText(text, store.Name+" "+store.Code+" "+store.Address+" "+store.City)
		if score > 0 {
			matches = append(matches, GroundedStore{ID: store.ID, Name: store.Name, Code: store.Code, Address: store.Address, City: store.City, Score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	return matches
}

func resolveCategory(text string, categories []core.Category, state AIConversationState) core.Category {
	best := core.Category{}
	bestScore := 0.0
	for _, category := range categories {
		if category.Status != core.StatusActive {
			continue
		}
		if categorySynonymMatches(text, category.Name) {
			return category
		}
		score := scoreText(text, category.Name+" "+category.Slug)
		if normalizeAIText(category.Name) == normalizeAIText(state.CurrentCategory) && refersToPrevious(text) {
			score += 0.25
		}
		if score > bestScore {
			best = category
			bestScore = score
		}
	}
	if bestScore >= 0.35 {
		return best
	}
	return core.Category{}
}

func categorySynonymMatches(text, categoryName string) bool {
	categoryName = normalizeAIText(categoryName)
	switch categoryName {
	case "shoes", "shoe":
		return strings.Contains(text, "shoe") || strings.Contains(text, "sneaker") || strings.Contains(text, "footwear")
	default:
		return false
	}
}

func productFromState(state AIConversationState, products []core.Product) GroundedProduct {
	id := uuid.Nil
	var preferredVariant uuid.UUID
	if state.CurrentProductID != "" {
		parsed, err := uuid.Parse(state.CurrentProductID)
		if err == nil {
			id = parsed
		}
	} else if len(state.LastCandidates) > 0 {
		parsed, err := uuid.Parse(state.LastCandidates[0].ProductID)
		if err == nil {
			id = parsed
		}
		if variantID, err := uuid.Parse(state.LastCandidates[0].VariantID); err == nil {
			preferredVariant = variantID
		}
	}
	if id == uuid.Nil {
		return GroundedProduct{}
	}
	for _, product := range products {
		if product.ID == id {
			grounded := groundedProduct(product, 0.55)
			if preferredVariant != uuid.Nil {
				for _, variant := range grounded.Variants {
					if variant.ID == preferredVariant {
						grounded.Variants = []GroundedVariant{variant}
						break
					}
				}
			}
			return grounded
		}
	}
	return GroundedProduct{}
}

func bestVariant(product GroundedProduct, text string, filters map[string]string, state AIConversationState) GroundedVariant {
	best := GroundedVariant{}
	bestScore := 0.0
	for _, variant := range product.Variants {
		score := scoreText(text, variant.Name)
		for key, value := range filters {
			if variant.Attributes[key] == value {
				score += 0.45
			}
		}
		if state.CurrentVariantID == variant.ID.String() && refersToPrevious(text) {
			score += 0.25
		}
		if score > bestScore {
			best = variant
			bestScore = score
		}
	}
	if bestScore >= 0.25 {
		return best
	}
	return GroundedVariant{}
}

func extractFilters(text string) map[string]string {
	attrs := extractAttributes(text)
	out := map[string]string{}
	for key, value := range attrs {
		out[key] = value
	}
	return out
}

func variantAttributes(product core.Product, variant core.Variant) map[string]string {
	attrs := extractAttributes(product.Name + " " + product.Description)
	for key, value := range extractAttributes(variant.Name + " " + variant.SKU) {
		attrs[key] = value
	}
	return attrs
}

func extractAttributes(text string) map[string]string {
	normalized := normalizeAIText(text)
	attrs := map[string]string{}
	colors := []string{"black", "white", "blue", "navy", "red", "green", "sage", "gold", "silver", "pearl", "amber", "nude", "dark", "midnight"}
	for _, color := range colors {
		if hasToken(normalized, color) {
			if color == "navy" || color == "blue" {
				attrs["color"] = color
			} else if _, ok := attrs["color"]; !ok {
				attrs["color"] = color
			}
		}
	}
	sizeRe := regexp.MustCompile(`\b(size\s*)?([0-9]{1,3}|small|medium|large|standard|regular|queen|pair)\b`)
	if match := sizeRe.FindStringSubmatch(normalized); len(match) > 2 {
		attrs["size"] = strings.TrimSpace(match[2])
	}
	capacityRe := regexp.MustCompile(`\b([0-9]+)\s*(ml|kg|g|gb|watt|inch|piece|pack|tablets?|bands?)\b`)
	if match := capacityRe.FindStringSubmatch(normalized); len(match) > 2 {
		attrs["capacity"] = match[1] + " " + match[2]
	}
	if strings.Contains(normalized, "shade") {
		shadeRe := regexp.MustCompile(`\bshade\s*([0-9a-z]+)\b`)
		if match := shadeRe.FindStringSubmatch(normalized); len(match) > 1 {
			attrs["shade"] = match[1]
		}
	}
	materials := []string{"leather", "linen", "cotton", "ceramic", "rattan", "sterling", "gold plated", "silver"}
	for _, material := range materials {
		if strings.Contains(normalized, material) {
			attrs["material"] = material
			break
		}
	}
	return attrs
}

func extractQuantity(text string) int {
	tokens := strings.Fields(normalizeAIText(text))
	for i, token := range tokens {
		value, err := strconv.Atoi(token)
		if err != nil {
			value = wordNumberAI(token)
			if value == 0 {
				continue
			}
		}
		if i > 0 && tokens[i-1] == "size" {
			continue
		}
		if looksLikeStandaloneSize(tokens, i, value) {
			continue
		}
		if i+1 < len(tokens) {
			switch tokens[i+1] {
			case "ml", "kg", "g", "gb", "watt", "inch":
				continue
			}
		}
		if value > 0 && value < 100 {
			return value
		}
	}
	return 1
}

func looksLikeStandaloneSize(tokens []string, index, value int) bool {
	if value < 30 || value > 55 {
		return false
	}
	for i, token := range tokens {
		if token == "size" && absInt(i-index) <= 2 {
			return true
		}
	}
	for _, token := range tokens {
		switch token {
		case "pairs", "pair", "pieces", "piece", "units", "unit", "qty", "quantity":
			return false
		case "take", "buy", "order", "purchase":
			return false
		}
	}
	return true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func wordNumberAI(token string) int {
	switch token {
	case "one":
		return 1
	case "two":
		return 2
	case "three":
		return 3
	case "four":
		return 4
	case "five":
		return 5
	case "six":
		return 6
	case "seven":
		return 7
	case "eight":
		return 8
	case "nine":
		return 9
	case "ten":
		return 10
	default:
		return 0
	}
}

func extractPriceConstraint(text string) string {
	switch {
	case strings.Contains(text, "cheapest"):
		return "cheapest"
	case strings.Contains(text, "cheaper") || strings.Contains(text, "less expensive"):
		return "cheaper"
	case strings.Contains(text, "more expensive"):
		return "more_expensive"
	default:
		return ""
	}
}

func normalizeAIText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(".", " ", ",", " ", "?", " ", "!", " ", "-", " ", "_", " ", ":", " ", ";", " ", "\n", " ", "'", "")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func tokenSetAI(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range strings.Fields(normalizeAIText(value)) {
		if len(token) >= 2 {
			out[singularAI(token)] = struct{}{}
		}
	}
	return out
}

func scoreText(query, candidate string) float64 {
	query = normalizeAIText(query)
	candidate = normalizeAIText(candidate)
	if query == "" || candidate == "" {
		return 0
	}
	if query == candidate {
		return 1
	}
	if strings.Contains(candidate, query) || strings.Contains(query, candidate) {
		return 0.82
	}
	queryTokens := tokenSetAI(query)
	candidateTokens := tokenSetAI(candidate)
	if len(queryTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}
	hits := 0
	for token := range queryTokens {
		if _, ok := candidateTokens[token]; ok {
			hits++
		} else if len(token) > 3 {
			for candidateToken := range candidateTokens {
				if strings.Contains(candidateToken, token) || strings.Contains(token, candidateToken) {
					hits++
					break
				}
			}
		}
	}
	return float64(hits) / float64(len(queryTokens))
}

func productNameReferenced(text, productName string) bool {
	textTokens := tokenSetAI(text)
	nameTokens := tokenSetAI(productName)
	if len(textTokens) == 0 || len(nameTokens) == 0 {
		return false
	}
	hits := 0
	for token := range nameTokens {
		if _, ok := textTokens[token]; ok {
			hits++
		}
	}
	return hits == len(nameTokens) || (len(nameTokens) > 1 && hits >= len(nameTokens)-1)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func singularAI(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 3 && strings.HasSuffix(value, "s") {
		return strings.TrimSuffix(value, "s")
	}
	return value
}

func hasToken(text, token string) bool {
	_, ok := tokenSetAI(text)[singularAI(token)]
	return ok
}

func anyVariantScore(variants []GroundedVariant) float64 {
	best := 0.0
	for _, variant := range variants {
		if variant.Score > best {
			best = variant.Score
		}
	}
	return best
}

func sortByLowestPrice(products []GroundedProduct) {
	sort.SliceStable(products, func(i, j int) bool {
		return lowestPrice(products[i]) < lowestPrice(products[j])
	})
}

func lowestPrice(product GroundedProduct) int64 {
	var best int64
	for _, variant := range product.Variants {
		if best == 0 || variant.PriceMinor < best {
			best = variant.PriceMinor
		}
	}
	return best
}

func limitProducts(products []GroundedProduct, limit int) []GroundedProduct {
	if len(products) <= limit {
		return products
	}
	return products[:limit]
}

func limitStores(stores []GroundedStore, limit int) []GroundedStore {
	if len(stores) <= limit {
		return stores
	}
	return stores[:limit]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func refersToPrevious(text string) bool {
	normalized := normalizeAIText(text)
	tokens := tokenSetAI(normalized)
	for _, token := range []string{"it", "that", "this", "same", "there"} {
		if _, ok := tokens[token]; ok {
			return true
		}
	}
	for _, phrase := range []string{"that one", "the one", "first one", "second one", "other one", "cheaper one", "black one", "blue one", "white one", "same one", "that item", "this item", "what about", "how many", "can i pick"} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func seemsProductSpecific(text string) bool {
	for _, term := range []string{"sell", "have", "price", "how much", "stock", "available", "buy", "order", "product", "item", "size", "shade", "color", "colour", "compare", "need", "want"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func isProductListing(text string) bool {
	for _, term := range []string{"what products", "what do you have", "what do you sell", "what are you selling", "menu", "show me", "list products", "products do you sell"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func isGenericStoreListing(text string) bool {
	for _, term := range []string{"where", "located", "location", "branches", "stores", "what store", "what branch", "which branch", "which branches"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
