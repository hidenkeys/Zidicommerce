package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"gorm.io/gorm"
)

type EmbeddingProvider interface {
	Name() string
	Model() string
	Embed(ctx context.Context, input string) ([]float32, error)
}

type EmbeddingOptions struct {
	Enabled               bool
	VectorSearchEnabled   bool
	Model                 string
	Dimensions            int
	VectorSearchThreshold float64
}

type disabledEmbeddingProvider struct{}

func (disabledEmbeddingProvider) Name() string  { return "disabled" }
func (disabledEmbeddingProvider) Model() string { return "" }
func (disabledEmbeddingProvider) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embeddings are disabled")
}

type HashEmbeddingProvider struct {
	model      string
	dimensions int
}

func NewHashEmbeddingProvider(model string, dimensions int) HashEmbeddingProvider {
	if dimensions <= 0 {
		dimensions = 1536
	}
	if strings.TrimSpace(model) == "" {
		model = "local_hash"
	}
	return HashEmbeddingProvider{model: model, dimensions: dimensions}
}

func (p HashEmbeddingProvider) Name() string  { return "local_hash" }
func (p HashEmbeddingProvider) Model() string { return p.model }
func (p HashEmbeddingProvider) Embed(_ context.Context, input string) ([]float32, error) {
	vector := make([]float32, p.dimensions)
	for _, token := range strings.Fields(normalizeAIText(input)) {
		sum := sha256.Sum256([]byte(token))
		idx := int(sum[0]) % p.dimensions
		weight := float32(1)
		if len(token) > 8 {
			weight = 1.5
		}
		vector[idx] += weight
	}
	return vector, nil
}

type knowledgeEmbeddingJobPayload struct {
	KnowledgeEntryID string `json:"knowledge_entry_id"`
}

func normalizeEmbeddingOptions(options EmbeddingOptions) EmbeddingOptions {
	if options.Dimensions <= 0 {
		options.Dimensions = 1536
	}
	if options.VectorSearchThreshold <= 0 || options.VectorSearchThreshold > 1 {
		options.VectorSearchThreshold = 0.72
	}
	return options
}

func (s *Service) ConfigureEmbeddings(provider EmbeddingProvider, options EmbeddingOptions) {
	options = normalizeEmbeddingOptions(options)
	if provider == nil {
		provider = disabledEmbeddingProvider{}
	}
	s.embeddingProvider = provider
	s.embeddingOptions = options
}

func (s *Service) ProcessKnowledgeEmbeddingJob(ctx context.Context, job jobs.Job) error {
	var payload knowledgeEmbeddingJobPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return jobs.PermanentError{Err: err}
	}
	entryID, err := uuid.Parse(payload.KnowledgeEntryID)
	if err != nil {
		return jobs.PermanentError{Err: err}
	}
	if job.OrganizationID != nil {
		return s.RefreshKnowledgeEmbeddingForOrganization(ctx, *job.OrganizationID, entryID)
	}
	return s.RefreshKnowledgeEmbedding(ctx, entryID)
}

func (s *Service) RefreshKnowledgeEmbedding(ctx context.Context, entryID uuid.UUID) error {
	return s.refreshKnowledgeEmbedding(ctx, uuid.Nil, entryID)
}

func (s *Service) RefreshKnowledgeEmbeddingForOrganization(ctx context.Context, organizationID uuid.UUID, entryID uuid.UUID) error {
	return s.refreshKnowledgeEmbedding(ctx, organizationID, entryID)
}

func (s *Service) refreshKnowledgeEmbedding(ctx context.Context, organizationID uuid.UUID, entryID uuid.UUID) error {
	query := s.db.WithContext(ctx).Model(&bot.KnowledgeEntry{}).Where("id = ?", entryID)
	if organizationID != uuid.Nil {
		query = query.Where("organization_id = ?", organizationID)
	}
	if !s.embeddingOptions.Enabled || s.embeddingProvider == nil {
		return query.Updates(map[string]any{"embedding_status": bot.KnowledgeEmbeddingStatusDisabled, "embedding_error": "", "embedded_at": nil, "embedding": nil}).Error
	}
	var entry bot.KnowledgeEntry
	find := s.db.WithContext(ctx).Where("id = ?", entryID)
	if organizationID != uuid.Nil {
		find = find.Where("organization_id = ?", organizationID)
	}
	if err := find.First(&entry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return jobs.PermanentError{Err: err}
		}
		return err
	}
	if entry.Status != "active" {
		return s.db.WithContext(ctx).Model(&entry).Updates(map[string]any{"embedding_status": bot.KnowledgeEmbeddingStatusDisabled, "embedding_error": "", "embedded_at": nil, "embedding": nil}).Error
	}
	sourceText := bot.KnowledgeEmbeddingSourceText(entry)
	hash := bot.KnowledgeEmbeddingHash(entry)
	vector, err := s.embeddingProvider.Embed(ctx, sourceText)
	if err != nil {
		return s.db.WithContext(ctx).Model(&entry).Updates(map[string]any{
			"embedding_status":       bot.KnowledgeEmbeddingStatusFailed,
			"embedding_error":        truncateEmbeddingError(err),
			"embedding_content_hash": hash,
			"updated_at":             time.Now().UTC(),
		}).Error
	}
	if len(vector) == 0 {
		err := errors.New("embedding provider returned an empty vector")
		return s.db.WithContext(ctx).Model(&entry).Updates(map[string]any{
			"embedding_status":       bot.KnowledgeEmbeddingStatusFailed,
			"embedding_error":        truncateEmbeddingError(err),
			"embedding_content_hash": hash,
			"updated_at":             time.Now().UTC(),
		}).Error
	}
	now := time.Now().UTC()
	serialized := serializeEmbeddingVector(vector)
	return s.db.WithContext(ctx).Model(&entry).Updates(map[string]any{
		"embedding":              serialized,
		"embedding_status":       bot.KnowledgeEmbeddingStatusReady,
		"embedding_model":        firstNonEmpty(s.embeddingOptions.Model, s.embeddingProvider.Model()),
		"embedding_content_hash": hash,
		"embedding_error":        "",
		"embedded_at":            &now,
		"updated_at":             now,
	}).Error
}

func (s *Service) RefreshStaleKnowledgeEmbeddings(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 25
	}
	var entries []bot.KnowledgeEntry
	if err := s.db.WithContext(ctx).
		Where("status = ?", "active").
		Order("updated_at ASC").
		Limit(limit * 3).
		Find(&entries).Error; err != nil {
		return 0, err
	}
	processed := 0
	for _, entry := range entries {
		if processed >= limit {
			break
		}
		if entry.EmbeddingStatus == bot.KnowledgeEmbeddingStatusReady && entry.EmbeddingContentHash == bot.KnowledgeEmbeddingHash(entry) {
			continue
		}
		if err := s.RefreshKnowledgeEmbedding(ctx, entry.ID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func serializeEmbeddingVector(vector []float32) string {
	parts := make([]string, 0, len(vector))
	for _, value := range vector {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func parseEmbeddingVector(raw string) ([]float32, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	if raw == "" {
		return nil, false
	}
	parts := strings.Split(raw, ",")
	vector := make([]float32, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, false
		}
		vector = append(vector, float32(value))
	}
	return vector, len(vector) > 0
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func truncateEmbeddingError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

func embeddingVectorLiteral(vector []float32) string {
	return serializeEmbeddingVector(vector)
}

func embeddingDebug(reason string) KnowledgeMatchDebug {
	return KnowledgeMatchDebug{Source: "merchant_knowledge_embeddings", Score: 0, Selected: false, SkippedReason: reason}
}

func embeddingMatchField(score float64) string {
	return fmt.Sprintf("vector:%.3f", score)
}
