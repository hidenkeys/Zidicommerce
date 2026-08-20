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
)

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	tokens := auth.NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5})
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	app.Get("/v1/auth/me", auth.Middleware(tokens), meHandler)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/auth/me", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestProtectedRouteAcceptsValidToken(t *testing.T) {
	tokens := auth.NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5})
	token, err := tokens.Issue(uuid.New(), uuid.New(), authz.MerchantAdmin)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	app.Get("/v1/auth/me", auth.Middleware(tokens), meHandler)

	req := httptest.NewRequest("GET", "/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
