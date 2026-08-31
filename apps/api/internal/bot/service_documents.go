package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

const documentChunkMaxLength = 1200

type documentChunkDraft struct {
	Title   string
	Heading string
	Content string
}

func (s *Service) ListDocumentSources(ctx context.Context, actor auth.CurrentUser, filter DocumentSourceFilter) ([]DocumentSource, error) {
	if !canViewKnowledge(actor.Role) {
		return nil, httperror.Forbidden("You cannot view merchant documents")
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID)
	if status := strings.ToLower(strings.TrimSpace(filter.Status)); status != "" {
		if !validDocumentSourceStatus(status) {
			return nil, httperror.BadRequest("Document source status is not valid")
		}
		query = query.Where("status = ?", status)
	}
	if sourceType := strings.ToLower(strings.TrimSpace(filter.SourceType)); sourceType != "" {
		if !validDocumentSourceType(sourceType) {
			return nil, httperror.BadRequest("Document source type is not valid")
		}
		query = query.Where("source_type = ?", sourceType)
	}
	if search := normalizeSearchText(filter.Search); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(source_label) LIKE ? OR LOWER(original_filename) LIKE ? OR LOWER(source_url) LIKE ?", pattern, pattern, pattern, pattern)
	}
	var sources []DocumentSource
	err := query.Order("updated_at DESC").Find(&sources).Error
	if err == nil {
		err = s.attachLinkedKnowledgeCounts(ctx, actor.OrganizationID, sources)
	}
	return sources, err
}

func (s *Service) GetDocumentSource(ctx context.Context, actor auth.CurrentUser, sourceID uuid.UUID) (DocumentSource, error) {
	if !canViewKnowledge(actor.Role) {
		return DocumentSource{}, httperror.Forbidden("You cannot view merchant documents")
	}
	var source DocumentSource
	err := s.db.WithContext(ctx).
		Preload("Chunks", func(db *gorm.DB) *gorm.DB { return db.Order("chunk_index ASC") }).
		Where("organization_id = ? AND id = ?", actor.OrganizationID, sourceID).
		First(&source).Error
	if err != nil {
		return DocumentSource{}, mapNotFound(err, "Document source not found")
	}
	sources := []DocumentSource{source}
	if err := s.attachLinkedKnowledgeCounts(ctx, actor.OrganizationID, sources); err != nil {
		return DocumentSource{}, err
	}
	source.LinkedKnowledgeCount = sources[0].LinkedKnowledgeCount
	source.ActiveLinkedKnowledgeCount = sources[0].ActiveLinkedKnowledgeCount
	return source, nil
}

func (s *Service) CreateDocumentSource(ctx context.Context, actor auth.CurrentUser, input DocumentSourceInput) (DocumentSource, error) {
	if !canManageKnowledge(actor.Role) {
		return DocumentSource{}, httperror.Forbidden("You cannot manage merchant documents")
	}
	source, drafts, err := documentSourceFromInput(actor, input, true)
	if err != nil {
		return DocumentSource{}, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		chunks := documentChunksFromDrafts(actor.OrganizationID, source.ID, drafts)
		if len(chunks) > 0 {
			if err := tx.Create(&chunks).Error; err != nil {
				return err
			}
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_document_source", source.ID, "document_source_created", documentSourceAuditMetadata(source, len(chunks)))
	})
	if err != nil {
		return DocumentSource{}, err
	}
	return s.GetDocumentSource(ctx, actor, source.ID)
}

func (s *Service) UpdateDocumentSource(ctx context.Context, actor auth.CurrentUser, sourceID uuid.UUID, input DocumentSourceInput) (DocumentSource, error) {
	if !canManageKnowledge(actor.Role) {
		return DocumentSource{}, httperror.Forbidden("You cannot manage merchant documents")
	}
	var source DocumentSource
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, sourceID).First(&source).Error; err != nil {
		return DocumentSource{}, mapNotFound(err, "Document source not found")
	}
	updates, drafts, err := documentSourceUpdates(source.Title, input)
	if err != nil {
		return DocumentSource{}, err
	}
	if len(updates) == 0 && drafts == nil {
		return s.GetDocumentSource(ctx, actor, sourceID)
	}
	updates["updated_at"] = s.now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&source).Updates(updates).Error; err != nil {
				return err
			}
		}
		if drafts != nil {
			if err := tx.Where("organization_id = ? AND document_source_id = ?", actor.OrganizationID, source.ID).Delete(&DocumentChunk{}).Error; err != nil {
				return err
			}
			chunks := documentChunksFromDrafts(actor.OrganizationID, source.ID, drafts)
			if len(chunks) > 0 {
				if err := tx.Create(&chunks).Error; err != nil {
					return err
				}
			}
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_document_source", source.ID, "document_source_updated", jsonValue(map[string]any{"fields": sortedMapKeys(updates), "chunks_refreshed": drafts != nil}))
	})
	if err != nil {
		return DocumentSource{}, err
	}
	return s.GetDocumentSource(ctx, actor, sourceID)
}

func (s *Service) ArchiveDocumentSource(ctx context.Context, actor auth.CurrentUser, sourceID uuid.UUID, options DocumentSourceArchiveOptions) (DocumentSource, error) {
	if !canManageKnowledge(actor.Role) {
		return DocumentSource{}, httperror.Forbidden("You cannot manage merchant documents")
	}
	var source DocumentSource
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, sourceID).First(&source).Error; err != nil {
		return DocumentSource{}, mapNotFound(err, "Document source not found")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now()
		linkedIDs, err := linkedKnowledgeIDsForSource(tx, actor.OrganizationID, source.ID)
		if err != nil {
			return err
		}
		if err := tx.Model(&source).Updates(map[string]any{"status": DocumentStatusArchived, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&DocumentChunk{}).Where("organization_id = ? AND document_source_id = ?", actor.OrganizationID, source.ID).Updates(map[string]any{"status": DocumentChunkStatusArchived, "updated_at": now}).Error; err != nil {
			return err
		}
		archivedLinkedCount := 0
		if options.ArchiveLinkedKnowledge && len(linkedIDs) > 0 {
			result := tx.Model(&KnowledgeEntry{}).
				Where("organization_id = ? AND id IN ? AND status <> ?", actor.OrganizationID, linkedIDs, "archived").
				Updates(map[string]any{"status": "archived", "embedding_status": KnowledgeEmbeddingStatusDisabled, "embedding": nil, "embedding_error": "", "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			archivedLinkedCount = int(result.RowsAffected)
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_document_source", source.ID, "document_source_archived", documentSourceArchiveAuditMetadata(source, len(linkedIDs), archivedLinkedCount, options.ArchiveLinkedKnowledge))
	})
	if err != nil {
		return DocumentSource{}, err
	}
	return s.GetDocumentSource(ctx, actor, sourceID)
}

func (s *Service) ListDocumentChunks(ctx context.Context, actor auth.CurrentUser, sourceID uuid.UUID) ([]DocumentChunk, error) {
	if !canViewKnowledge(actor.Role) {
		return nil, httperror.Forbidden("You cannot view merchant documents")
	}
	var source DocumentSource
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, sourceID).First(&source).Error; err != nil {
		return nil, mapNotFound(err, "Document source not found")
	}
	var chunks []DocumentChunk
	err := s.db.WithContext(ctx).Where("organization_id = ? AND document_source_id = ?", actor.OrganizationID, sourceID).Order("chunk_index ASC").Find(&chunks).Error
	return chunks, err
}

func (s *Service) UpdateDocumentChunk(ctx context.Context, actor auth.CurrentUser, chunkID uuid.UUID, input DocumentChunkInput) (DocumentChunk, error) {
	if !canManageKnowledge(actor.Role) {
		return DocumentChunk{}, httperror.Forbidden("You cannot manage merchant documents")
	}
	var chunk DocumentChunk
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunkID).First(&chunk).Error; err != nil {
		return DocumentChunk{}, mapNotFound(err, "Document chunk not found")
	}
	updates, err := documentChunkUpdates(input)
	if err != nil {
		return DocumentChunk{}, err
	}
	if len(updates) == 0 {
		return chunk, nil
	}
	updates["updated_at"] = s.now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&chunk).Updates(updates).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_document_chunk", chunk.ID, "document_chunk_updated", jsonValue(map[string]any{"fields": sortedMapKeys(updates)}))
	})
	if err != nil {
		return DocumentChunk{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunkID).First(&chunk).Error; err != nil {
		return DocumentChunk{}, err
	}
	return chunk, nil
}

func (s *Service) ApproveDocumentChunk(ctx context.Context, actor auth.CurrentUser, chunkID uuid.UUID, input DocumentChunkApprovalInput) (DocumentChunk, error) {
	if !canManageKnowledge(actor.Role) {
		return DocumentChunk{}, httperror.Forbidden("You cannot manage merchant documents")
	}
	var chunk DocumentChunk
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunkID).First(&chunk).Error; err != nil {
		return DocumentChunk{}, mapNotFound(err, "Document chunk not found")
	}
	if chunk.Status == DocumentChunkStatusArchived {
		return DocumentChunk{}, httperror.BadRequest("Archived document chunks cannot be approved")
	}
	if strings.TrimSpace(chunk.Content) == "" {
		return DocumentChunk{}, httperror.BadRequest("Document chunk content is required")
	}
	var source DocumentSource
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunk.DocumentSourceID).First(&source).Error; err != nil {
		return DocumentChunk{}, mapNotFound(err, "Document source not found")
	}
	approvedAt := s.now()
	entryInput := knowledgeInputFromChunk(chunk, source, actor, approvedAt, input)
	entry, err := knowledgeEntryFromInput(actor.OrganizationID, entryInput, true)
	if err != nil {
		return DocumentChunk{}, err
	}
	s.applyKnowledgeEmbeddingLifecycle(&entry)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := approvedAt
		knowledgeAction := "knowledge_entry_created_from_document"
		if chunk.KnowledgeEntryID != nil {
			var existing KnowledgeEntry
			if err := tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, *chunk.KnowledgeEntryID).First(&existing).Error; err != nil {
				return mapNotFound(err, "Knowledge entry not found")
			}
			updates, err := knowledgeEntryUpdates(entryInput)
			if err != nil {
				return err
			}
			applyKnowledgeEmbeddingUpdates(s.embeddingsEnabled, existing, updates)
			updates["updated_at"] = now
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
			entry.ID = existing.ID
			applyKnowledgeEntryMap(&entry, updates)
			knowledgeAction = "knowledge_entry_updated_from_document"
		} else {
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			chunk.KnowledgeEntryID = &entry.ID
		}
		if err := s.enqueueKnowledgeEmbeddingTx(ctx, tx, actor.OrganizationID, entry); err != nil {
			return err
		}
		if err := tx.Model(&chunk).Updates(map[string]any{"status": DocumentChunkStatusApproved, "knowledge_entry_id": entry.ID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := auditTx(tx, actor.OrganizationID, actor.ID, "merchant_document_chunk", chunk.ID, "document_chunk_approved", jsonValue(map[string]any{"knowledge_entry_id": entry.ID.String()})); err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_knowledge_entry", entry.ID, knowledgeAction, documentKnowledgeAuditMetadata(entry, source, chunk))
	})
	if err != nil {
		return DocumentChunk{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunkID).First(&chunk).Error; err != nil {
		return DocumentChunk{}, err
	}
	return chunk, nil
}

func (s *Service) ArchiveDocumentChunk(ctx context.Context, actor auth.CurrentUser, chunkID uuid.UUID) (DocumentChunk, error) {
	if !canManageKnowledge(actor.Role) {
		return DocumentChunk{}, httperror.Forbidden("You cannot manage merchant documents")
	}
	var chunk DocumentChunk
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunkID).First(&chunk).Error; err != nil {
		return DocumentChunk{}, mapNotFound(err, "Document chunk not found")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&chunk).Updates(map[string]any{"status": DocumentChunkStatusArchived, "updated_at": s.now()}).Error; err != nil {
			return err
		}
		return auditTx(tx, actor.OrganizationID, actor.ID, "merchant_document_chunk", chunk.ID, "document_chunk_archived", "{}")
	})
	if err != nil {
		return DocumentChunk{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, chunkID).First(&chunk).Error; err != nil {
		return DocumentChunk{}, err
	}
	return chunk, nil
}

func documentSourceFromInput(actor auth.CurrentUser, input DocumentSourceInput, requireText bool) (DocumentSource, []documentChunkDraft, error) {
	sourceType := strings.ToLower(strings.TrimSpace(input.SourceType))
	if sourceType == "" {
		sourceType = DocumentSourcePastedText
	}
	if sourceType != DocumentSourcePastedText && sourceType != DocumentSourceManual {
		return DocumentSource{}, nil, httperror.BadRequest("Only pasted text and manual document sources are supported in this phase")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return DocumentSource{}, nil, httperror.BadRequest("Document title is required")
	}
	rawText := strings.TrimSpace(input.RawText)
	if requireText && rawText == "" {
		return DocumentSource{}, nil, httperror.BadRequest("Document text is required")
	}
	drafts := chunkDocumentText(title, rawText, documentChunkMaxLength)
	if requireText && len(drafts) == 0 {
		return DocumentSource{}, nil, httperror.BadRequest("Document text did not contain reviewable content")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" || status == DocumentStatusDraft || status == DocumentStatusExtracted {
		status = DocumentStatusReviewRequired
	}
	if !validDocumentSourceStatus(status) {
		return DocumentSource{}, nil, httperror.BadRequest("Document source status is not valid")
	}
	return DocumentSource{
		ID:               uuid.New(),
		OrganizationID:   actor.OrganizationID,
		Title:            title,
		SourceType:       sourceType,
		Status:           status,
		OriginalFilename: strings.TrimSpace(input.OriginalFilename),
		SourceURL:        strings.TrimSpace(input.SourceURL),
		SourceLabel:      strings.TrimSpace(input.SourceLabel),
		MimeType:         defaultString(strings.TrimSpace(input.MimeType), "text/plain"),
		StorageKey:       strings.TrimSpace(input.StorageKey),
		RawText:          rawText,
		ErrorMessage:     strings.TrimSpace(input.ErrorMessage),
		CreatedBy:        &actor.ID,
		Metadata:         jsonObject(input.Metadata),
	}, drafts, nil
}

func documentSourceUpdates(currentTitle string, input DocumentSourceInput) (map[string]any, []documentChunkDraft, error) {
	updates := map[string]any{}
	if strings.TrimSpace(input.Title) != "" {
		updates["title"] = strings.TrimSpace(input.Title)
	}
	if strings.TrimSpace(input.SourceType) != "" {
		sourceType := strings.ToLower(strings.TrimSpace(input.SourceType))
		if sourceType != DocumentSourcePastedText && sourceType != DocumentSourceManual {
			return nil, nil, httperror.BadRequest("Only pasted text and manual document sources are supported in this phase")
		}
		updates["source_type"] = sourceType
	}
	if strings.TrimSpace(input.Status) != "" {
		status := strings.ToLower(strings.TrimSpace(input.Status))
		if !validDocumentSourceStatus(status) {
			return nil, nil, httperror.BadRequest("Document source status is not valid")
		}
		updates["status"] = status
	}
	if strings.TrimSpace(input.OriginalFilename) != "" {
		updates["original_filename"] = strings.TrimSpace(input.OriginalFilename)
	}
	if strings.TrimSpace(input.SourceURL) != "" {
		updates["source_url"] = strings.TrimSpace(input.SourceURL)
	}
	if strings.TrimSpace(input.SourceLabel) != "" {
		updates["source_label"] = strings.TrimSpace(input.SourceLabel)
	}
	if strings.TrimSpace(input.MimeType) != "" {
		updates["mime_type"] = strings.TrimSpace(input.MimeType)
	}
	if strings.TrimSpace(input.StorageKey) != "" {
		updates["storage_key"] = strings.TrimSpace(input.StorageKey)
	}
	if input.ErrorMessage != "" {
		updates["error_message"] = strings.TrimSpace(input.ErrorMessage)
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	var drafts []documentChunkDraft
	if strings.TrimSpace(input.RawText) != "" {
		rawText := strings.TrimSpace(input.RawText)
		drafts = chunkDocumentText(firstNonEmptyString(strings.TrimSpace(input.Title), currentTitle, "Document"), rawText, documentChunkMaxLength)
		if len(drafts) == 0 {
			return nil, nil, httperror.BadRequest("Document text did not contain reviewable content")
		}
		updates["raw_text"] = rawText
		updates["status"] = DocumentStatusReviewRequired
	}
	return updates, drafts, nil
}

func documentChunkUpdates(input DocumentChunkInput) (map[string]any, error) {
	updates := map[string]any{}
	if strings.TrimSpace(input.Title) != "" {
		updates["title"] = strings.TrimSpace(input.Title)
	}
	if strings.TrimSpace(input.Heading) != "" {
		updates["heading"] = strings.TrimSpace(input.Heading)
	}
	if strings.TrimSpace(input.Content) != "" {
		updates["content"] = strings.TrimSpace(input.Content)
	}
	if strings.TrimSpace(input.Status) != "" {
		status := strings.ToLower(strings.TrimSpace(input.Status))
		if !validDocumentChunkStatus(status) {
			return nil, httperror.BadRequest("Document chunk status is not valid")
		}
		updates["status"] = status
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	return updates, nil
}

func knowledgeInputFromChunk(chunk DocumentChunk, source DocumentSource, actor auth.CurrentUser, approvedAt time.Time, input DocumentChunkApprovalInput) KnowledgeEntryInput {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = firstNonEmptyString(chunk.Title, chunk.Heading, fmt.Sprintf("Document chunk %d", chunk.ChunkIndex+1))
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind == "" {
		kind = inferKnowledgeKindFromDocument(title + " " + chunk.Heading + " " + chunk.Content)
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = core.StatusActive
	}
	metadata := documentApprovalMetadata(source, chunk, actor.ID, approvedAt, input.Metadata)
	return KnowledgeEntryInput{
		Kind:       kind,
		Category:   firstNonEmptyString(input.Category, kind),
		Title:      title,
		Question:   strings.TrimSpace(input.Question),
		Answer:     chunk.Content,
		Keywords:   cleanKeywords(input.Keywords),
		SourceType: "document_chunk",
		Status:     status,
		Metadata:   metadata,
	}
}

func documentChunksFromDrafts(organizationID, sourceID uuid.UUID, drafts []documentChunkDraft) []DocumentChunk {
	chunks := make([]DocumentChunk, 0, len(drafts))
	for idx, draft := range drafts {
		chunks = append(chunks, DocumentChunk{
			ID:               uuid.New(),
			OrganizationID:   organizationID,
			DocumentSourceID: sourceID,
			ChunkIndex:       idx,
			Title:            draft.Title,
			Heading:          draft.Heading,
			Content:          draft.Content,
			Status:           DocumentChunkStatusReviewRequired,
			Metadata:         "{}",
		})
	}
	return chunks
}

func chunkDocumentText(sourceTitle, rawText string, maxLength int) []documentChunkDraft {
	rawText = strings.ReplaceAll(rawText, "\r\n", "\n")
	rawText = strings.ReplaceAll(rawText, "\r", "\n")
	blocks := splitDocumentBlocks(rawText)
	var chunks []documentChunkDraft
	currentHeading := ""
	var buffer []string
	flush := func() {
		content := strings.TrimSpace(strings.Join(buffer, "\n\n"))
		buffer = nil
		if content == "" {
			return
		}
		heading := strings.TrimSpace(currentHeading)
		title := firstNonEmptyString(heading, sourceTitle)
		for _, part := range splitLongDocumentBlock(content, maxLength) {
			chunks = append(chunks, documentChunkDraft{Title: title, Heading: heading, Content: part})
		}
	}
	for _, block := range blocks {
		if isDocumentHeading(block) {
			flush()
			currentHeading = normalizeDocumentHeading(block)
			continue
		}
		next := strings.TrimSpace(strings.Join(append(append([]string{}, buffer...), block), "\n\n"))
		if buffer != nil && utf8.RuneCountInString(next) > maxLength {
			flush()
		}
		buffer = append(buffer, block)
	}
	flush()
	return chunks
}

func splitDocumentBlocks(rawText string) []string {
	lines := strings.Split(rawText, "\n")
	blocks := make([]string, 0, len(lines))
	var current []string
	flush := func() {
		block := strings.TrimSpace(strings.Join(current, "\n"))
		current = nil
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		if isDocumentHeading(line) {
			flush()
			blocks = append(blocks, line)
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func splitLongDocumentBlock(block string, maxLength int) []string {
	if utf8.RuneCountInString(block) <= maxLength {
		return []string{block}
	}
	words := strings.Fields(block)
	var parts []string
	var current []string
	for _, word := range words {
		candidate := strings.Join(append(append([]string{}, current...), word), " ")
		if len(current) > 0 && utf8.RuneCountInString(candidate) > maxLength {
			parts = append(parts, strings.Join(current, " "))
			current = []string{word}
			continue
		}
		current = append(current, word)
	}
	if len(current) > 0 {
		parts = append(parts, strings.Join(current, " "))
	}
	return parts
}

func isDocumentHeading(block string) bool {
	block = strings.TrimSpace(block)
	if block == "" || strings.Contains(block, "\n") || utf8.RuneCountInString(block) > 90 {
		return false
	}
	return strings.HasPrefix(block, "#") || strings.HasSuffix(block, ":")
}

func normalizeDocumentHeading(block string) string {
	return strings.Trim(strings.TrimSpace(block), "#: ")
}

func validDocumentSourceType(sourceType string) bool {
	switch sourceType {
	case DocumentSourceUpload, DocumentSourceURL, DocumentSourcePastedText, DocumentSourceManual:
		return true
	default:
		return false
	}
}

func validDocumentSourceStatus(status string) bool {
	switch status {
	case DocumentStatusDraft, DocumentStatusProcessing, DocumentStatusExtracted, DocumentStatusReviewRequired, DocumentStatusActive, DocumentStatusArchived, DocumentStatusFailed:
		return true
	default:
		return false
	}
}

func validDocumentChunkStatus(status string) bool {
	switch status {
	case DocumentChunkStatusDraft, DocumentChunkStatusReviewRequired, DocumentChunkStatusApproved, DocumentChunkStatusArchived:
		return true
	default:
		return false
	}
}

func inferKnowledgeKindFromDocument(text string) string {
	normalized := normalizeSearchText(text)
	switch {
	case strings.Contains(normalized, "deliver") || strings.Contains(normalized, "shipping"):
		return KnowledgeKindDelivery
	case strings.Contains(normalized, "return") || strings.Contains(normalized, "refund") || strings.Contains(normalized, "exchange"):
		return KnowledgeKindReturns
	case strings.Contains(normalized, "warranty") || strings.Contains(normalized, "guarantee"):
		return KnowledgeKindWarranty
	case strings.Contains(normalized, "payment") || strings.Contains(normalized, "pay with") || strings.Contains(normalized, "bank transfer"):
		return KnowledgeKindPaymentInfo
	case strings.Contains(normalized, "location") || strings.Contains(normalized, "address") || strings.Contains(normalized, "opening hour"):
		return KnowledgeKindLocation
	default:
		return KnowledgeKindPolicy
	}
}

func documentSourceAuditMetadata(source DocumentSource, chunkCount int) string {
	return jsonValue(map[string]any{"source_type": source.SourceType, "status": source.Status, "chunk_count": chunkCount})
}

func documentSourceArchiveAuditMetadata(source DocumentSource, linkedKnowledgeCount, archivedLinkedKnowledgeCount int, archiveLinkedKnowledge bool) string {
	return jsonValue(map[string]any{
		"source_type":                     source.SourceType,
		"status":                          DocumentStatusArchived,
		"linked_knowledge_count":          linkedKnowledgeCount,
		"archived_linked_knowledge_count": archivedLinkedKnowledgeCount,
		"archive_linked_knowledge":        archiveLinkedKnowledge,
	})
}

func documentKnowledgeAuditMetadata(entry KnowledgeEntry, source DocumentSource, chunk DocumentChunk) string {
	return jsonValue(map[string]any{
		"kind":               entry.Kind,
		"category":           entry.Category,
		"status":             entry.Status,
		"document_source_id": chunk.DocumentSourceID.String(),
		"document_chunk_id":  chunk.ID.String(),
		"source_title":       source.Title,
		"source_label":       source.SourceLabel,
	})
}

func documentApprovalMetadata(source DocumentSource, chunk DocumentChunk, approvedBy uuid.UUID, approvedAt time.Time, raw string) string {
	metadata := metadataMap(raw)
	metadata["document_source_id"] = source.ID.String()
	metadata["document_chunk_id"] = chunk.ID.String()
	metadata["document_source_title"] = source.Title
	metadata["document_source_label"] = source.SourceLabel
	metadata["document_chunk_title"] = chunk.Title
	metadata["document_chunk_index"] = chunk.ChunkIndex
	metadata["approved_at"] = approvedAt.Format(time.RFC3339Nano)
	metadata["approved_by"] = approvedBy.String()
	return jsonValue(metadata)
}

func metadataMap(raw string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out
	}
	return out
}

func linkedKnowledgeIDsForSource(tx *gorm.DB, organizationID, sourceID uuid.UUID) ([]uuid.UUID, error) {
	var chunks []DocumentChunk
	if err := tx.Where("organization_id = ? AND document_source_id = ? AND knowledge_entry_id IS NOT NULL", organizationID, sourceID).Find(&chunks).Error; err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]struct{}{}
	ids := make([]uuid.UUID, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.KnowledgeEntryID == nil {
			continue
		}
		if _, ok := seen[*chunk.KnowledgeEntryID]; ok {
			continue
		}
		seen[*chunk.KnowledgeEntryID] = struct{}{}
		ids = append(ids, *chunk.KnowledgeEntryID)
	}
	return ids, nil
}

func (s *Service) attachLinkedKnowledgeCounts(ctx context.Context, organizationID uuid.UUID, sources []DocumentSource) error {
	if len(sources) == 0 {
		return nil
	}
	sourceIDs := make([]uuid.UUID, 0, len(sources))
	sourceIndex := map[uuid.UUID]int{}
	for idx, source := range sources {
		sourceIDs = append(sourceIDs, source.ID)
		sourceIndex[source.ID] = idx
	}
	var chunks []DocumentChunk
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND document_source_id IN ? AND knowledge_entry_id IS NOT NULL", organizationID, sourceIDs).
		Find(&chunks).Error; err != nil {
		return err
	}
	entryToSource := map[uuid.UUID]uuid.UUID{}
	entryIDs := make([]uuid.UUID, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.KnowledgeEntryID == nil {
			continue
		}
		entryID := *chunk.KnowledgeEntryID
		if _, exists := entryToSource[entryID]; exists {
			continue
		}
		entryToSource[entryID] = chunk.DocumentSourceID
		entryIDs = append(entryIDs, entryID)
	}
	if len(entryIDs) == 0 {
		return nil
	}
	var entries []KnowledgeEntry
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id IN ?", organizationID, entryIDs).Find(&entries).Error; err != nil {
		return err
	}
	for _, entry := range entries {
		sourceID, ok := entryToSource[entry.ID]
		if !ok {
			continue
		}
		idx, ok := sourceIndex[sourceID]
		if !ok {
			continue
		}
		sources[idx].LinkedKnowledgeCount++
		if entry.Status == core.StatusActive {
			sources[idx].ActiveLinkedKnowledgeCount++
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
