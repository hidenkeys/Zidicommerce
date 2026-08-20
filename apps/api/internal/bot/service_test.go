package bot

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
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
