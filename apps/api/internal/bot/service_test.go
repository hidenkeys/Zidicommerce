package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type botFixture struct {
	db      *gorm.DB
	service *Service
	actor   auth.CurrentUser
}

type testWhatsAppConfiguration struct {
	OrganizationID     uuid.UUID `gorm:"type:uuid;index"`
	ConnectionID       uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index"`
	DisplayPhoneNumber string
}

func (testWhatsAppConfiguration) TableName() string { return "channel_whatsapp_configs" }

func newBotFixture(t *testing.T) botFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{},
		&organization.User{},
		&organization.OrganizationMembership{},
		&organization.AuditLog{},
		&core.Store{},
		&core.Product{},
		&core.Variant{},
		&core.InventoryLevel{},
		&core.Channel{},
		&core.PaymentConfiguration{},
		&core.PaymentProviderSecret{},
		&Bot{},
		&CommerceWorkflowConfiguration{},
		&BotVersion{},
		&VersionModule{},
		&Variable{},
		&Question{},
		&Action{},
		&Condition{},
		&Integration{},
		&Step{},
		&PublishedSnapshot{},
		&FAQ{},
		&KnowledgeEntry{},
		&DocumentSource{},
		&DocumentChunk{},
		&jobs.Job{},
	); err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: orgID, Role: authz.MerchantAdmin}
	if err := db.Create(&organization.Organization{ID: orgID, Name: "Test Merchant", Slug: "test-merchant", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	return botFixture{db: db, service: NewService(db), actor: actor}
}

func TestCreateBotCreatesDraftVersionAndStaysTenantScoped(t *testing.T) {
	fx := newBotFixture(t)
	bot, err := fx.service.CreateBot(context.Background(), fx.actor, BotInput{Name: "Bing Chun Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := fx.service.ListVersions(context.Background(), fx.actor, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].VersionNumber != 1 || versions[0].Status != VersionStatusDraft {
		t.Fatalf("expected one draft version, got %+v", versions)
	}
	otherActor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	if _, err := fx.service.GetBot(context.Background(), otherActor, bot.ID); err == nil {
		t.Fatal("expected cross-tenant bot lookup to fail")
	}
}

func TestSummarizeSetupStatusOnlyBlocksOnRequiredItems(t *testing.T) {
	items := []ChecklistItem{
		{Key: "business", Complete: true, Required: true},
		{Key: "catalogue", Complete: false, Required: true},
		{Key: "channel", Complete: false, Required: false},
		{Key: "team", Complete: true, Required: false},
	}
	complete, requiredComplete, required := summarizeSetupStatus(items)
	if complete != 2 || requiredComplete != 1 || required != 2 {
		t.Fatalf("unexpected readiness summary: complete=%d required_complete=%d required=%d", complete, requiredComplete, required)
	}
}

func TestSetupStatusIsTenantScopedAndRecognizesStructuredKnowledge(t *testing.T) {
	fx := newBotFixture(t)
	otherOrgID := uuid.New()
	if err := fx.db.Create(&organization.Organization{ID: otherOrgID, Name: "Other Merchant", Slug: "other", Currency: "NGN", Timezone: "Africa/Lagos", Country: "NG", Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&core.Store{ID: uuid.New(), OrganizationID: otherOrgID, Name: "Other Store", Code: "OTHER", Status: core.StatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&KnowledgeEntry{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Kind: "policy", Category: "returns", Title: "Returns", Answer: "Returns are accepted within seven days.", Status: core.StatusActive}).Error; err != nil {
		t.Fatal(err)
	}

	status, err := fx.service.GetSetupStatus(context.Background(), fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	items := make(map[string]ChecklistItem, len(status.Items))
	for _, item := range status.Items {
		items[item.Key] = item
	}
	if items["stores"].Complete {
		t.Fatal("another tenant's store must not satisfy readiness")
	}
	if !items["faqs"].Complete {
		t.Fatal("active structured knowledge should satisfy business knowledge readiness")
	}
	if items["organization"].Complete {
		t.Fatal("an incomplete business profile should not be marked ready")
	}
	if items["whatsapp"].Required || status.RequiredCount == 0 {
		t.Fatal("customer channel should be non-blocking")
	}

	staff := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.StoreStaff}
	if _, err := fx.service.GetSetupStatus(context.Background(), staff); err == nil {
		t.Fatal("store staff should not receive organization setup details")
	}
}

func TestPhaseGCommerceWorkflowConfigurationIsTenantScopedPermissionedAndAudited(t *testing.T) {
	fx := newBotFixture(t)
	ordering := true
	payment := false
	handoff := true
	updated, err := fx.service.UpdateCommerceWorkflowConfiguration(context.Background(), fx.actor, CommerceWorkflowConfigurationInput{
		BotDisplayName:           "Operations assistant",
		Greeting:                 "Welcome to the store.",
		Tone:                     "concise",
		OrderingEnabled:          &ordering,
		PaymentEnabled:           &payment,
		HumanHandoffEnabled:      &handoff,
		StoreSelectionStrategy:   StoreSelectionFirstAvailable,
		EnabledActions:           []string{"get_stores", "create_order", "handoff_to_agent"},
		SupportedFulfilmentModes: []string{"pickup", "merchant_rider"},
		PostPaymentSteps:         []string{"notify_customer", "merchant_prepares", "enable_tracking"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.OrganizationID != fx.actor.OrganizationID || updated.PaymentEnabled || updated.StoreSelectionStrategy != StoreSelectionFirstAvailable {
		t.Fatalf("unexpected workflow configuration: %+v", updated)
	}
	if !updated.AllowsAction("create_order") || updated.AllowsAction("initialize_payment") || updated.AllowsFulfilmentMode("customer_rider") {
		t.Fatalf("workflow enforcement does not match saved configuration: %+v", updated)
	}

	storeStaff := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.StoreStaff}
	if _, err := fx.service.UpdateCommerceWorkflowConfiguration(context.Background(), storeStaff, CommerceWorkflowConfigurationInput{}); err == nil {
		t.Fatal("expected store staff workflow update to be denied")
	}
	other := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	otherView, err := fx.service.GetCommerceWorkflowConfiguration(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if otherView.OrganizationID != other.OrganizationID || otherView.BotDisplayName == updated.BotDisplayName {
		t.Fatalf("expected another tenant to receive only its own default, got %+v", otherView)
	}
	var auditCount int64
	if err := fx.db.Model(&organization.AuditLog{}).Where("organization_id = ? AND action = ?", fx.actor.OrganizationID, "commerce_workflow_configuration_updated").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one workflow audit record, got %d", auditCount)
	}
}

func TestValidateAndPublishCreatesImmutableSnapshot(t *testing.T) {
	fx := newBotFixture(t)
	bot, version := createBotWithVersion(t, fx)
	nameVar, err := fx.service.CreateVariable(context.Background(), fx.actor, version.ID, VariableInput{Name: "customer_name", Type: "string", Scope: "user"})
	if err != nil {
		t.Fatal(err)
	}
	question, err := fx.service.CreateQuestion(context.Background(), fx.actor, version.ID, QuestionInput{QuestionKey: "ask_name", Text: "What is your name?", Type: "text", ResponseMode: "free_text", VariableName: nameVar.Name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "start", Type: StepMessage, Title: "Welcome", Message: "Welcome", NextStepKey: "ask_name"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "ask_name", Type: StepQuestion, Title: "Ask name", QuestionID: &question.ID, NextStepKey: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "done", Type: StepEnd, Title: "Done"}); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ValidateVersion(context.Background(), fx.actor, version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("expected valid config, got %+v", result.Issues)
	}
	snapshot, err := fx.service.PublishVersion(context.Background(), fx.actor, version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BotID != bot.ID || snapshot.VersionID != version.ID || snapshot.Snapshot == "" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "late_change", Type: StepMessage, Title: "Late change"}); err == nil {
		t.Fatal("expected published version to reject changes")
	}
}

func TestValidationReportsMissingStartStep(t *testing.T) {
	fx := newBotFixture(t)
	_, version := createBotWithVersion(t, fx)
	result, err := fx.service.ValidateVersion(context.Background(), fx.actor, version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || len(result.Issues) == 0 {
		t.Fatalf("expected validation issues, got %+v", result)
	}
}

func TestValidationReportsUnreachableAndCircularSteps(t *testing.T) {
	fx := newBotFixture(t)
	_, version := createBotWithVersion(t, fx)
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "start", Type: StepMessage, Title: "Start", NextStepKey: "loop_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "loop_a", Type: StepMessage, Title: "Loop A", NextStepKey: "loop_b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "loop_b", Type: StepMessage, Title: "Loop B", NextStepKey: "loop_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "unused", Type: StepMessage, Title: "Unused"}); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ValidateVersion(context.Background(), fx.actor, version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("expected graph validation issues, got %+v", result)
	}
	if !hasValidationMessage(result.Issues, "circular") || !hasValidationMessage(result.Issues, "not reachable") {
		t.Fatalf("expected circular and unreachable validation issues, got %+v", result.Issues)
	}
}

func TestStoreStaffCannotManageBots(t *testing.T) {
	fx := newBotFixture(t)
	staff := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.StoreStaff}
	if _, err := fx.service.CreateBot(context.Background(), staff, BotInput{Name: "Staff Bot"}); err == nil {
		t.Fatal("expected store staff to be blocked from bot creation")
	}
}

func TestIntegrationRejectsPlaintextSecrets(t *testing.T) {
	fx := newBotFixture(t)
	_, version := createBotWithVersion(t, fx)
	_, err := fx.service.CreateIntegration(context.Background(), fx.actor, version.ID, IntegrationInput{Provider: "paystack", DisplayName: "Paystack", Config: `{"secret_key":"sk_test"}`})
	if err == nil {
		t.Fatal("expected secret-like integration config to be rejected")
	}
}

func TestDraftCopyPreservesPublishedSnapshot(t *testing.T) {
	fx := newBotFixture(t)
	_, version := createBotWithVersion(t, fx)
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "start", Type: StepMessage, Title: "Start", Message: "Published message"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.PublishVersion(context.Background(), fx.actor, version.ID); err != nil {
		t.Fatal(err)
	}
	draft, err := fx.service.CreateVersion(context.Background(), fx.actor, version.BotID, VersionInput{SourceVersionID: &version.ID})
	if err != nil {
		t.Fatal(err)
	}
	if draft.StartStepKey != version.StartStepKey {
		t.Fatalf("expected copied start step %q, got %q", version.StartStepKey, draft.StartStepKey)
	}
	steps, err := fx.service.ListSteps(context.Background(), fx.actor, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Message != "Published message" {
		t.Fatalf("expected copied step, got %+v", steps)
	}
}

func TestCreateSelfServiceBotBuildsMerchantFriendlyDraft(t *testing.T) {
	fx := newBotFixture(t)
	bot, err := fx.service.CreateSelfServiceBot(context.Background(), fx.actor, SelfServiceBotInput{Name: "Bing Chun Bot"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := fx.service.ListVersions(context.Background(), fx.actor, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected one version, got %d", len(versions))
	}
	config, err := fx.service.GetConfiguration(context.Background(), fx.actor, versions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Modules) < 6 || len(config.Steps) < 8 {
		t.Fatalf("expected populated modules and steps, got modules=%d steps=%d", len(config.Modules), len(config.Steps))
	}
	foundSystemVariable := false
	for _, variable := range config.Variables {
		if variable.Scope == "system" && variable.Name == "system.customer.phone" {
			foundSystemVariable = true
		}
	}
	if !foundSystemVariable {
		t.Fatal("expected read-only system variables in template")
	}
	foundFAQQuestion := false
	foundFAQAction := false
	foundFAQStep := false
	for _, question := range config.Questions {
		if question.QuestionKey == "faq_query" && question.VariableName == "faq_query" {
			foundFAQQuestion = true
		}
	}
	for _, action := range config.Actions {
		if action.ActionKey == "answer_faq" && action.ActionType == "match_faq" {
			foundFAQAction = true
		}
	}
	for _, step := range config.Steps {
		if !json.Valid([]byte(step.Options)) {
			t.Fatalf("expected step %q options to contain valid JSON, got %q", step.StepKey, step.Options)
		}
		if step.StepKey == "answer_faq" && step.Type == StepAction {
			foundFAQStep = true
		}
	}
	if !foundFAQQuestion || !foundFAQAction || !foundFAQStep {
		t.Fatalf("expected scaffolded FAQ path to execute match_faq, question=%v action=%v step=%v", foundFAQQuestion, foundFAQAction, foundFAQStep)
	}
	result, err := fx.service.ValidateVersion(context.Background(), fx.actor, versions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("expected template to validate, got %+v", result.Issues)
	}
}

func TestModuleManagementUpdatesEnablementAndOrder(t *testing.T) {
	fx := newBotFixture(t)
	_, version := createBotWithVersion(t, fx)
	first, err := fx.service.AddModule(context.Background(), fx.actor, version.ID, ModuleInput{ModuleKey: "ORDER", SortOrder: 20, Metadata: `{"enabled":true}`})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.AddModule(context.Background(), fx.actor, version.ID, ModuleInput{ModuleKey: "FAQ", SortOrder: 10, Metadata: `{"enabled":true}`})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := fx.service.UpdateModule(context.Background(), fx.actor, first.ID, ModuleInput{Name: "Sales order", SortOrder: 30, Parameters: `{"entry_step":"done"}`})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Sales order" || updated.SortOrder != 30 || !strings.Contains(updated.Parameters, "entry_step") {
		t.Fatalf("expected module update, got %+v", updated)
	}
	disabled, err := fx.service.SetModuleEnabled(context.Background(), fx.actor, first.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if moduleEnabled(disabled.Metadata) {
		t.Fatalf("expected disabled module metadata, got %s", disabled.Metadata)
	}
	reordered, err := fx.service.ReorderModules(context.Background(), fx.actor, version.ID, ModuleReorderInput{Modules: []ModuleOrderInput{{ModuleID: first.ID, SortOrder: 10}, {ModuleID: second.ID, SortOrder: 20}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(reordered) != 2 || reordered[0].ID != first.ID || reordered[1].ID != second.ID {
		t.Fatalf("expected reordered modules, got %+v", reordered)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "start", Type: StepModule, Title: "Start module", ModuleID: &first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "done", Type: StepEnd, Title: "Done", Message: "Done."}); err != nil {
		t.Fatal(err)
	}
	result, err := fx.service.ValidateVersion(context.Background(), fx.actor, version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("expected disabled module reference to remain publishable for safe runtime handling, got %+v", result.Issues)
	}
}

func TestFAQMatchingIsDeterministicAndTenantScoped(t *testing.T) {
	fx := newBotFixture(t)
	if _, err := fx.service.CreateFAQ(context.Background(), fx.actor, FAQInput{Question: "What time do you open?", Answer: "We open by 10am.", Keywords: []string{"opening hours", "open"}}); err != nil {
		t.Fatal(err)
	}
	match, err := fx.service.MatchFAQ(context.Background(), fx.actor, "opening hours")
	if err != nil {
		t.Fatal(err)
	}
	if match.FAQ.Answer != "We open by 10am." || match.Confidence == "" {
		t.Fatalf("unexpected match: %+v", match)
	}
	otherActor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	if _, err := fx.service.MatchFAQ(context.Background(), otherActor, "opening hours"); err == nil {
		t.Fatal("expected cross-tenant FAQ lookup to miss")
	}
}

func TestKnowledgeEntriesCRUDIsTenantScopedAndAudited(t *testing.T) {
	fx := newBotFixture(t)
	entry, err := fx.service.CreateKnowledgeEntry(context.Background(), fx.actor, KnowledgeEntryInput{
		Kind:     KnowledgeKindReturns,
		Category: "Returns",
		Title:    "Returns policy",
		Question: "Can I return an item?",
		Answer:   "Returns are accepted within 7 days with a receipt.",
		Keywords: []string{"refunds", "returns"},
		Status:   "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.OrganizationID != fx.actor.OrganizationID || entry.Kind != KnowledgeKindReturns || entry.Category != "returns" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	entries, err := fx.service.ListKnowledgeEntries(context.Background(), fx.actor, KnowledgeEntryFilter{Kind: KnowledgeKindReturns, Search: "receipt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("expected filtered entry, got %+v", entries)
	}
	otherActor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	if _, err := fx.service.GetKnowledgeEntry(context.Background(), otherActor, entry.ID); err == nil {
		t.Fatal("expected cross-tenant knowledge lookup to fail")
	}
	updated, err := fx.service.UpdateKnowledgeEntry(context.Background(), fx.actor, entry.ID, KnowledgeEntryInput{Answer: "Returns are accepted within 14 days with a receipt.", Status: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Answer != "Returns are accepted within 14 days with a receipt." || updated.Status != "draft" {
		t.Fatalf("unexpected update: %+v", updated)
	}
	archived, err := fx.service.ArchiveKnowledgeEntry(context.Background(), fx.actor, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != "archived" {
		t.Fatalf("expected archived status, got %+v", archived)
	}
	var logs []organization.AuditLog
	if err := fx.db.Where("organization_id = ? AND target_type = ?", fx.actor.OrganizationID, "merchant_knowledge_entry").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected create/update/archive audit logs, got %d", len(logs))
	}
}

func TestKnowledgeEntriesPermissions(t *testing.T) {
	fx := newBotFixture(t)
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	if _, err := fx.service.ListKnowledgeEntries(context.Background(), viewer, KnowledgeEntryFilter{}); err != nil {
		t.Fatalf("viewer should read knowledge: %v", err)
	}
	if _, err := fx.service.CreateKnowledgeEntry(context.Background(), viewer, KnowledgeEntryInput{Kind: KnowledgeKindFAQ, Title: "FAQ", Answer: "Answer"}); err == nil {
		t.Fatal("expected viewer create knowledge to be forbidden")
	}
	support := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.SupportAgent}
	if _, err := fx.service.ListKnowledgeEntries(context.Background(), support, KnowledgeEntryFilter{}); err != nil {
		t.Fatalf("support agent should read knowledge: %v", err)
	}
	if _, err := fx.service.CreateKnowledgeEntry(context.Background(), support, KnowledgeEntryInput{Kind: KnowledgeKindFAQ, Title: "FAQ", Answer: "Answer"}); err == nil {
		t.Fatal("expected support create knowledge to be forbidden")
	}
}

func TestKnowledgeEntriesActiveFilteringSupportsAIRetrieval(t *testing.T) {
	fx := newBotFixture(t)
	active, err := fx.service.CreateKnowledgeEntry(context.Background(), fx.actor, KnowledgeEntryInput{Kind: KnowledgeKindWarranty, Title: "Warranty", Question: "Warranty policy?", Answer: "Warranty lasts 30 days.", Keywords: []string{"warranty"}, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.CreateKnowledgeEntry(context.Background(), fx.actor, KnowledgeEntryInput{Kind: KnowledgeKindWarranty, Title: "Old warranty", Question: "Old warranty policy?", Answer: "This draft should not ground answers.", Keywords: []string{"warranty"}, Status: "draft"}); err != nil {
		t.Fatal(err)
	}
	entries, err := fx.service.ListKnowledgeEntries(context.Background(), fx.actor, KnowledgeEntryFilter{Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != active.ID {
		t.Fatalf("expected only active entry, got %+v", entries)
	}
}

func TestKnowledgeEmbeddingLifecycleIsOptInAndActiveOnly(t *testing.T) {
	fx := newBotFixture(t)
	jobService := jobs.NewService(fx.db, nil)
	fx.service.ConfigureJobs(jobService)

	disabled, err := fx.service.CreateKnowledgeEntry(context.Background(), fx.actor, KnowledgeEntryInput{
		Kind:     KnowledgeKindWarranty,
		Category: "warranty",
		Title:    "Disabled warranty",
		Question: "What is your warranty?",
		Answer:   "Warranty lasts 30 days.",
		Status:   core.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.EmbeddingStatus != KnowledgeEmbeddingStatusDisabled {
		t.Fatalf("expected disabled embedding status before opt-in, got %+v", disabled)
	}
	assertEmbeddingJobCount(t, fx, disabled.ID, 0)

	fx.service.ConfigureKnowledgeEmbeddings(true)
	active, err := fx.service.CreateKnowledgeEntry(context.Background(), fx.actor, KnowledgeEntryInput{
		Kind:     KnowledgeKindReturns,
		Category: "returns",
		Title:    "Returns",
		Question: "Can I return an order?",
		Answer:   "Returns are accepted within 7 days with receipt proof.",
		Keywords: []string{"returns", "receipt"},
		Status:   core.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.EmbeddingStatus != KnowledgeEmbeddingStatusPending || active.EmbeddingContentHash == "" || active.Embedding != nil {
		t.Fatalf("expected active knowledge to be pending embedding, got %+v", active)
	}
	assertEmbeddingJobCount(t, fx, active.ID, 1)

	draft, err := fx.service.CreateKnowledgeEntry(context.Background(), fx.actor, KnowledgeEntryInput{
		Kind:   KnowledgeKindPolicy,
		Title:  "Draft policy",
		Answer: "Draft policy should not be embedded.",
		Status: "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.EmbeddingStatus != KnowledgeEmbeddingStatusDisabled {
		t.Fatalf("expected draft knowledge to keep embeddings disabled, got %+v", draft)
	}
	assertEmbeddingJobCount(t, fx, draft.ID, 0)

	updated, err := fx.service.UpdateKnowledgeEntry(context.Background(), fx.actor, active.ID, KnowledgeEntryInput{
		Answer: "Returns are accepted within 14 days with receipt proof.",
		Status: core.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.EmbeddingStatus != KnowledgeEmbeddingStatusPending || updated.EmbeddingContentHash == active.EmbeddingContentHash {
		t.Fatalf("expected content update to mark embedding stale, before=%s after=%+v", active.EmbeddingContentHash, updated)
	}
	assertEmbeddingJobCount(t, fx, updated.ID, 2)

	archived, err := fx.service.ArchiveKnowledgeEntry(context.Background(), fx.actor, updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.EmbeddingStatus != KnowledgeEmbeddingStatusDisabled || archived.Embedding != nil {
		t.Fatalf("expected archive to disable embedding, got %+v", archived)
	}
}

func TestShareLinkRequiresPublishedBotAndUsableWhatsAppChannel(t *testing.T) {
	fx := newBotFixture(t)
	bot, version := createBotWithVersion(t, fx)
	link, err := fx.service.GetShareLink(context.Background(), fx.actor, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if link.Available || link.Reason == "" {
		t.Fatalf("expected unavailable link before publish, got %+v", link)
	}
	if _, err := fx.service.CreateStep(context.Background(), fx.actor, version.ID, StepInput{StepKey: "start", Type: StepMessage, Title: "Start", Message: "Hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.PublishVersion(context.Background(), fx.actor, version.ID); err != nil {
		t.Fatal(err)
	}
	channel := core.Channel{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Provider: "whatsapp", DisplayName: "WhatsApp", PhoneNumberID: "phone-id", Status: "healthy", Config: "{}", SecretConfig: `{"token":"hidden"}`}
	if err := fx.db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if err := fx.db.AutoMigrate(&testWhatsAppConfiguration{}); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.Create(&testWhatsAppConfiguration{OrganizationID: fx.actor.OrganizationID, ConnectionID: channel.ID, DisplayPhoneNumber: "+2348012345678"}).Error; err != nil {
		t.Fatal(err)
	}
	link, err = fx.service.GetShareLink(context.Background(), fx.actor, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !link.Available || link.URL == "" || link.DisplayNumber != "2348012345678" {
		t.Fatalf("expected available wa.me link, got %+v", link)
	}
}

func assertEmbeddingJobCount(t *testing.T, fx botFixture, entryID uuid.UUID, want int64) {
	t.Helper()
	var count int64
	if err := fx.db.Model(&jobs.Job{}).
		Where("job_type = ? AND correlation_id = ?", jobs.JobTypeKnowledgeEmbedding, entryID.String()).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("expected %d embedding jobs for %s, got %d", want, entryID, count)
	}
}

func hasValidationMessage(issues []ValidationIssue, fragment string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, fragment) {
			return true
		}
	}
	return false
}

func createBotWithVersion(t *testing.T, fx botFixture) (Bot, BotVersion) {
	t.Helper()
	bot, err := fx.service.CreateBot(context.Background(), fx.actor, BotInput{Name: "Bing Chun Assistant"})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := fx.service.ListVersions(context.Background(), fx.actor, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected one version, got %d", len(versions))
	}
	return bot, versions[0]
}
