package organization

import (
	"context"

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
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return auth.UserIdentity{}, err
	}

	role := user.Role
	if !authz.IsValid(role.String()) {
		return auth.UserIdentity{}, httperror.Forbidden("User role is not valid")
	}

	return auth.UserIdentity{
		ID:             user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		PasswordHash:   user.PasswordHash,
		Role:           role,
		Status:         user.Status,
	}, nil
}
