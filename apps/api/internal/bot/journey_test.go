package bot

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"testing"
)

func TestJourneyCanonicalMappingValidationAcceptsPostgresJSONButRejectsTampering(t *testing.T) {
	fx := newBotFixture(t)
	if err := fx.db.AutoMigrate(&JourneyBinding{}, &JourneyFlowDeployment{}); err != nil {
		t.Fatal(err)
	}
	detail, err := fx.service.CreateJourney(context.Background(), fx.actor, "order", "Mapping validation")
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.service.ensureEditable(context.Background(), fx.actor, detail.Version.ID); err == nil {
		t.Fatal("generic graph edits could bypass the journey version lock")
	}
	config, err := fx.service.loadConfiguration(context.Background(), fx.actor.OrganizationID, detail.Version)
	if err != nil {
		t.Fatal(err)
	}
	for i := range config.Actions {
		var inputs any
		if err := json.Unmarshal([]byte(config.Actions[i].InputMappings), &inputs); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(inputs, "", "  ")
		config.Actions[i].InputMappings = string(raw)
	}
	if result := validateConfiguration(config); !result.Valid {
		t.Fatalf("JSON formatting must not block publication: %+v", result.Issues)
	}
	original := config.Actions[0].OutputMappings
	config.Actions[0].OutputMappings = `{"order_id":"variables.payment_status"}`
	if validateConfiguration(config).Valid {
		t.Fatal("altered output mappings accepted")
	}
	config.Actions[0].OutputMappings = original
	for i := range config.Steps {
		if config.Steps[i].ActionID != nil {
			config.Steps[i].ActionID = nil
			break
		}
	}
	if validateConfiguration(config).Valid {
		t.Fatal("removed mandatory action accepted")
	}
}

func TestJourneyTemplatesAndCompiler(t *testing.T) {
	for _, template := range JourneyTemplates() {
		t.Run(template["id"], func(t *testing.T) {
			d, _ := JourneyTemplate(template["id"])
			if v := ValidateJourney(d); !v.Valid {
				t.Fatalf("invalid template: %+v", v.Issues)
			}
			c := CompileJourneyFlow(d)
			if template["id"] == "entry_menu" {
				if len(c.Errors) == 0 {
					t.Fatal("cross-journey menu must explicitly use guided chat")
				}
				return
			}
			if len(c.Errors) > 0 {
				t.Fatalf("flow: %+v", c.Errors)
			}
			c2 := CompileJourneyFlow(d)
			if jsonValue(c.JSON) != jsonValue(c2.JSON) {
				t.Fatal("unstable compiler")
			}
			routes := c.JSON["routing_model"].(map[string][]string)
			visiting, done := map[string]bool{}, map[string]bool{}
			var visit func(string)
			visit = func(id string) {
				if visiting[id] {
					t.Fatalf("cyclic provider routing model: %s", id)
				}
				if done[id] {
					return
				}
				visiting[id] = true
				for _, target := range routes[id] {
					if target == id {
						t.Fatal("provider self route")
					}
					visit(target)
				}
				visiting[id], done[id] = false, true
			}
			for id := range routes {
				visit(id)
			}
		})
	}
}

func TestJourneyCRUDScopeImmutableAndRollback(t *testing.T) {
	fx := newBotFixture(t)
	ctx := context.Background()
	if err := fx.db.AutoMigrate(&JourneyBinding{}, &JourneyFlowDeployment{}); err != nil {
		t.Fatal(err)
	}
	d, _ := JourneyTemplate("order")
	detail, err := fx.service.CreateJourney(ctx, fx.actor, "order", d.Name)
	if err != nil {
		t.Fatal(err)
	}
	other := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	if _, err := fx.service.GetJourney(ctx, other, detail.Bot.ID, nil); err == nil {
		t.Fatal("cross tenant read")
	}
	viewer := fx.actor
	viewer.Role = authz.Viewer
	if _, err := fx.service.SaveJourney(ctx, viewer, detail.Version.ID, d); err == nil {
		t.Fatal("viewer edit")
	}
	published, err := fx.service.PublishJourney(ctx, fx.actor, detail.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.SaveJourney(ctx, fx.actor, published.Version.ID, d); err == nil {
		t.Fatal("published edit")
	}
	clone, err := fx.service.CloneJourneyDraft(ctx, fx.actor, detail.Bot.ID, published.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if clone.Version.Status != VersionStatusDraft || len(clone.Definition.Steps) != len(d.Steps) {
		t.Fatal("bad clone")
	}
	if _, err := fx.service.ActivateJourney(ctx, fx.actor, detail.Bot.ID, published.Version.ID, "guided_chat", true); err != nil {
		t.Fatal(err)
	}
}

func TestJourneySafetyValidation(t *testing.T) {
	d, _ := JourneyTemplate("order")
	for i := range d.Steps {
		if d.Steps[i].Kind == "check_inventory" {
			d.Steps[i].Kind = "message"
		}
	}
	if ValidateJourney(d).Valid {
		t.Fatal("inventory bypass")
	}
	d, _ = JourneyTemplate("order")
	for i := range d.Steps {
		if d.Steps[i].Kind == "create_order" {
			d.Steps[i].VisibleWhen = "optional"
		}
	}
	if ValidateJourney(d).Valid {
		t.Fatal("skipping mandatory order boundary")
	}
	d, _ = JourneyTemplate("order")
	d.Steps[0].Text = "<script>alert(1)</script>"
	if ValidateJourney(d).Valid {
		t.Fatal("markup accepted")
	}
}
