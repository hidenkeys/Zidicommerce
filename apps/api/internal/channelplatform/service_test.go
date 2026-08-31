package channelplatform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type channelFixture struct {
	db      *gorm.DB
	service *Service
	actor   auth.CurrentUser
	other   auth.CurrentUser
}

func newChannelFixture(t *testing.T) channelFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&organization.Organization{},
		&organization.AuditLog{},
		&ChannelConnection{},
		&ProviderAccount{},
		&ChannelIdentity{},
		&CredentialReference{},
		&HealthCheck{},
		&ProviderEvent{},
		&MetricDaily{},
	); err != nil {
		t.Fatal(err)
	}
	actor := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	other := auth.CurrentUser{ID: uuid.New(), OrganizationID: uuid.New(), Role: authz.MerchantAdmin}
	for _, org := range []organization.Organization{
		{ID: actor.OrganizationID, Name: "First Merchant", Slug: "first-merchant", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"},
		{ID: other.OrganizationID, Name: "Other Merchant", Slug: "other-merchant", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"},
	} {
		if err := db.Create(&org).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(db)
	service.now = func() time.Time { return time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC) }
	return channelFixture{db: db, service: service, actor: actor, other: other}
}

func createConnection(t *testing.T, fx channelFixture, actor auth.CurrentUser, provider, ownership string) ConnectionView {
	t.Helper()
	connection, err := fx.service.CreateConnection(context.Background(), actor, CreateConnectionInput{Provider: provider, DisplayName: strings.ToUpper(provider), OwnershipModel: ownership, Environment: EnvironmentSandbox})
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func TestConnectionsAreTenantScopedAndPermissioned(t *testing.T) {
	fx := newChannelFixture(t)
	first := createConnection(t, fx, fx.actor, "whatsapp", OwnershipMerchantManaged)
	_ = createConnection(t, fx, fx.other, "instagram", OwnershipZidiManaged)

	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	connections, err := fx.service.ListConnections(context.Background(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].ID != first.ID {
		t.Fatalf("expected only the viewer tenant's connection, got %+v", connections)
	}
	if _, err := fx.service.CreateConnection(context.Background(), viewer, CreateConnectionInput{Provider: "web", DisplayName: "Web"}); err == nil {
		t.Fatal("viewer must not create channel connections")
	}
	manager := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.StoreManager}
	if _, err := fx.service.ListConnections(context.Background(), manager); err == nil {
		t.Fatal("store manager must not receive organization channel configuration")
	}
	if _, err := fx.service.GetConnection(context.Background(), fx.other, first.ID); err == nil {
		t.Fatal("cross-tenant channel detail must not be visible")
	}
}

func TestOwnershipAndLifecycleTransitionsAreExplicitAndAudited(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "instagram", OwnershipZidiManaged)
	if connection.OwnershipModel != OwnershipZidiManaged || connection.Status != StatusSetupRequired {
		t.Fatalf("unexpected initial connection: %+v", connection)
	}
	healthy := StatusHealthy
	if _, err := fx.service.UpdateConnection(context.Background(), fx.actor, connection.ID, UpdateConnectionInput{Status: &healthy}); err == nil {
		t.Fatal("setup required must not jump directly to healthy")
	}
	for _, status := range []string{StatusConnecting, StatusConnected, StatusHealthy} {
		status := status
		updated, err := fx.service.UpdateConnection(context.Background(), fx.actor, connection.ID, UpdateConnectionInput{Status: &status})
		if err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
		connection = updated
	}
	for _, status := range []string{StatusDisconnected, StatusConnecting, StatusConnected, StatusHealthy} {
		status := status
		updated, err := fx.service.UpdateConnection(context.Background(), fx.actor, connection.ID, UpdateConnectionInput{Status: &status})
		if err != nil {
			t.Fatalf("disconnect/reconnect transition to %s: %v", status, err)
		}
		connection = updated
	}
	ownership := OwnershipMerchantManaged
	connection, err := fx.service.UpdateConnection(context.Background(), fx.actor, connection.ID, UpdateConnectionInput{OwnershipModel: &ownership})
	if err != nil {
		t.Fatal(err)
	}
	if connection.OwnershipModel != OwnershipMerchantManaged || connection.LastConnectedAt == nil {
		t.Fatalf("ownership or connection timestamp was not persisted: %+v", connection)
	}
	var ownershipAudits int64
	if err := fx.db.Model(&organization.AuditLog{}).Where("organization_id = ? AND action = ?", fx.actor.OrganizationID, "channel_ownership_changed").Count(&ownershipAudits).Error; err != nil {
		t.Fatal(err)
	}
	if ownershipAudits != 1 {
		t.Fatalf("expected one ownership audit, got %d", ownershipAudits)
	}
	viewer := auth.CurrentUser{ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, Role: authz.Viewer}
	zidiOwned := OwnershipZidiManaged
	if _, err := fx.service.UpdateConnection(context.Background(), viewer, connection.ID, UpdateConnectionInput{OwnershipModel: &zidiOwned}); err == nil {
		t.Fatal("viewer must not change connection management mode")
	}
	archived, err := fx.service.ArchiveConnection(context.Background(), fx.actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != StatusArchived {
		t.Fatalf("expected archived connection, got %s", archived.Status)
	}
	connecting := StatusConnecting
	if _, err := fx.service.UpdateConnection(context.Background(), fx.actor, connection.ID, UpdateConnectionInput{Status: &connecting}); err == nil {
		t.Fatal("archived channels must not be reactivated")
	}
}

func TestCredentialReferenceNeverExposesStoredReferenceOrAcceptsRawToken(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "whatsapp", OwnershipMerchantManaged)
	view, err := fx.service.UpsertCredentialReference(context.Background(), fx.actor, connection.ID, CredentialReferenceInput{CredentialType: "access", SecretRef: "vault://zidi/channels/access-token", Status: CredentialPresent})
	if err != nil {
		t.Fatal(err)
	}
	if !view.HasSecretReference || view.ReferenceType != "vault" {
		t.Fatalf("expected safe reference status, got %+v", view)
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "vault://") || strings.Contains(string(body), `"secret_ref":`) || strings.Contains(string(body), "access-token") {
		t.Fatalf("credential API exposed the stored reference: %s", body)
	}
	if _, err := fx.service.UpsertCredentialReference(context.Background(), fx.actor, connection.ID, CredentialReferenceInput{CredentialType: "access", SecretRef: "raw-provider-token", Status: CredentialPresent}); err == nil {
		t.Fatal("raw provider tokens must be rejected")
	}
	var stored CredentialReference
	if err := fx.db.Where("organization_id = ? AND channel_connection_id = ?", fx.actor.OrganizationID, connection.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SecretRef != "vault://zidi/channels/access-token" {
		t.Fatalf("expected reference to remain server-side, got %q", stored.SecretRef)
	}
	if _, err := fx.service.ListCredentials(context.Background(), fx.other, connection.ID); err == nil {
		t.Fatal("cross-tenant credential status lookup must fail")
	}
}

func TestHealthSummaryIsTenantScopedAndUpdatesConnectedLifecycle(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "web", OwnershipMerchantManaged)
	for _, status := range []string{StatusConnecting, StatusConnected} {
		status := status
		if _, err := fx.service.UpdateConnection(context.Background(), fx.actor, connection.ID, UpdateConnectionInput{Status: &status}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.service.RecordHealthCheck(context.Background(), fx.actor.OrganizationID, connection.ID, HealthCheckInput{Status: HealthFailed, ErrorCode: "adapter_unavailable", ErrorMessage: "No provider adapter is configured."}); err != nil {
		t.Fatal(err)
	}
	summary, err := fx.service.GetHealthSummary(context.Background(), fx.actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != HealthFailed || summary.FailedCount != 1 || summary.LastCheck == nil {
		t.Fatalf("unexpected health summary: %+v", summary)
	}
	detail, err := fx.service.GetConnection(context.Background(), fx.actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Connection.Status != StatusRequiresAttention {
		t.Fatalf("failed health check should require attention, got %s", detail.Connection.Status)
	}
	if _, err := fx.service.GetHealthSummary(context.Background(), fx.other, connection.ID); err == nil {
		t.Fatal("cross-tenant health summary must fail")
	}
}

func TestProviderEventsAreIdempotentAndTenantScoped(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "instagram", OwnershipMerchantManaged)
	input := ProviderEventInput{OrganizationID: fx.actor.OrganizationID, ConnectionID: connection.ID, Provider: "instagram", EventType: "message_received", ProviderEventID: "provider-event-1", IdempotencyKey: "event-key-1", PayloadMetadata: map[string]any{"message_type": "text"}}
	first, created, err := fx.service.RecordProviderEvent(context.Background(), input)
	if err != nil || !created {
		t.Fatalf("record first event: created=%t err=%v", created, err)
	}
	second, created, err := fx.service.RecordProviderEvent(context.Background(), input)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("duplicate event was not idempotent: created=%t first=%s second=%s err=%v", created, first.ID, second.ID, err)
	}
	input.ProviderEventID = "provider-event-2"
	third, created, err := fx.service.RecordProviderEvent(context.Background(), input)
	if err != nil || created || third.ID != first.ID {
		t.Fatalf("duplicate idempotency key should return first event: created=%t event=%+v err=%v", created, third, err)
	}
	if _, _, err := fx.service.RecordProviderEvent(context.Background(), ProviderEventInput{OrganizationID: fx.other.OrganizationID, ConnectionID: connection.ID, Provider: "instagram", EventType: "message_received", ProviderEventID: "other"}); err == nil {
		t.Fatal("cross-tenant provider event must fail")
	}
	events, err := fx.service.ListProviderEvents(context.Background(), fx.actor, connection.ID, 20)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one stored event, got %d err=%v", len(events), err)
	}
}

func TestMetricsSummaryIsTenantScopedAndUsesDatabaseRows(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "web", OwnershipMerchantManaged)
	for day, inbound := range []int64{4, 6} {
		_, err := fx.service.UpsertMetric(context.Background(), MetricInput{OrganizationID: fx.actor.OrganizationID, ConnectionID: connection.ID, MetricDate: time.Date(2026, 8, 27+day, 0, 0, 0, 0, time.UTC), InboundCount: inbound, OutboundCount: inbound - 1, FailedOutboundCount: int64(day), ConversationsStarted: 2, ConversationsAIHandled: 1, ConversationsHumanHandled: 1, AverageResponseMS: int64(100 + day*100)})
		if err != nil {
			t.Fatal(err)
		}
	}
	summary, err := fx.service.GetMetricsSummary(context.Background(), fx.actor, connection.ID, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if summary.InboundCount != 10 || summary.OutboundCount != 8 || summary.Days != 2 || summary.AverageResponseMS != 150 {
		t.Fatalf("unexpected metrics summary: %+v", summary)
	}
	if _, err := fx.service.GetMetricsSummary(context.Background(), fx.other, connection.ID, time.Time{}, time.Time{}); err == nil {
		t.Fatal("cross-tenant metrics must fail")
	}
}

func TestNoopAdapterNormalizesWithoutProviderSpecificFields(t *testing.T) {
	connectionID := uuid.New()
	adapter := NewNoopAdapter("web")
	events, err := adapter.NormalizeInbound(context.Background(), ProviderEnvelope{
		Provider: "web", ConnectionID: connectionID, ReceivedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		NormalizedEvents: []InboundEvent{{ProviderEventID: "event-1", ExternalCustomerID: "customer-1", ExternalMessageID: "message-1", ExternalConversationID: "conversation-1", MessageType: "text", Body: "Hello"}},
	})
	if err != nil || len(events) != 1 {
		t.Fatalf("normalize event: %+v err=%v", events, err)
	}
	if events[0].Provider != "web" || events[0].ChannelConnectionID != connectionID || events[0].Timestamp.IsZero() {
		t.Fatalf("event was not normalized: %+v", events[0])
	}
	request, err := adapter.BuildOutbound(context.Background(), OutboundCommand{Provider: "web", ChannelConnectionID: connectionID, ExternalCustomerID: "customer-1", ExternalConversationID: "conversation-1", MessageType: "text", Body: "Welcome", IdempotencyKey: "reply-1"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Payload["body"] != "Welcome" || request.Payload["external_customer_id"] != "customer-1" {
		t.Fatalf("unexpected generic outbound request: %+v", request)
	}
	if _, exists := request.Payload["phone_number_id"]; exists {
		t.Fatal("generic outbound request must not require WhatsApp fields")
	}
}
