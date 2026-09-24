package channelplatform

import (
	"context"
	"testing"
)

func TestProviderCatalogRepresentsTikTokMessagingTruthfully(t *testing.T) {
	for _, provider := range ProviderCatalog() {
		if provider.Provider != "tiktok" {
			continue
		}
		statuses := map[string]string{}
		for _, capability := range provider.Capabilities {
			statuses[capability.Capability] = capability.Support
		}
		if statuses["oauth_onboarding"] != ProviderSupportUnverified || statuses["profile_read"] != ProviderSupportUnverified {
			t.Fatalf("expected TikTok account capabilities to remain unverified, got %+v", statuses)
		}
		if statuses["inbound_text"] != ProviderSupportUnavailable || statuses["outbound_text"] != ProviderSupportUnavailable {
			t.Fatalf("TikTok messaging must be unavailable, got %+v", statuses)
		}
		return
	}
	t.Fatal("TikTok provider definition missing")
}

func TestConnectionCapabilitiesResolveFromReviewScopesAndAsset(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "instagram", OwnershipMerchantManaged)
	if len(connection.CapabilityStates) == 0 || connection.Capabilities == nil {
		t.Fatalf("expected initialized capability states, got %+v", connection)
	}
	for _, state := range connection.CapabilityStates {
		if state.Capability == "outbound_text" && state.Status != CapabilityAwaitingPermissionReview {
			t.Fatalf("expected review-gated Instagram messaging, got %+v", state)
		}
	}
	if err := fx.service.ResolveConnectionCapabilities(context.Background(), fx.actor.OrganizationID, connection.ID, CapabilityResolution{ReviewApproved: true, AssetReady: true, GrantedScopes: []string{"instagram_business_basic"}}); err != nil {
		t.Fatal(err)
	}
	view, err := fx.service.GetConnection(context.Background(), fx.actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]CapabilityView{}
	for _, state := range view.Connection.CapabilityStates {
		states[state.Capability] = state
	}
	if states["profile_read"].Status != CapabilityAvailable {
		t.Fatalf("expected profile capability after scope grant, got %+v", states["profile_read"])
	}
	if states["outbound_text"].Status != CapabilityMissingPermission || len(states["outbound_text"].MissingScopes) != 1 {
		t.Fatalf("expected missing messaging permission, got %+v", states["outbound_text"])
	}
	if err := fx.service.RequireCapability(context.Background(), fx.actor.OrganizationID, connection.ID, "outbound_text"); err == nil {
		t.Fatal("outbound text must be blocked while its provider permission is missing")
	}
	if err := fx.service.RequireCapability(context.Background(), fx.other.OrganizationID, connection.ID, "profile_read"); err == nil {
		t.Fatal("capability checks must be tenant scoped")
	}
}

func TestTikTokOutboundCapabilityRemainsBlocked(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "tiktok", OwnershipMerchantManaged)
	if err := fx.service.RequireCapability(context.Background(), fx.actor.OrganizationID, connection.ID, "outbound_text"); err == nil {
		t.Fatal("TikTok messaging must not pass the outbound capability guard")
	}
}
