package auth

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
)

func TestIssueAndParseToken(t *testing.T) {
	manager := NewTokenManager(config.JWTConfig{
		Secret:     "test-secret",
		Issuer:     "zidicommerce-test",
		TTLMinutes: 5,
	})

	userID := uuid.New()
	orgID := uuid.New()
	token, err := manager.Issue(userID, orgID, authz.MerchantAdmin)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != userID || claims.OrganizationID != orgID || claims.Role != authz.MerchantAdmin.String() {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}
