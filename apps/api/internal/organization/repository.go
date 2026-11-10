package organization

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/tenant"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByID(ctx context.Context, scope tenant.Scope, id uuid.UUID) (Organization, error) {
	if id != scope.OrganizationID {
		return Organization{}, httperror.NotFound("Organization not found")
	}
	var org Organization
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&org).Error
	if err == gorm.ErrRecordNotFound {
		return Organization{}, httperror.NotFound("Organization not found")
	}
	return org, err
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (auth.UserIdentity, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("lower(email) = ?", strings.ToLower(email)).First(&user).Error; err != nil {
		return auth.UserIdentity{}, err
	}
	if user.Status != "active" {
		return auth.UserIdentity{}, nilUserIdentity(user)
	}

	var membership OrganizationMembership
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", user.ID, "active").
		Order("is_owner DESC, created_at ASC").
		First(&membership).Error
	if err == nil {
		return auth.UserIdentity{
			ID:             user.ID,
			OrganizationID: membership.OrganizationID,
			Email:          user.Email,
			PasswordHash:   user.PasswordHash,
			Role:           membership.Role,
			Status:         user.Status,
		}, nil
	}
	if err != gorm.ErrRecordNotFound {
		return auth.UserIdentity{}, err
	}
	var membershipCount int64
	if err := r.db.WithContext(ctx).Model(&OrganizationMembership{}).Where("user_id = ?", user.ID).Count(&membershipCount).Error; err != nil {
		return auth.UserIdentity{}, err
	}
	if membershipCount > 0 {
		return auth.UserIdentity{}, httperror.Forbidden("User has no active organization membership")
	}

	role := user.Role
	if !authz.IsValid(role.String()) {
		return auth.UserIdentity{}, httperror.Forbidden("User role is not valid")
	}

	return auth.UserIdentity{
		ID:             user.ID,
		OrganizationID: optionalUUID(user.OrganizationID),
		Email:          user.Email,
		PasswordHash:   user.PasswordHash,
		Role:           role,
		Status:         user.Status,
	}, nil
}

// SessionRole checks the exact tenant in the token. Legacy users without any
// membership rows and users awaiting onboarding retain their account role.
func (r *UserRepository) SessionRole(ctx context.Context, userID, organizationID uuid.UUID) (authz.Role, error) {
	var session struct{ Role authz.Role }
	query := r.db.WithContext(ctx).Table("users AS u").
		Select("COALESCE(m.role, u.role) AS role").
		Joins("LEFT JOIN organization_memberships AS m ON m.user_id = u.id AND m.organization_id = ?", organizationID).
		Where("u.id = ? AND u.status = ?", userID, "active")

	legacy := "u.organization_id = ?"
	if organizationID == uuid.Nil {
		legacy = "u.organization_id IS NULL OR u.organization_id = ?"
	}
	err := query.Where(
		"m.status = ? OR (("+legacy+") AND NOT EXISTS (SELECT 1 FROM organization_memberships WHERE user_id = u.id))",
		"active", organizationID,
	).Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", httperror.Unauthorized("Session access is no longer active")
	}
	return session.Role, err
}

func (r *UserRepository) CreateUser(ctx context.Context, input auth.RegisterInput, passwordHash string) (auth.UserIdentity, error) {
	user := User{
		ID:           uuid.New(),
		Email:        strings.ToLower(strings.TrimSpace(input.Email)),
		FirstName:    strings.TrimSpace(input.FirstName),
		LastName:     strings.TrimSpace(input.LastName),
		PasswordHash: passwordHash,
		Role:         authz.Viewer,
		Status:       "active",
	}
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return auth.UserIdentity{}, err
	}
	return auth.UserIdentity{
		ID:             user.ID,
		OrganizationID: uuid.Nil,
		Email:          user.Email,
		PasswordHash:   user.PasswordHash,
		Role:           authz.Viewer,
		Status:         user.Status,
	}, nil
}

func optionalUUID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func nilUserIdentity(User) error {
	return httperror.Forbidden("User account is not active")
}
