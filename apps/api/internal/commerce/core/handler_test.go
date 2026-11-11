package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
)

func TestStoreDetailsRequireStoreViewPermission(t *testing.T) {
	fixture := newCommerceFixture(t, 1)
	tokens := auth.NewTokenManager(config.JWTConfig{Secret: "store-details-test", Issuer: "test", TTLMinutes: 5})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	NewHandler(fixture.service, tokens).Register(app.Group("/v1", auth.Middleware(tokens, organization.NewUserRepository(fixture.db))))

	for _, tc := range []struct {
		role authz.Role
		want int
	}{
		{authz.MerchantAdmin, http.StatusOK},
		{authz.SupportAgent, http.StatusForbidden},
		{authz.Viewer, http.StatusOK},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			orgID := fixture.actor.OrganizationID
			user := organization.User{ID: uuid.New(), OrganizationID: &orgID, Email: string(tc.role) + "@example.invalid", Role: tc.role, Status: "active"}
			if err := fixture.db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			member := organization.OrganizationMembership{ID: uuid.New(), OrganizationID: orgID, UserID: user.ID, Role: tc.role, Status: "active"}
			if err := fixture.db.Create(&member).Error; err != nil {
				t.Fatal(err)
			}
			token, err := tokens.Issue(user.ID, orgID, tc.role)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/v1/stores/"+fixture.store.ID.String(), nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("store detail status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
