package authz_test

import (
	"testing"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
)

func TestPublishedFieldActionsHaveAuthorizationPolicies(t *testing.T) {
	for _, action := range bot.SystemActions() {
		if action.Category != "Field service" {
			continue
		}
		t.Run(action.Key, func(t *testing.T) {
			policy := authz.PolicyForRuntimeAction(action.Key)
			if policy.Permission == "" || policy.Class == "" || policy.Confirmation == "" {
				t.Fatal("published Field action has no complete authorization policy")
			}
			wantMode := authz.RuntimeActionWrite
			switch action.Key {
			case "get_service_welcome", "list_service_pools", "select_service_pool":
				wantMode = authz.RuntimeActionRead
			}
			if policy.Mode != wantMode {
				t.Fatalf("mode = %q, want %q", policy.Mode, wantMode)
			}
		})
	}
	if authz.PolicyForRuntimeAction("unknown_field_action").Permission != "" {
		t.Fatal("unknown actions must remain denied")
	}
}
