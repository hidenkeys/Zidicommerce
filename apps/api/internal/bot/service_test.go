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
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type botFixture struct {
	db      *gorm.DB
	service *Service
	actor   auth.CurrentUser
}

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

func TestShareLinkRequiresPublishedBotAndActiveWhatsAppChannel(t *testing.T) {
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
	channel := core.Channel{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Provider: "whatsapp", DisplayName: "WhatsApp", PhoneNumberID: "phone-id", DisplayNumber: "+2348012345678", Status: core.StatusActive, Config: "{}", SecretConfig: `{"token":"hidden"}`}
	if err := fx.db.Create(&channel).Error; err != nil {
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
