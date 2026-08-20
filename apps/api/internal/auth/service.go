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

type UserWriter interface {
	UserReader
	CreateUser(ctx context.Context, input RegisterInput, passwordHash string) (UserIdentity, error)
}

type RegisterInput struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
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

func (s *Service) Register(ctx context.Context, input RegisterInput) (string, UserIdentity, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	password := strings.TrimSpace(input.Password)
	if email == "" || len(password) < 8 {
		return "", UserIdentity{}, httperror.BadRequest("Email and a password of at least 8 characters are required")
	}
	writer, ok := s.users.(UserWriter)
	if !ok {
		return "", UserIdentity{}, httperror.Internal("Registration is not configured")
	}
	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return "", UserIdentity{}, httperror.BadRequest("A user with this email already exists")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", UserIdentity{}, err
	}
	input.Email = email
	user, err := writer.CreateUser(ctx, input, string(hash))
	if err != nil {
		return "", UserIdentity{}, err
	}
	token, err := s.tokens.Issue(user.ID, user.OrganizationID, user.Role)
	if err != nil {
		return "", UserIdentity{}, err
	}
	return token, user, nil
}
