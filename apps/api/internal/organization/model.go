package organization

import (
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
)

type Organization struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Description     string    `json:"description"`
	LogoURL         string    `gorm:"column:logo_url" json:"logo_url"`
	Currency        string    `json:"currency"`
	Timezone        string    `json:"timezone"`
	Country         string    `json:"country"`
	ContactName     string    `json:"contact_name"`
	ContactEmail    string    `json:"contact_email"`
	ContactPhone    string    `json:"contact_phone"`
	OnboardingState string    `json:"onboarding_state"`
	Status          string    `json:"status"`
	Metadata        string    `json:"metadata"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (Organization) TableName() string {
	return "organizations"
}

type User struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	Email          string     `json:"email"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	PasswordHash   string     `json:"-"`
	Role           authz.Role `json:"role"`
	Status         string     `json:"status"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

type OrganizationMembership struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_org_member_user" json:"organization_id"`
	UserID         uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_org_member_user" json:"user_id"`
	Role           authz.Role `json:"role"`
	Status         string     `json:"status"`
	IsOwner        bool       `json:"is_owner"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	User           User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (OrganizationMembership) TableName() string {
	return "organization_memberships"
}

type OrganizationInvitation struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	Email            string     `json:"email"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	Role             authz.Role `json:"role"`
	StoreIDs         string     `json:"store_ids"`
	Status           string     `json:"status"`
	TokenHash        string     `gorm:"uniqueIndex" json:"-"`
	InvitedByUserID  uuid.UUID  `gorm:"type:uuid" json:"invited_by_user_id"`
	AcceptedByUserID *uuid.UUID `gorm:"type:uuid" json:"accepted_by_user_id,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	AcceptedAt       *time.Time `json:"accepted_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (OrganizationInvitation) TableName() string {
	return "organization_invitations"
}

type AuditLog struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	ActorUserID    *uuid.UUID `gorm:"type:uuid" json:"actor_user_id,omitempty"`
	TargetType     string     `json:"target_type"`
	TargetID       *uuid.UUID `gorm:"type:uuid" json:"target_id,omitempty"`
	Action         string     `json:"action"`
	Metadata       string     `json:"metadata"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
