package tenant

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewScopeRejectsMissingOrganization(t *testing.T) {
	if _, err := NewScope(uuid.Nil); err == nil {
		t.Fatal("expected missing organization to be rejected")
	}
}

func TestNewScopeAcceptsOrganization(t *testing.T) {
	orgID := uuid.New()
	scope, err := NewScope(orgID)
	if err != nil {
		t.Fatal(err)
	}
	if scope.OrganizationID != orgID {
		t.Fatalf("expected %s, got %s", orgID, scope.OrganizationID)
	}
}
