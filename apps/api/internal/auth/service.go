package auth

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"golang.org/x/crypto/bcrypt"
)

type UserIdentity struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Email          string
	PasswordHash   string
	Role           authz.Role
	Status         string
}

type UserReader interface {
	FindByEmail(ctx context.Context, email string) (UserIdentity, error)
}

type Service struct {
	users  UserReader
	tokens *TokenManager
}

func NewService(users UserReader, tokens *TokenManager) *Service {
	return &Service{users: users, tokens: tokens}
}

func (s *Service) Login(ctx context.Context, email string, password string) (string, UserIdentity, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || strings.TrimSpace(password) == "" {
		return "", UserIdentity{}, httperror.BadRequest("Email and password are required")
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return "", UserIdentity{}, httperror.Unauthorized("Invalid email or password")
	}
	if user.Status != "active" {
		return "", UserIdentity{}, httperror.Forbidden("User account is not active")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", UserIdentity{}, httperror.Unauthorized("Invalid email or password")
	}

	token, err := s.tokens.Issue(user.ID, user.OrganizationID, user.Role)
	if err != nil {
		return "", UserIdentity{}, err
	}
	return token, user, nil
}
