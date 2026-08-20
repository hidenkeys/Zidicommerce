package organization

import (
	"context"
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
	var org Organization
	err := scope.Apply(r.db.WithContext(ctx)).Where("id = ?", id).First(&org).Error
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
