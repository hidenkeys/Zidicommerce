package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

var templateNamePattern = regexp.MustCompile(`^[a-z0-9_]{1,128}$`)
var templateVariablePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
var templateLanguagePattern = regexp.MustCompile(`^[a-z]{2,3}(?:_[A-Z]{2})?$`)

func (s *Service) ListTemplates(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, status, category string) ([]MessageTemplateView, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsView); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID)
	if status = strings.ToLower(strings.TrimSpace(status)); status != "" {
		query = query.Where("status = ?", status)
	}
	if category = strings.ToLower(strings.TrimSpace(category)); category != "" {
		query = query.Where("category = ?", category)
	}
	var rows []MessageTemplate
	if err := query.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]MessageTemplateView, 0, len(rows))
	for _, row := range rows {
		views = append(views, templateView(row))
	}
	return views, nil
}

func (s *Service) GetTemplate(ctx context.Context, actor auth.CurrentUser, connectionID, templateID uuid.UUID) (MessageTemplateView, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsView); err != nil {
		return MessageTemplateView{}, err
	}
	row, err := s.findTemplate(ctx, actor.OrganizationID, connectionID, templateID)
	if err != nil {
		return MessageTemplateView{}, err
	}
	return templateView(row), nil
}

func (s *Service) CreateTemplate(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, input MessageTemplateInput) (MessageTemplateView, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsManage); err != nil {
		return MessageTemplateView{}, err
	}
	if err := validateTemplateInput(input); err != nil {
		return MessageTemplateView{}, err
	}
	now := s.now()
	row := MessageTemplate{
		ID: uuid.New(), OrganizationID: actor.OrganizationID, ConnectionID: connectionID,
		ProviderTemplateID: strings.TrimSpace(input.ProviderTemplateID), Name: normalizeTemplateName(input.Name),
		Language: normalizeTemplateLanguage(input.Language), Category: strings.ToLower(strings.TrimSpace(input.Category)),
		Status: strings.ToLower(strings.TrimSpace(input.Status)), Body: strings.TrimSpace(input.Body),
		HeaderMetadata: jsonValue(input.HeaderMetadata), FooterMetadata: jsonValue(input.FooterMetadata),
		ButtonsMetadata: jsonValue(input.ButtonsMetadata), VariableSchema: jsonValue(normalizeVariables(input.VariableSchema)),
		SampleValues: jsonValue(input.SampleValues), RejectionReason: strings.TrimSpace(input.RejectionReason),
		CreatedByUserID: &actor.ID, CreatedAt: now, UpdatedAt: now,
	}
	if row.Status == "" {
		row.Status = TemplateDraft
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_template_created", map[string]any{"template_id": row.ID, "name": row.Name, "status": row.Status, "category": row.Category})
	}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return MessageTemplateView{}, httperror.Conflict("A WhatsApp template with this name and language already exists")
		}
		return MessageTemplateView{}, err
	}
	return templateView(row), nil
}

func (s *Service) UpdateTemplate(ctx context.Context, actor auth.CurrentUser, connectionID, templateID uuid.UUID, input MessageTemplateUpdate) (MessageTemplateView, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsManage); err != nil {
		return MessageTemplateView{}, err
	}
	row, err := s.findTemplate(ctx, actor.OrganizationID, connectionID, templateID)
	if err != nil {
		return MessageTemplateView{}, err
	}
	if row.Status == TemplateArchived {
		return MessageTemplateView{}, httperror.BadRequest("Archived WhatsApp templates cannot be edited")
	}
	applyTemplateUpdate(&row, input)
	viewInput := inputFromTemplate(row)
	if err := validateTemplateInput(viewInput); err != nil {
		return MessageTemplateView{}, err
	}
	row.UpdatedAt = s.now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("organization_id = ? AND channel_connection_id = ? AND id = ?", actor.OrganizationID, connectionID, templateID).Save(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_template_updated", map[string]any{"template_id": row.ID, "name": row.Name, "status": row.Status, "category": row.Category})
	}); err != nil {
		return MessageTemplateView{}, err
	}
	return templateView(row), nil
}

func (s *Service) ArchiveTemplate(ctx context.Context, actor auth.CurrentUser, connectionID, templateID uuid.UUID) (MessageTemplateView, error) {
	status := TemplateArchived
	return s.updateTemplateStatus(ctx, actor, connectionID, templateID, status, "whatsapp_template_archived")
}

func (s *Service) PreviewTemplate(ctx context.Context, actor auth.CurrentUser, connectionID, templateID uuid.UUID, variables map[string]string) (TemplatePreview, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsView); err != nil {
		return TemplatePreview{}, err
	}
	row, err := s.findTemplate(ctx, actor.OrganizationID, connectionID, templateID)
	if err != nil {
		return TemplatePreview{}, err
	}
	return renderTemplate(row, variables), nil
}

func (s *Service) ListContactStates(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, consent string, limit int) ([]ContactState, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsView); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", actor.OrganizationID, connectionID)
	if consent = strings.ToLower(strings.TrimSpace(consent)); consent != "" {
		query = query.Where("consent_status = ?", consent)
	}
	var rows []ContactState
	if err := query.Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) UpdateContactConsent(ctx context.Context, actor auth.CurrentUser, connectionID, contactID uuid.UUID, input ContactStateInput) (ContactState, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsManage); err != nil {
		return ContactState{}, err
	}
	status := strings.ToLower(strings.TrimSpace(input.ConsentStatus))
	if status != ConsentOptedIn && status != ConsentOptedOut && status != ConsentUnknown {
		return ContactState{}, httperror.BadRequest("Consent status must be opted_in, opted_out, or unknown")
	}
	source := strings.ToLower(strings.TrimSpace(input.ConsentSource))
	if source == "" {
		source = ConsentSourceManual
	}
	if !validConsentSource(source) {
		return ContactState{}, httperror.BadRequest("Consent source is not supported")
	}
	var row ContactState
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND id = ?", actor.OrganizationID, connectionID, contactID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ContactState{}, httperror.NotFound("WhatsApp contact state not found")
		}
		return ContactState{}, err
	}
	now := s.now()
	row.ConsentStatus, row.ConsentSource, row.UpdatedAt = status, source, now
	if status == ConsentOptedIn {
		row.OptedInAt, row.OptedOutAt = &now, nil
	} else if status == ConsentOptedOut {
		row.OptedOutAt = &now
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND channel_connection_id = ? AND id = ?", actor.OrganizationID, connectionID, contactID).Save(&row).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, "whatsapp_contact_consent_updated", map[string]any{"contact_state_id": row.ID, "consent_status": status, "consent_source": source})
	}); err != nil {
		return ContactState{}, err
	}
	return row, nil
}

func (s *Service) requireWhatsAppConnection(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID, permission authz.Permission) error {
	if actor.OrganizationID == uuid.Nil || !actor.Role.HasPermission(permission) {
		return httperror.Forbidden("You do not have permission to manage WhatsApp messaging policy")
	}
	detail, err := s.platform.GetConnection(ctx, actor, connectionID)
	if err != nil {
		return err
	}
	if detail.Connection.Provider != Provider {
		return httperror.BadRequest("This connection is not a WhatsApp channel")
	}
	return nil
}

func (s *Service) findTemplate(ctx context.Context, organizationID, connectionID, templateID uuid.UUID) (MessageTemplate, error) {
	var row MessageTemplate
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND id = ?", organizationID, connectionID, templateID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MessageTemplate{}, httperror.NotFound("WhatsApp template not found")
		}
		return MessageTemplate{}, err
	}
	return row, nil
}

func (s *Service) updateTemplateStatus(ctx context.Context, actor auth.CurrentUser, connectionID, templateID uuid.UUID, status, action string) (MessageTemplateView, error) {
	if err := s.requireWhatsAppConnection(ctx, actor, connectionID, authz.PermissionChannelsManage); err != nil {
		return MessageTemplateView{}, err
	}
	row, err := s.findTemplate(ctx, actor.OrganizationID, connectionID, templateID)
	if err != nil {
		return MessageTemplateView{}, err
	}
	row.Status, row.UpdatedAt = status, s.now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&MessageTemplate{}).Where("organization_id = ? AND channel_connection_id = ? AND id = ?", actor.OrganizationID, connectionID, templateID).Updates(map[string]any{"status": status, "updated_at": row.UpdatedAt}).Error; err != nil {
			return err
		}
		return audit(tx, &actor.OrganizationID, &actor.ID, connectionID, action, map[string]any{"template_id": templateID, "status": status})
	}); err != nil {
		return MessageTemplateView{}, err
	}
	return templateView(row), nil
}

func validateTemplateInput(input MessageTemplateInput) error {
	name := normalizeTemplateName(input.Name)
	if !templateNamePattern.MatchString(name) {
		return httperror.BadRequest("Template name must use lowercase letters, numbers, and underscores")
	}
	if !templateLanguagePattern.MatchString(normalizeTemplateLanguage(input.Language)) {
		return httperror.BadRequest("Template language must be a Meta locale such as en or en_GB")
	}
	if !inSet(strings.ToLower(strings.TrimSpace(input.Category)), "utility", "authentication", "marketing", "service") {
		return httperror.BadRequest("Template category is not supported")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = TemplateDraft
	}
	if !inSet(status, TemplateDraft, TemplatePending, TemplateApproved, TemplateRejected, TemplatePaused, TemplateDisabled, TemplateArchived) {
		return httperror.BadRequest("Template status is not supported")
	}
	if body := strings.TrimSpace(input.Body); body == "" || len(body) > 4096 {
		return httperror.BadRequest("Template body is required and must be 4096 characters or fewer")
	}
	seen := map[string]bool{}
	for _, variable := range normalizeVariables(input.VariableSchema) {
		if !templateVariablePattern.MatchString(variable) || seen[variable] {
			return httperror.BadRequest("Template variables must be unique names beginning with a letter")
		}
		seen[variable] = true
	}
	return nil
}

func applyTemplateUpdate(row *MessageTemplate, input MessageTemplateUpdate) {
	if input.ProviderTemplateID != nil {
		row.ProviderTemplateID = strings.TrimSpace(*input.ProviderTemplateID)
	}
	if input.Name != nil {
		row.Name = normalizeTemplateName(*input.Name)
	}
	if input.Language != nil {
		row.Language = normalizeTemplateLanguage(*input.Language)
	}
	if input.Category != nil {
		row.Category = strings.ToLower(strings.TrimSpace(*input.Category))
	}
	if input.Status != nil {
		row.Status = strings.ToLower(strings.TrimSpace(*input.Status))
	}
	if input.Body != nil {
		row.Body = strings.TrimSpace(*input.Body)
	}
	if input.HeaderMetadata != nil {
		row.HeaderMetadata = jsonValue(*input.HeaderMetadata)
	}
	if input.FooterMetadata != nil {
		row.FooterMetadata = jsonValue(*input.FooterMetadata)
	}
	if input.ButtonsMetadata != nil {
		row.ButtonsMetadata = jsonValue(*input.ButtonsMetadata)
	}
	if input.VariableSchema != nil {
		row.VariableSchema = jsonValue(normalizeVariables(*input.VariableSchema))
	}
	if input.SampleValues != nil {
		row.SampleValues = jsonValue(*input.SampleValues)
	}
	if input.RejectionReason != nil {
		row.RejectionReason = strings.TrimSpace(*input.RejectionReason)
	}
}

func inputFromTemplate(row MessageTemplate) MessageTemplateInput {
	view := templateView(row)
	return MessageTemplateInput{ProviderTemplateID: row.ProviderTemplateID, Name: row.Name, Language: row.Language, Category: row.Category, Status: row.Status, Body: row.Body, HeaderMetadata: view.HeaderMetadata, FooterMetadata: view.FooterMetadata, ButtonsMetadata: view.ButtonsMetadata, VariableSchema: view.VariableSchema, SampleValues: view.SampleValues, RejectionReason: row.RejectionReason}
}

func templateView(row MessageTemplate) MessageTemplateView {
	view := MessageTemplateView{ID: row.ID, ConnectionID: row.ConnectionID, ProviderTemplateID: row.ProviderTemplateID, Name: row.Name, Language: row.Language, Category: row.Category, Status: row.Status, Body: row.Body, LastSyncedAt: row.LastSyncedAt, RejectionReason: row.RejectionReason, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	_ = json.Unmarshal([]byte(defaultJSON(row.HeaderMetadata, "{}")), &view.HeaderMetadata)
	_ = json.Unmarshal([]byte(defaultJSON(row.FooterMetadata, "{}")), &view.FooterMetadata)
	_ = json.Unmarshal([]byte(defaultJSON(row.ButtonsMetadata, "[]")), &view.ButtonsMetadata)
	_ = json.Unmarshal([]byte(defaultJSON(row.VariableSchema, "[]")), &view.VariableSchema)
	_ = json.Unmarshal([]byte(defaultJSON(row.SampleValues, "{}")), &view.SampleValues)
	if view.HeaderMetadata == nil {
		view.HeaderMetadata = map[string]any{}
	}
	if view.FooterMetadata == nil {
		view.FooterMetadata = map[string]any{}
	}
	if view.ButtonsMetadata == nil {
		view.ButtonsMetadata = []map[string]any{}
	}
	if view.VariableSchema == nil {
		view.VariableSchema = []string{}
	}
	if view.SampleValues == nil {
		view.SampleValues = map[string]string{}
	}
	return view
}

func renderTemplate(row MessageTemplate, variables map[string]string) TemplatePreview {
	view := templateView(row)
	values := variables
	if values == nil || len(values) == 0 {
		values = view.SampleValues
	}
	body := row.Body
	missing := []string{}
	for index, name := range view.VariableSchema {
		value := strings.TrimSpace(values[name])
		if value == "" {
			missing = append(missing, name)
			continue
		}
		body = strings.ReplaceAll(body, "{{"+name+"}}", value)
		body = strings.ReplaceAll(body, "{{"+strconv.Itoa(index+1)+"}}", value)
	}
	sort.Strings(missing)
	return TemplatePreview{Body: body, MissingVariables: missing, Valid: len(missing) == 0}
}

func templateVariableValues(row MessageTemplate, variables map[string]string) ([]string, []string) {
	view := templateView(row)
	values := make([]string, 0, len(view.VariableSchema))
	missing := []string{}
	for _, name := range view.VariableSchema {
		value := strings.TrimSpace(variables[name])
		if value == "" {
			missing = append(missing, name)
		} else {
			values = append(values, value)
		}
	}
	return values, missing
}

func normalizeTemplateName(value string) string     { return strings.ToLower(strings.TrimSpace(value)) }
func normalizeTemplateLanguage(value string) string { return strings.TrimSpace(value) }
func normalizeVariables(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
func defaultJSON(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func inSet(value string, values ...string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}
func validConsentSource(value string) bool {
	return inSet(value, ConsentSourceInbound, ConsentSourceManual, "import", "checkout", "support", "system")
}
