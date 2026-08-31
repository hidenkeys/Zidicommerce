package ai

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
)

type hybridKnowledgeRetriever struct {
	db       *gorm.DB
	lexical  KnowledgeRetriever
	provider EmbeddingProvider
	options  EmbeddingOptions
}

func (s *Service) knowledgeRetriever() KnowledgeRetriever {
	lexical := lexicalFAQRetriever{db: s.db}
	options := normalizeEmbeddingOptions(s.embeddingOptions)
	if !options.Enabled || !options.VectorSearchEnabled || s.embeddingProvider == nil {
		return lexical
	}
	return hybridKnowledgeRetriever{db: s.db, lexical: lexical, provider: s.embeddingProvider, options: options}
}

func (r hybridKnowledgeRetriever) Retrieve(ctx context.Context, req KnowledgeRequest) (KnowledgeResult, error) {
	queryParts := splitKnowledgeQuery(req.Query)
	if len(queryParts) == 0 {
		return KnowledgeResult{}, nil
	}
	vector, err := r.provider.Embed(ctx, req.Query)
	if err != nil {
		return r.lexicalFallback(ctx, req, embeddingDebug("vector_query_failed"))
	}
	vectorScores, debug, err := r.vectorScores(ctx, req.OrganizationID, vector, req.MaxResults)
	if err != nil {
		return r.lexicalFallback(ctx, req, embeddingDebug("vector_retrieval_failed"))
	}
	var entries []bot.KnowledgeEntry
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND status = ?", req.OrganizationID, core.StatusActive).Find(&entries).Error; err != nil {
		return KnowledgeResult{}, err
	}
	var faqs []bot.FAQ
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND status = ?", req.OrganizationID, core.StatusActive).Find(&faqs).Error; err != nil {
		return KnowledgeResult{}, err
	}
	return rankStructuredKnowledgeWithVector(req, queryParts, entries, faqs, vectorScores, debug), nil
}

func (r hybridKnowledgeRetriever) lexicalFallback(ctx context.Context, req KnowledgeRequest, debug KnowledgeMatchDebug) (KnowledgeResult, error) {
	result, err := r.lexical.Retrieve(ctx, req)
	if err != nil {
		return result, err
	}
	result.Debug = append(result.Debug, debug)
	return result, nil
}

func (r hybridKnowledgeRetriever) vectorScores(ctx context.Context, organizationID uuid.UUID, queryVector []float32, maxResults int) (map[uuid.UUID]vectorKnowledgeMatch, []KnowledgeMatchDebug, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	if r.db.Dialector.Name() == "postgres" {
		return r.postgresVectorScores(ctx, organizationID, queryVector, maxResults)
	}
	return r.localVectorScores(ctx, organizationID, queryVector, maxResults)
}

type vectorKnowledgeRow struct {
	bot.KnowledgeEntry
	VectorScore float64 `gorm:"column:vector_score"`
}

func (r hybridKnowledgeRetriever) postgresVectorScores(ctx context.Context, organizationID uuid.UUID, queryVector []float32, maxResults int) (map[uuid.UUID]vectorKnowledgeMatch, []KnowledgeMatchDebug, error) {
	literal := embeddingVectorLiteral(queryVector)
	var rows []vectorKnowledgeRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT *, 1 - (embedding <=> ?::vector) AS vector_score
		FROM merchant_knowledge_entries
		WHERE organization_id = ?
		  AND status = ?
		  AND embedding_status = ?
		  AND embedding IS NOT NULL
		ORDER BY embedding <=> ?::vector
		LIMIT ?
	`, literal, organizationID, core.StatusActive, bot.KnowledgeEmbeddingStatusReady, literal, maxResults).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	return vectorRowsToScores(rows, r.options.VectorSearchThreshold), vectorDebugRows(rows, r.options.VectorSearchThreshold), nil
}

func (r hybridKnowledgeRetriever) localVectorScores(ctx context.Context, organizationID uuid.UUID, queryVector []float32, maxResults int) (map[uuid.UUID]vectorKnowledgeMatch, []KnowledgeMatchDebug, error) {
	var entries []bot.KnowledgeEntry
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND status = ? AND embedding_status = ? AND embedding IS NOT NULL", organizationID, core.StatusActive, bot.KnowledgeEmbeddingStatusReady).
		Find(&entries).Error; err != nil {
		return nil, nil, err
	}
	rows := make([]vectorKnowledgeRow, 0, len(entries))
	debug := make([]KnowledgeMatchDebug, 0, len(entries))
	for _, entry := range entries {
		if entry.Embedding == nil {
			continue
		}
		vector, ok := parseEmbeddingVector(*entry.Embedding)
		if !ok {
			debug = append(debug, KnowledgeMatchDebug{Source: "merchant_knowledge_embeddings", Title: entry.Title, Score: 0, Selected: false, SkippedReason: "invalid_vector"})
			continue
		}
		score := cosineSimilarity(queryVector, vector)
		rows = append(rows, vectorKnowledgeRow{KnowledgeEntry: entry, VectorScore: score})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].VectorScore > rows[j].VectorScore })
	if len(rows) > maxResults {
		rows = rows[:maxResults]
	}
	scores := vectorRowsToScores(rows, r.options.VectorSearchThreshold)
	debug = append(debug, vectorDebugRows(rows, r.options.VectorSearchThreshold)...)
	return scores, debug, nil
}

func vectorRowsToScores(rows []vectorKnowledgeRow, threshold float64) map[uuid.UUID]vectorKnowledgeMatch {
	scores := map[uuid.UUID]vectorKnowledgeMatch{}
	for _, row := range rows {
		if row.VectorScore < threshold {
			continue
		}
		scores[row.ID] = vectorKnowledgeMatch{Score: normalizeVectorScore(row.VectorScore)}
	}
	return scores
}

func normalizeVectorScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1.2 {
		return 1.2
	}
	// Put strong semantic matches above the lexical select threshold while still
	// letting exact lexical matches win.
	return 0.5 + (score * 0.45)
}

func vectorDebugRows(rows []vectorKnowledgeRow, threshold float64) []KnowledgeMatchDebug {
	debug := make([]KnowledgeMatchDebug, 0, len(rows))
	for _, row := range rows {
		selected := row.VectorScore >= threshold
		reason := ""
		if !selected {
			reason = "vector_below_threshold"
		}
		debug = append(debug, KnowledgeMatchDebug{
			Source:        "merchant_knowledge_embeddings",
			Title:         row.Title,
			Question:      row.Question,
			Kind:          row.Kind,
			Category:      row.Category,
			Score:         row.VectorScore,
			MatchedFields: []string{embeddingMatchField(row.VectorScore)},
			Selected:      selected,
			SkippedReason: reason,
		})
	}
	return debug
}

func (r hybridKnowledgeRetriever) String() string {
	return fmt.Sprintf("hybridKnowledgeRetriever(vector=%t)", r.options.VectorSearchEnabled)
}
