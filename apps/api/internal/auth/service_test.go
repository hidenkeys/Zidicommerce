package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"golang.org/x/crypto/bcrypt"
)

type fakeUserReader struct {
	user UserIdentity
	err  error
}

func (r fakeUserReader) FindByEmail(context.Context, string) (UserIdentity, error) {
	if r.err != nil {
		return UserIdentity{}, r.err
	}
	return r.user, nil
}

func TestServiceLoginIssuesTokenForActiveUser(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	tokens := NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5})
	service := NewService(fakeUserReader{user: UserIdentity{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		Email:          "owner@example.com",
		PasswordHash:   string(hash),
		Role:           authz.MerchantAdmin,
		Status:         "active",
	}}, tokens)

	token, user, err := service.Login(context.Background(), "owner@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected token")
	}
	if user.Role != authz.MerchantAdmin {
		t.Fatalf("expected merchant_admin, got %s", user.Role)
	}
}

func TestServiceLoginRejectsWrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	service := NewService(fakeUserReader{user: UserIdentity{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		Email:          "owner@example.com",
		PasswordHash:   string(hash),
		Role:           authz.MerchantAdmin,
		Status:         "active",
	}}, NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5}))

	if _, _, err := service.Login(context.Background(), "owner@example.com", "wrong"); err == nil {
		t.Fatal("expected wrong password to be rejected")
	}
}

func TestServiceLoginHidesMissingUser(t *testing.T) {
	service := NewService(fakeUserReader{err: errors.New("missing")}, NewTokenManager(config.JWTConfig{Secret: "test-secret", Issuer: "test", TTLMinutes: 5}))
	if _, _, err := service.Login(context.Background(), "missing@example.com", "password123"); err == nil {
		t.Fatal("expected missing user to be rejected")
	}
}
