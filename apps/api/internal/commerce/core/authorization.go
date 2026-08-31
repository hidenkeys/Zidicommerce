package core

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

func (s *Service) authorize(ctx context.Context, actor auth.CurrentUser, permission authz.Permission, resourceType string, resourceID *uuid.UUID) error {
	if actor.Role.HasPermission(permission) {
		return nil
	}
	s.auditAuthorizationDenied(ctx, actor, permission, resourceType, resourceID, nil)
	return httperror.Forbidden("You do not have permission to perform this action")
}

func (s *Service) auditAuthorizationDenied(ctx context.Context, actor auth.CurrentUser, permission authz.Permission, resourceType string, resourceID *uuid.UUID, storeID *uuid.UUID) {
	if actor.OrganizationID == uuid.Nil {
		return
	}
	metadata := fmt.Sprintf(`{"permission":%q`, permission)
	if storeID != nil {
		metadata += fmt.Sprintf(`,"store_id":%q`, *storeID)
	}
	metadata += "}"
	_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, resourceType, resourceID, "authorization_denied", metadata)
}

func (s *Service) ensureStoreAccessible(ctx context.Context, actor auth.CurrentUser, storeID uuid.UUID, permission authz.Permission) error {
	return s.ensureStoreAccessibleWithDB(s.db.WithContext(ctx), actor, storeID, permission)
}

func (s *Service) ensureStoreAccessibleWithDB(db *gorm.DB, actor auth.CurrentUser, storeID uuid.UUID, permission authz.Permission) error {
	if !actor.Role.HasPermission(permission) {
		s.auditAuthorizationDenied(context.Background(), actor, permission, "store", &storeID, &storeID)
		return httperror.Forbidden("You do not have permission to perform this action")
	}
	var count int64
	query := db.Table("stores").Where("stores.organization_id = ? AND stores.id = ?", actor.OrganizationID, storeID)
	if actor.Role.RequiresStoreScope() {
		query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = stores.organization_id AND sua.store_id = stores.id AND sua.user_id = ?", actor.ID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		s.auditAuthorizationDenied(context.Background(), actor, permission, "store", &storeID, &storeID)
		return httperror.NotFound("Store not found")
	}
	return nil
}

func (s *Service) storeScopedQuery(db *gorm.DB, actor auth.CurrentUser, tableName, storeColumn string) *gorm.DB {
	if actor.Role.RequiresStoreScope() {
		return db.Joins(
			"JOIN store_user_assignments sua ON sua.organization_id = "+tableName+".organization_id AND sua.store_id = "+tableName+"."+storeColumn+" AND sua.user_id = ?",
			actor.ID,
		)
	}
	return db
}
