package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) OnboardOrganization(ctx context.Context, actor auth.CurrentUser, input OrganizationInput) (organization.Organization, organization.OrganizationMembership, error) {
	if actor.ID == uuid.Nil {
		return organization.Organization{}, organization.OrganizationMembership{}, httperror.Unauthorized("Authentication is required")
	}
	if actor.OrganizationID != uuid.Nil && actor.Role != authz.PlatformAdmin {
		return organization.Organization{}, organization.OrganizationMembership{}, httperror.BadRequest("User already belongs to an organization")
	}
	input.normalize()
	if input.Name == "" || input.Slug == "" {
		return organization.Organization{}, organization.OrganizationMembership{}, httperror.BadRequest("Business name and slug are required")
	}

	var org organization.Organization
	var membership organization.OrganizationMembership
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org = organization.Organization{
			ID:              uuid.New(),
			Name:            input.Name,
			Slug:            input.Slug,
			Description:     input.Description,
			LogoURL:         input.LogoURL,
			Country:         input.Country,
			Currency:        defaultString(input.Currency, "NGN"),
			Timezone:        defaultString(input.Timezone, "Africa/Lagos"),
			ContactName:     input.ContactName,
			ContactEmail:    input.ContactEmail,
			ContactPhone:    input.ContactPhone,
			OnboardingState: onboardingState("business_profile"),
			Status:          StatusActive,
			Metadata:        jsonObject(input.Metadata),
		}
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		membership = organization.OrganizationMembership{
			ID:             uuid.New(),
			OrganizationID: org.ID,
			UserID:         actor.ID,
			Role:           authz.MerchantAdmin,
			Status:         "active",
			IsOwner:        true,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}
		if err := tx.Model(&organization.User{}).Where("id = ?", actor.ID).Updates(map[string]any{
			"organization_id": org.ID,
			"role":            authz.MerchantAdmin,
			"status":          "active",
			"updated_at":      s.now(),
		}).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, &org.ID, &actor.ID, "organization", &org.ID, "organization_created", "{}"); err != nil {
			return err
		}
		return s.auditTx(tx, &org.ID, &actor.ID, "member", &actor.ID, "member_joined", `{"role":"merchant_admin"}`)
	})
	return org, membership, err
}

func (s *Service) UpdateOnboardingState(ctx context.Context, actor auth.CurrentUser, state string) (organization.Organization, error) {
	if !actor.Role.CanManageOrganization() {
		return organization.Organization{}, httperror.Forbidden("You cannot update onboarding progress")
	}
	org, err := s.GetOrganization(ctx, actor, actor.OrganizationID)
	if err != nil {
		return organization.Organization{}, err
	}
	if strings.TrimSpace(state) == "" {
		return organization.Organization{}, httperror.BadRequest("Onboarding state is required")
	}
	if err := s.db.WithContext(ctx).Model(&org).Updates(map[string]any{"onboarding_state": jsonObject(state), "updated_at": s.now()}).Error; err != nil {
		return organization.Organization{}, err
	}
	return s.GetOrganization(ctx, actor, actor.OrganizationID)
}

func (s *Service) ListMembers(ctx context.Context, actor auth.CurrentUser) ([]organization.OrganizationMembership, error) {
	if !canViewTeam(actor.Role) {
		return nil, httperror.Forbidden("You cannot view team members")
	}
	var members []organization.OrganizationMembership
	err := s.db.WithContext(ctx).
		Where("organization_id = ?", actor.OrganizationID).
		Preload("User").
		Order("created_at ASC").
		Find(&members).Error
	return members, err
}

func (s *Service) InviteMember(ctx context.Context, actor auth.CurrentUser, input InviteInput) (organization.OrganizationInvitation, string, error) {
	if !actor.Role.CanManageOrganization() {
		return organization.OrganizationInvitation{}, "", httperror.Forbidden("You cannot invite team members")
	}
	input.normalize()
	role := authz.Role(input.Role)
	if input.Email == "" || !isAssignableMerchantRole(role) {
		return organization.OrganizationInvitation{}, "", httperror.BadRequest("Valid email and role are required")
	}
	if role == authz.PlatformAdmin {
		return organization.OrganizationInvitation{}, "", httperror.Forbidden("Platform admin cannot be granted by a merchant")
	}
	for _, storeID := range input.StoreIDs {
		if _, err := s.GetStore(ctx, actor, storeID); err != nil {
			return organization.OrganizationInvitation{}, "", err
		}
	}

	token, hash, err := secureInvitationToken()
	if err != nil {
		return organization.OrganizationInvitation{}, "", err
	}
	storeIDs, err := json.Marshal(input.StoreIDs)
	if err != nil {
		return organization.OrganizationInvitation{}, "", err
	}
	invitation := organization.OrganizationInvitation{
		ID:              uuid.New(),
		OrganizationID:  actor.OrganizationID,
		Email:           input.Email,
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		Role:            role,
		StoreIDs:        string(storeIDs),
		Status:          "pending",
		TokenHash:       hash,
		InvitedByUserID: actor.ID,
		ExpiresAt:       s.now().Add(7 * 24 * time.Hour),
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&invitation).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, &actor.OrganizationID, &actor.ID, "invitation", &invitation.ID, "member_invited", fmt.Sprintf(`{"email":%q,"role":%q}`, invitation.Email, invitation.Role)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return organization.OrganizationInvitation{}, "", err
	}
	if s.mailer != nil {
		_ = s.mailer.Send(email.Message{
			To:      invitation.Email,
			Subject: "You have been invited to ZidiCommerce",
			Text:    invitationEmailText(s.appBaseURL, token),
		})
	}
	return invitation, token, nil
}

func (s *Service) ListInvitations(ctx context.Context, actor auth.CurrentUser) ([]organization.OrganizationInvitation, error) {
	if !actor.Role.CanManageOrganization() {
		return nil, httperror.Forbidden("You cannot view invitations")
	}
	var invitations []organization.OrganizationInvitation
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Find(&invitations).Error
	return invitations, err
}

func (s *Service) AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (InvitationAcceptance, error) {
	token := strings.TrimSpace(input.Token)
	password := strings.TrimSpace(input.Password)
	if token == "" || len(password) < 8 {
		return InvitationAcceptance{}, httperror.BadRequest("Invitation token and a password of at least 8 characters are required")
	}
	tokenHash := hashInvitationToken(token)
	var accepted InvitationAcceptance
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation organization.OrganizationInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).First(&invitation).Error; err != nil {
			return mapNotFound(err, "Invitation not found")
		}
		if invitation.Status != "pending" || !invitation.ExpiresAt.After(s.now()) {
			return httperror.BadRequest("Invitation is expired or already used")
		}
		var user organization.User
		err := tx.Where("lower(email) = ?", strings.ToLower(invitation.Email)).First(&user).Error
		if err == gorm.ErrRecordNotFound {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if hashErr != nil {
				return hashErr
			}
			user = organization.User{
				ID:           uuid.New(),
				Email:        invitation.Email,
				FirstName:    defaultString(input.FirstName, invitation.FirstName),
				LastName:     defaultString(input.LastName, invitation.LastName),
				PasswordHash: string(hash),
				Role:         invitation.Role,
				Status:       "active",
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		membership := organization.OrganizationMembership{
			ID:             uuid.New(),
			OrganizationID: invitation.OrganizationID,
			UserID:         user.ID,
			Role:           invitation.Role,
			Status:         "active",
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{"role": invitation.Role, "status": "active", "updated_at": s.now()}),
		}).Create(&membership).Error; err != nil {
			return err
		}
		now := s.now()
		if err := tx.Model(&invitation).Updates(map[string]any{"status": "accepted", "accepted_by_user_id": user.ID, "accepted_at": &now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&organization.User{}).Where("id = ?", user.ID).Updates(map[string]any{"organization_id": invitation.OrganizationID, "role": invitation.Role, "status": "active", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := s.applyInvitationStoresTx(tx, invitation, user.ID); err != nil {
			return err
		}
		if err := s.auditTx(tx, &invitation.OrganizationID, &user.ID, "member", &user.ID, "member_joined", fmt.Sprintf(`{"role":%q}`, invitation.Role)); err != nil {
			return err
		}
		accepted = InvitationAcceptance{UserID: user.ID, OrganizationID: invitation.OrganizationID, Role: invitation.Role.String()}
		return nil
	})
	return accepted, err
}

func (s *Service) UpdateMember(ctx context.Context, actor auth.CurrentUser, memberID uuid.UUID, input MemberUpdateInput) (organization.OrganizationMembership, error) {
	if !actor.Role.CanManageOrganization() {
		return organization.OrganizationMembership{}, httperror.Forbidden("You cannot update members")
	}
	var membership organization.OrganizationMembership
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, memberID).First(&membership).Error; err != nil {
			return mapNotFound(err, "Member not found")
		}
		updates := map[string]any{"updated_at": s.now()}
		effectiveRole := membership.Role.String()
		action := "member_updated"
		if input.Role != "" {
			role := authz.Role(strings.ToLower(strings.TrimSpace(input.Role)))
			if !isAssignableMerchantRole(role) {
				return httperror.BadRequest("Role is not assignable")
			}
			updates["role"] = role
			effectiveRole = role.String()
			action = "member_role_changed"
		}
		if input.Status != "" {
			status := strings.ToLower(strings.TrimSpace(input.Status))
			if status != "active" && status != "disabled" {
				return httperror.BadRequest("Unsupported member status")
			}
			updates["status"] = status
			if status == "disabled" {
				action = "member_deactivated"
			}
		}
		if err := tx.Model(&membership).Updates(updates).Error; err != nil {
			return err
		}
		userUpdates := map[string]any{"updated_at": s.now()}
		if input.Role != "" {
			userUpdates["role"] = effectiveRole
		}
		if input.Status != "" {
			userUpdates["status"] = updates["status"]
		}
		if len(userUpdates) > 1 {
			if err := tx.Model(&organization.User{}).Where("id = ?", membership.UserID).Updates(userUpdates).Error; err != nil {
				return err
			}
		}
		if input.StoreIDs != nil {
			if err := s.replaceMemberStoresTx(tx, actor, membership.UserID, effectiveRole, input.StoreIDs); err != nil {
				return err
			}
		}
		if err := tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, memberID).Preload("User").First(&membership).Error; err != nil {
			return err
		}
		return s.auditTx(tx, &actor.OrganizationID, &actor.ID, "member", &membership.UserID, action, "{}")
	})
	return membership, err
}

func (s *Service) AssignMemberStores(ctx context.Context, actor auth.CurrentUser, memberID uuid.UUID, storeIDs []uuid.UUID) ([]StoreUserAssignment, error) {
	if !actor.Role.CanManageOrganization() && actor.Role != authz.StoreManager {
		return nil, httperror.Forbidden("You cannot assign store access")
	}
	var membership organization.OrganizationMembership
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, memberID).First(&membership).Error
	if err != nil {
		return nil, mapNotFound(err, "Member not found")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.replaceMemberStoresTx(tx, actor, membership.UserID, membership.Role.String(), storeIDs); err != nil {
			return err
		}
		return s.auditTx(tx, &actor.OrganizationID, &actor.ID, "member", &membership.UserID, "store_access_granted", "{}")
	})
	if err != nil {
		return nil, err
	}
	return s.ListMemberStores(ctx, actor, memberID)
}

func (s *Service) ListMemberStores(ctx context.Context, actor auth.CurrentUser, memberID uuid.UUID) ([]StoreUserAssignment, error) {
	var membership organization.OrganizationMembership
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, memberID).First(&membership).Error; err != nil {
		return nil, mapNotFound(err, "Member not found")
	}
	var assignments []StoreUserAssignment
	err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", actor.OrganizationID, membership.UserID).Preload("Store").Find(&assignments).Error
	return assignments, err
}

func (s *Service) applyInvitationStoresTx(tx *gorm.DB, invitation organization.OrganizationInvitation, userID uuid.UUID) error {
	var storeIDs []uuid.UUID
	if strings.TrimSpace(invitation.StoreIDs) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(invitation.StoreIDs), &storeIDs); err != nil {
		return err
	}
	for _, storeID := range storeIDs {
		var count int64
		if err := tx.Model(&Store{}).Where("organization_id = ? AND id = ?", invitation.OrganizationID, storeID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return httperror.BadRequest("Invitation contains invalid store assignment")
		}
		assignment := StoreUserAssignment{ID: uuid.New(), OrganizationID: invitation.OrganizationID, StoreID: storeID, UserID: userID, Role: defaultStoreRole(invitation.Role.String())}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&assignment).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, &invitation.OrganizationID, &invitation.InvitedByUserID, "member", &userID, "store_access_granted", fmt.Sprintf(`{"store_id":%q}`, storeID)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ListAuditLogs(ctx context.Context, actor auth.CurrentUser) ([]organization.AuditLog, error) {
	if !actor.Role.CanManageOrganization() && actor.Role != authz.Viewer {
		return nil, httperror.Forbidden("You cannot view audit logs")
	}
	var logs []organization.AuditLog
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Limit(200).Find(&logs).Error
	return logs, err
}

func (s *Service) replaceMemberStoresTx(tx *gorm.DB, actor auth.CurrentUser, userID uuid.UUID, role string, storeIDs []uuid.UUID) error {
	if err := tx.Where("organization_id = ? AND user_id = ?", actor.OrganizationID, userID).Delete(&StoreUserAssignment{}).Error; err != nil {
		return err
	}
	for _, storeID := range storeIDs {
		if err := s.ensureStoreAccessibleTx(tx, actor, storeID); err != nil {
			return err
		}
		assignment := StoreUserAssignment{ID: uuid.New(), OrganizationID: actor.OrganizationID, StoreID: storeID, UserID: userID, Role: defaultStoreRole(role)}
		if err := tx.Create(&assignment).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) auditTx(tx *gorm.DB, organizationID, actorID *uuid.UUID, targetType string, targetID *uuid.UUID, action string, metadata string) error {
	log := organization.AuditLog{ID: uuid.New(), OrganizationID: organizationID, ActorUserID: actorID, TargetType: targetType, TargetID: targetID, Action: action, Metadata: jsonObject(metadata)}
	return tx.Create(&log).Error
}

func secureInvitationToken() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(bytes)
	return token, hashInvitationToken(token), nil
}

func hashInvitationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func invitationEmailText(appBaseURL string, token string) string {
	if appBaseURL == "" {
		appBaseURL = "http://localhost:3000"
	}
	return fmt.Sprintf("You have been invited to ZidiCommerce.\n\nAccept your invitation here:\n%s/invitations/accept?token=%s\n\nThis invitation expires in 7 days.", appBaseURL, token)
}

func canViewTeam(role authz.Role) bool {
	return role == authz.PlatformAdmin || role == authz.MerchantAdmin || role == authz.StoreManager || role == authz.Viewer
}

func isAssignableMerchantRole(role authz.Role) bool {
	switch role {
	case authz.MerchantAdmin, authz.StoreManager, authz.StoreStaff, authz.SupportAgent, authz.Viewer:
		return true
	default:
		return false
	}
}

func defaultStoreRole(role string) string {
	switch authz.Role(role) {
	case authz.StoreManager:
		return authz.StoreManager.String()
	default:
		return authz.StoreStaff.String()
	}
}

func onboardingState(done string) string {
	if strings.TrimSpace(done) == "" {
		return "{}"
	}
	return fmt.Sprintf(`{"%s":true}`, done)
}
