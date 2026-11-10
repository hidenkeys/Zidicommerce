package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	tokens := auth.NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	app.Get("/v1/auth/me", auth.Middleware(tokens, nil), meHandler)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/auth/me", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestProtectedRouteRevokesStaleStaffTokens(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&organization.User{}, &organization.OrganizationMembership{}); err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	user := organization.User{ID: uuid.New(), OrganizationID: &orgID, Role: authz.MerchantAdmin, Status: "active"}
	member := organization.OrganizationMembership{ID: uuid.New(), OrganizationID: orgID, UserID: user.ID, Role: user.Role, Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	tokens := auth.NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	app.Get("/v1/auth/me", auth.Middleware(tokens, organization.NewUserRepository(db)), meHandler)
	issue := func(org uuid.UUID, role authz.Role) string {
		t.Helper()
		token, err := tokens.Issue(user.ID, org, role)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	check := func(token string, expected int) {
		t.Helper()
		req := httptest.NewRequest("GET", "/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != expected {
			t.Fatalf("expected %d, got %d", expected, resp.StatusCode)
		}
	}

	adminToken := issue(orgID, authz.MerchantAdmin)
	check(adminToken, 200)
	check(issue(uuid.New(), authz.MerchantAdmin), 401)

	if err := db.Model(&member).Update("role", authz.Viewer).Error; err != nil {
		t.Fatal(err)
	}
	check(adminToken, 401)
	viewerToken := issue(orgID, authz.Viewer)
	check(viewerToken, 200)

	if err := db.Model(&member).Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	check(viewerToken, 401)
}
