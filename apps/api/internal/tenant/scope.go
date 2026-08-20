package tenant

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Scope struct {
	OrganizationID uuid.UUID
}

func NewScope(organizationID uuid.UUID) (Scope, error) {
	if organizationID == uuid.Nil {
		return Scope{}, ErrMissingOrganization
	}
	return Scope{OrganizationID: organizationID}, nil
}

func (s Scope) Apply(db *gorm.DB) *gorm.DB {
	return db.Where("organization_id = ?", s.OrganizationID)
}

type tenantError string

func (e tenantError) Error() string {
	return string(e)
}

const ErrMissingOrganization tenantError = "organization_id is required for tenant-scoped operations"
