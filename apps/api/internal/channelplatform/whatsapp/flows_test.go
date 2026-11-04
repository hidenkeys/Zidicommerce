package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type flowTestTransport struct{ handler http.Handler }

func (f flowTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w.Result(), nil
}

type flowTestServer struct {
	URL    string
	client *http.Client
}

func (f flowTestServer) Client() *http.Client { return f.client }
func (f flowTestServer) Close()               {}
func newFlowTestServer(handler http.Handler) flowTestServer {
	return flowTestServer{URL: "https://graph.example.test", client: &http.Client{Transport: flowTestTransport{handler: handler}}}
}

func TestJourneyFlowProviderLifecycleIsolationAndImmutability(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	ctx := context.Background()
	if err := fx.db.AutoMigrate(&bot.Bot{}, &bot.BotVersion{}, &bot.VersionModule{}, &bot.Variable{}, &bot.Question{}, &bot.Action{}, &bot.Condition{}, &bot.Integration{}, &bot.Step{}, &bot.CommerceWorkflowConfiguration{}, &bot.PublishedSnapshot{}, &bot.JourneyBinding{}, &bot.JourneyFlowDeployment{}); err != nil {
		t.Fatal(err)
	}
	count, clone := 0, ""
	server := newFlowTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-secret" || strings.Contains(r.URL.RawQuery, "secret") {
			t.Error("credential transport")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/waba-1/flows"):
			count++
			r.ParseForm()
			clone = r.Form.Get("clone_flow_id")
			fmt.Fprintf(w, `{"id":"flow-%d"}`, count)
		case strings.HasSuffix(r.URL.Path, "/assets"):
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Error(err)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			var data map[string]any
			if err := json.NewDecoder(file).Decode(&data); err != nil || data["data_api_version"] != "3.0" {
				t.Error("compiled upload", err)
			}
			fmt.Fprint(w, `{"success":true}`)
		case r.Method == "GET":
			fmt.Fprint(w, `{"id":"flow-1","status":"DRAFT","validation_errors":[],"preview":{"preview_url":"https://business.facebook.com/flow-preview"}}`)
		default:
			fmt.Fprint(w, `{"success":true}`)
		}
	}))
	defer server.Close()
	fx.service.publicURL = "https://qa.example.test"
	fx.service.ConfigureFlows(FlowConfig{Enabled: true, TestWABAID: "waba-1", TestPhoneID: "1234567890", GraphBaseURL: server.URL}, server.Client())
	bs := bot.NewService(fx.db)
	detail, err := bs.CreateJourney(ctx, fx.actor, "support", "Provider QA")
	if err != nil {
		t.Fatal(err)
	}
	d := detail.Definition
	d.ConnectionID = &fx.connection.ID
	detail, err = bs.SaveJourney(ctx, fx.actor, detail.Version.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.PrepareJourneyFlow(ctx, fx.other, fx.connection.ID, detail.Version.ID); err == nil {
		t.Fatal("tenant provider access")
	}
	viewer := fx.actor
	viewer.Role = authz.Viewer
	if _, err := fx.service.PrepareJourneyFlow(ctx, viewer, fx.connection.ID, detail.Version.ID); err == nil {
		t.Fatal("viewer deployment")
	}
	deployment, err := fx.service.PrepareJourneyFlow(ctx, fx.actor, fx.connection.ID, detail.Version.ID)
	if err != nil || deployment.Status != "validated" {
		t.Fatal("prepare", err, deployment)
	}
	if err := fx.service.RegisterFlowKey(ctx, fx.actor, fx.connection.ID); err != nil {
		t.Fatal(err)
	}
	d.Steps[1].Text = "Changed after upload"
	if _, err = bs.SaveJourney(ctx, fx.actor, detail.Version.ID, d); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.JourneyFlowOperation(ctx, fx.actor, fx.connection.ID, detail.Version.ID, "publish"); err == nil {
		t.Fatal("stale compiled revision published")
	}
	if _, err := fx.service.PrepareJourneyFlow(ctx, fx.actor, fx.connection.ID, detail.Version.ID); err != nil {
		t.Fatal(err)
	}
	deployment, err = fx.service.JourneyFlowOperation(ctx, fx.actor, fx.connection.ID, detail.Version.ID, "publish")
	if err != nil || deployment.Status != "published" {
		t.Fatal("publish", err)
	}
	if _, err := bs.SaveJourney(ctx, fx.actor, detail.Version.ID, d); err == nil {
		t.Fatal("published provider mutated")
	}
	next, err := bs.CloneJourneyDraft(ctx, fx.actor, detail.Bot.ID, detail.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.PrepareJourneyFlow(ctx, fx.actor, fx.connection.ID, next.Version.ID); err != nil {
		t.Fatal(err)
	}
	if count != 2 || clone != "flow-1" {
		t.Fatal("revision cloning", count, clone)
	}
	if _, err := fx.service.JourneyFlowOperation(ctx, fx.actor, fx.connection.ID, detail.Version.ID, "deprecate"); err != nil {
		t.Fatal(err)
	}
	fx.db.Model(&channelplatform.ChannelConnection{}).Where("id = ?", fx.connection.ID).Update("environment", channelplatform.EnvironmentProduction)
	if err := fx.service.RegisterFlowKey(ctx, fx.actor, fx.connection.ID); err == nil {
		t.Fatal("production asset altered")
	}
	fx.service.ConfigureFlows(FlowConfig{}, server.Client())
	if _, err := fx.service.PrepareJourneyFlow(ctx, fx.actor, fx.connection.ID, next.Version.ID); err == nil {
		t.Fatal("disabled provider calls")
	}
}

func TestJourneyFlowProviderFailureIsSafe(t *testing.T) {
	fx := newWhatsAppFixture(t)
	view := configureWhatsApp(t, fx)
	server := newFlowTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"access-secret private internal payload"}}`)
	}))
	defer server.Close()
	fx.service.ConfigureFlows(FlowConfig{Enabled: true, GraphBaseURL: server.URL}, server.Client())
	_, err := fx.service.flowGraph(context.Background(), view.Configuration, "POST", "some-flow/publish", "", nil)
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "429") {
		t.Fatal("unsafe provider failure", err)
	}
}
