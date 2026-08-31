package ai

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
)

const (
	knowledgeSelectThreshold = 0.48
	knowledgeConflictScore   = 0.72
)

type KnowledgeMatchDebug struct {
	Source           string   `json:"source"`
	Title            string   `json:"title,omitempty"`
	Question         string   `json:"question,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	Category         string   `json:"category,omitempty"`
	InferredCategory string   `json:"inferred_category,omitempty"`
	Score            float64  `json:"score"`
	MatchedFields    []string `json:"matched_fields,omitempty"`
	Selected         bool     `json:"selected"`
	SkippedReason    string   `json:"skipped_reason,omitempty"`
}

type KnowledgeConflict struct {
	Category string   `json:"category"`
	Titles   []string `json:"titles"`
	Reason   string   `json:"reason"`
}

type knowledgeCandidate struct {
	id               uuid.UUID
	source           string
	title            string
	question         string
	answer           string
	kind             string
	category         string
	inferredCategory string
	score            float64
	matchedFields    []string
	updatedAt        time.Time
	knowledge        *bot.KnowledgeEntry
	faq              *bot.FAQ
	skippedReason    string
}

type vectorKnowledgeMatch struct {
	Score float64
}

func rankStructuredKnowledge(req KnowledgeRequest, queryParts []string, entries []bot.KnowledgeEntry, faqs []bot.FAQ) KnowledgeResult {
	return rankStructuredKnowledgeWithVector(req, queryParts, entries, faqs, nil, nil)
}

func rankStructuredKnowledgeWithVector(req KnowledgeRequest, queryParts []string, entries []bot.KnowledgeEntry, faqs []bot.FAQ, vectorScores map[uuid.UUID]vectorKnowledgeMatch, extraDebug []KnowledgeMatchDebug) KnowledgeResult {
	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	entriesByID := map[uuid.UUID]GroundedKnowledge{}
	matchesByID := map[uuid.UUID]GroundedFAQ{}
	debugByKey := map[string]KnowledgeMatchDebug{}
	conflicts := []KnowledgeConflict{}
	unknowns := []UnknownFact{}

	for _, part := range queryParts {
		route := routeKnowledgeQuery(part)
		candidates := scoreKnowledgeCandidates(part, route, entries, faqs, vectorScores)
		sortKnowledgeCandidates(candidates)
		conflict, hasConflict := detectKnowledgeConflict(candidates)
		if hasConflict {
			conflicts = append(conflicts, conflict)
			unknowns = append(unknowns, UnknownFact{Kind: "knowledge_conflict", Detail: "Conflicting active merchant knowledge matched: " + part})
			for i := range candidates {
				candidates[i].skippedReason = "conflict"
			}
			addKnowledgeDebug(debugByKey, candidates, maxResults)
			continue
		}

		bestScore := 0.0
		selectedForPart := 0
		for i := range candidates {
			candidate := &candidates[i]
			if candidate.score > bestScore {
				bestScore = candidate.score
			}
			if candidate.score < knowledgeSelectThreshold {
				if candidate.skippedReason == "" {
					candidate.skippedReason = "below_threshold"
				}
				continue
			}
			if selectedForPart >= maxResults {
				candidate.skippedReason = "outside_top_results"
				continue
			}
			selectedForPart++
			if candidate.knowledge != nil {
				grounded := groundedKnowledgeEntry(*candidate.knowledge, candidate.score, strings.Join(candidate.matchedFields, ","))
				grounded.Category = firstNonEmpty(candidate.category, grounded.Category)
				grounded.Kind = firstNonEmpty(candidate.kind, grounded.Kind)
				existing := entriesByID[candidate.id]
				if candidate.score > existing.Score {
					entriesByID[candidate.id] = grounded
					matchesByID[candidate.id] = faqFromKnowledgeEntry(grounded)
				}
				continue
			}
			if candidate.faq != nil {
				existing := matchesByID[candidate.id]
				if candidate.score > existing.Score {
					matchesByID[candidate.id] = GroundedFAQ{ID: candidate.id, Question: candidate.question, Answer: candidate.answer, Category: candidate.inferredCategory, Score: candidate.score, MatchedOn: strings.Join(candidate.matchedFields, ",")}
				}
			}
		}
		addKnowledgeDebug(debugByKey, candidates, maxResults)
		if bestScore < knowledgeSelectThreshold {
			kind := route.unknownKind
			if kind == "" {
				kind = "policy"
			}
			unknowns = append(unknowns, UnknownFact{Kind: kind, Detail: "No configured merchant knowledge matched: " + part})
		}
	}
	return KnowledgeResult{
		Matches:   knowledgeMatches(matchesByID, maxResults),
		Entries:   knowledgeEntryMatches(entriesByID, maxResults),
		Unknown:   unknowns,
		Debug:     append(sortedKnowledgeDebug(debugByKey), extraDebug...),
		Conflicts: conflicts,
	}
}

type knowledgeRoute struct {
	category            string
	unknownKind         string
	preferredKinds      map[string]struct{}
	preferredCategories map[string]struct{}
}

func routeKnowledgeQuery(query string) knowledgeRoute {
	category := classifyFAQCategory(query)
	route := knowledgeRoute{category: category, unknownKind: category, preferredKinds: map[string]struct{}{}, preferredCategories: map[string]struct{}{}}
	addPreferred := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				route.preferredKinds[value] = struct{}{}
				route.preferredCategories[value] = struct{}{}
			}
		}
	}
	switch category {
	case "delivery":
		addPreferred(bot.KnowledgeKindDelivery)
	case "returns":
		addPreferred(bot.KnowledgeKindReturns, bot.KnowledgeKindPolicy)
	case "warranty":
		addPreferred(bot.KnowledgeKindWarranty)
	case "opening_hours":
		route.unknownKind = "opening_hours"
		addPreferred(bot.KnowledgeKindLocation, bot.KnowledgeKindBusinessInfo, "opening_hours")
	case "locations":
		route.unknownKind = "location"
		addPreferred(bot.KnowledgeKindLocation, bot.KnowledgeKindBusinessInfo, "locations")
	case "payment":
		route.unknownKind = "payment"
		addPreferred(bot.KnowledgeKindPaymentInfo, "payment")
	default:
		if category != "" {
			addPreferred(category)
		}
	}
	return route
}

func scoreKnowledgeCandidates(query string, route knowledgeRoute, entries []bot.KnowledgeEntry, faqs []bot.FAQ, vectorScores map[uuid.UUID]vectorKnowledgeMatch) []knowledgeCandidate {
	candidates := make([]knowledgeCandidate, 0, len(entries)+len(faqs))
	for _, entry := range entries {
		candidate := scoreStructuredEntry(query, route, entry, vectorScores)
		if candidate.score > 0 {
			candidates = append(candidates, candidate)
		}
	}
	for _, faq := range faqs {
		candidate := scoreLegacyFAQEntry(query, route, faq)
		if candidate.score > 0 {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func scoreStructuredEntry(query string, route knowledgeRoute, entry bot.KnowledgeEntry, vectorScores map[uuid.UUID]vectorKnowledgeMatch) knowledgeCandidate {
	title := normalizeAIText(entry.Title)
	question := normalizeAIText(entry.Question)
	answer := normalizeAIText(entry.Answer)
	kind := normalizeAIText(entry.Kind)
	category := normalizeAIText(entry.Category)
	keywords := parseFAQKeywords(entry.Keywords)
	keywordText := normalizeAIText(strings.Join(keywords, " "))
	inferred := firstNonEmpty(category, classifyFAQCategory(entry.Kind+" "+entry.Category+" "+entry.Title+" "+entry.Question+" "+strings.Join(keywords, " ")))
	score, fields := scoreKnowledgeText(query, route, title, question, answer, kind, category, keywords)
	joined := strings.Join([]string{title, question, answer, kind, category, keywordText}, " ")
	blockedBySPF := strings.Contains(query, "spf") && !strings.Contains(joined, "spf")
	if blockedBySPF {
		score = 0
		fields = nil
	}
	if match, ok := vectorScores[entry.ID]; ok && !blockedBySPF && match.Score > score {
		score = match.Score
		fields = append(fields, embeddingMatchField(match.Score))
		sort.Strings(fields)
	}
	return knowledgeCandidate{id: entry.ID, source: "merchant_knowledge_entries", title: entry.Title, question: entry.Question, answer: entry.Answer, kind: firstNonEmpty(entry.Kind, bot.KnowledgeKindFAQ), category: firstNonEmpty(entry.Category, inferred, "general"), inferredCategory: firstNonEmpty(route.category, inferred), score: score, matchedFields: fields, updatedAt: entry.UpdatedAt, knowledge: &entry}
}

func scoreLegacyFAQEntry(query string, route knowledgeRoute, faq bot.FAQ) knowledgeCandidate {
	question := normalizeAIText(faq.Question)
	answer := normalizeAIText(faq.Answer)
	keywords := parseFAQKeywords(faq.Keywords)
	inferred := classifyFAQCategory(faq.Question + " " + strings.Join(keywords, " "))
	score, fields := scoreKnowledgeText(query, route, "", question, answer, bot.KnowledgeKindFAQ, inferred, keywords)
	joined := strings.Join([]string{question, answer, normalizeAIText(strings.Join(keywords, " "))}, " ")
	if strings.Contains(query, "spf") && !strings.Contains(joined, "spf") {
		score = 0
		fields = nil
	}
	return knowledgeCandidate{id: faq.ID, source: "bot_faqs", title: faq.Question, question: faq.Question, answer: faq.Answer, kind: bot.KnowledgeKindFAQ, category: inferred, inferredCategory: firstNonEmpty(route.category, inferred), score: score, matchedFields: fields, updatedAt: faq.UpdatedAt, faq: &faq}
}

func scoreKnowledgeText(query string, route knowledgeRoute, title, question, answer, kind, category string, keywords []string) (float64, []string) {
	fields := []string{}
	bestField := 0.0
	addField := func(field string, score float64) {
		if score <= 0 {
			return
		}
		if score > bestField {
			bestField = score
		}
		if !containsString(fields, field) {
			fields = append(fields, field)
		}
	}
	if question != "" && question == query {
		addField("question_exact", 1.0)
	}
	if title != "" && title == query {
		addField("title_exact", 0.98)
	}
	if question != "" && (strings.Contains(question, query) || strings.Contains(query, question)) {
		addField("question_phrase", 0.9)
	}
	if title != "" && (strings.Contains(title, query) || strings.Contains(query, title)) {
		addField("title_phrase", 0.86)
	}
	if score := scoreText(query, question) * 0.88; score > 0 {
		addField("question_terms", score)
	}
	if score := scoreText(query, title) * 0.82; score > 0 {
		addField("title_terms", score)
	}
	for _, keyword := range keywords {
		keyword = normalizeAIText(keyword)
		if keyword == "" {
			continue
		}
		score := scoreText(query, keyword) * 0.86
		if strings.Contains(query, keyword) || strings.Contains(keyword, query) {
			score = 0.93
		}
		addField("keyword", score)
	}
	if score := scoreText(query, answer) * 0.38; score > 0 {
		addField("answer_terms", score)
	}
	routeBoost := 0.0
	if route.category != "" {
		if _, ok := route.preferredKinds[kind]; ok {
			routeBoost += 0.2
			addField("kind_route", 0.2)
		}
		if _, ok := route.preferredCategories[category]; ok {
			routeBoost += 0.18
			addField("category_route", 0.18)
		}
		if route.category == category {
			routeBoost += 0.12
			addField("category_exact", 0.12)
		}
	}
	if bestField == 0 && routeBoost == 0 {
		return 0, nil
	}
	score := bestField + routeBoost
	if route.category != "" && !routeMatchesCandidate(route, kind, category) && !hasStrongField(fields) {
		score = minFloat(score, knowledgeSelectThreshold-0.01)
	}
	if score > 1.2 {
		score = 1.2
	}
	sort.Strings(fields)
	return score, fields
}

func routeMatchesCandidate(route knowledgeRoute, kind, category string) bool {
	if route.category == "" {
		return true
	}
	if route.category == category {
		return true
	}
	if _, ok := route.preferredKinds[kind]; ok {
		return true
	}
	if _, ok := route.preferredCategories[category]; ok {
		return true
	}
	return false
}

func hasStrongField(fields []string) bool {
	for _, field := range fields {
		switch field {
		case "question_exact", "title_exact", "question_phrase", "title_phrase", "keyword":
			return true
		}
	}
	return false
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func detectKnowledgeConflict(candidates []knowledgeCandidate) (KnowledgeConflict, bool) {
	strong := []knowledgeCandidate{}
	for _, candidate := range candidates {
		if candidate.score >= knowledgeConflictScore {
			strong = append(strong, candidate)
		}
	}
	if len(strong) < 2 {
		return KnowledgeConflict{}, false
	}
	sortKnowledgeCandidates(strong)
	top := strong[0]
	conflicting := []knowledgeCandidate{top}
	for _, candidate := range strong[1:] {
		if top.score-candidate.score > 0.18 {
			continue
		}
		if top.source != candidate.source {
			continue
		}
		if !sameKnowledgeRoute(top, candidate) {
			continue
		}
		if !knowledgeAnswersConflict(top.answer, candidate.answer) {
			continue
		}
		conflicting = append(conflicting, candidate)
	}
	if len(conflicting) < 2 {
		return KnowledgeConflict{}, false
	}
	titles := make([]string, 0, len(conflicting))
	for _, candidate := range conflicting {
		titles = append(titles, firstNonEmpty(candidate.title, candidate.question, "Untitled knowledge"))
	}
	sort.Strings(titles)
	return KnowledgeConflict{Category: firstNonEmpty(top.inferredCategory, top.category, top.kind, "knowledge"), Titles: titles, Reason: "multiple active entries strongly matched with different answers"}, true
}

func sameKnowledgeRoute(a, b knowledgeCandidate) bool {
	if a.inferredCategory != "" && b.inferredCategory != "" && a.inferredCategory == b.inferredCategory {
		return true
	}
	if a.category != "" && b.category != "" && a.category == b.category {
		return true
	}
	if a.kind != "" && b.kind != "" && a.kind == b.kind {
		return true
	}
	return false
}

func knowledgeAnswersConflict(a, b string) bool {
	aNorm := normalizeAIText(a)
	bNorm := normalizeAIText(b)
	if aNorm == "" || bNorm == "" || aNorm == bNorm {
		return false
	}
	aNumbers := numbersInText(aNorm)
	bNumbers := numbersInText(bNorm)
	if len(aNumbers) > 0 || len(bNumbers) > 0 {
		return !sameStringSet(aNumbers, bNumbers)
	}
	return scoreText(aNorm, bNorm) < 0.72
}

func numbersInText(text string) []string {
	values := []string{}
	for _, token := range strings.Fields(text) {
		trimmed := strings.Trim(token, ".,;:!?")
		hasDigit := false
		for _, r := range trimmed {
			if r >= '0' && r <= '9' {
				hasDigit = true
				break
			}
		}
		if hasDigit {
			values = append(values, trimmed)
		}
	}
	sort.Strings(values)
	return values
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortKnowledgeCandidates(candidates []knowledgeCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if !candidates[i].updatedAt.Equal(candidates[j].updatedAt) {
			return candidates[i].updatedAt.After(candidates[j].updatedAt)
		}
		return candidates[i].title < candidates[j].title
	})
}

func addKnowledgeDebug(debugByKey map[string]KnowledgeMatchDebug, candidates []knowledgeCandidate, maxResults int) {
	limit := maxResults * 2
	if limit < 8 {
		limit = 8
	}
	for i, candidate := range candidates {
		if i >= limit && candidate.skippedReason != "conflict" {
			break
		}
		selected := candidate.score >= knowledgeSelectThreshold && candidate.skippedReason == ""
		reason := candidate.skippedReason
		if !selected && reason == "" {
			reason = "below_threshold"
		}
		key := fmt.Sprintf("%s:%s", candidate.source, candidate.id.String())
		debugByKey[key] = KnowledgeMatchDebug{Source: candidate.source, Title: candidate.title, Question: candidate.question, Kind: candidate.kind, Category: candidate.category, InferredCategory: candidate.inferredCategory, Score: candidate.score, MatchedFields: candidate.matchedFields, Selected: selected, SkippedReason: reason}
	}
}

func sortedKnowledgeDebug(debugByKey map[string]KnowledgeMatchDebug) []KnowledgeMatchDebug {
	debug := make([]KnowledgeMatchDebug, 0, len(debugByKey))
	for _, item := range debugByKey {
		debug = append(debug, item)
	}
	sort.SliceStable(debug, func(i, j int) bool {
		if debug[i].Selected != debug[j].Selected {
			return debug[i].Selected
		}
		if debug[i].Score != debug[j].Score {
			return debug[i].Score > debug[j].Score
		}
		return debug[i].Title < debug[j].Title
	})
	return debug
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
