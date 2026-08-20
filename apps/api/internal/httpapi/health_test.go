package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(context.Context) error {
	return p.err
}

func TestHealthEndpointReportsHealthyDatabase(t *testing.T) {
	app := fiber.New()
	app.Get("/health", healthHandler(fakePinger{}))

	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHealthEndpointReportsUnavailableDatabase(t *testing.T) {
	app := fiber.New()
	app.Get("/health", healthHandler(fakePinger{err: errors.New("db down")}))

	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}
