package channelplatform

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ProviderSupportAvailable      = "available"
	ProviderSupportReviewRequired = "review_required"
	ProviderSupportRestricted     = "restricted"
	ProviderSupportUnavailable    = "unavailable"
	ProviderSupportUnverified     = "unverified"
)

type capabilityDefinition struct {
	Capability     string
	Support        string
	RequiredScopes []string
	Reason         string
}

type providerDefinition struct {
	Provider     string
	DisplayName  string
	Capabilities []capabilityDefinition
}

var providerCatalog = []providerDefinition{
	{Provider: "whatsapp", DisplayName: "WhatsApp", Capabilities: []capabilityDefinition{
		{Capability: "oauth_onboarding", Support: ProviderSupportAvailable},
		{Capability: "inbound_text", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "outbound_text", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "inbound_media", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "outbound_media", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "interactive_messages", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "templates", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "read_receipts", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "delivery_receipts", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_messaging"}},
		{Capability: "profile_read", Support: ProviderSupportAvailable, RequiredScopes: []string{"whatsapp_business_management"}},
		{Capability: "commerce_actions", Support: ProviderSupportAvailable},
		{Capability: "human_handoff", Support: ProviderSupportAvailable},
	}},
	{Provider: "instagram", DisplayName: "Instagram", Capabilities: []capabilityDefinition{
		{Capability: "oauth_onboarding", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_basic"}},
		{Capability: "profile_read", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_basic"}},
		{Capability: "inbound_text", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "outbound_text", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "inbound_media", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "outbound_media", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "interactive_messages", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "message_reactions", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "read_receipts", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "comments", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_comments"}},
		{Capability: "content_publishing", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_content_publish"}},
		{Capability: "commerce_actions", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
		{Capability: "human_handoff", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"instagram_business_manage_messages"}},
	}},
	{Provider: "facebook", DisplayName: "Facebook Messenger", Capabilities: []capabilityDefinition{
		{Capability: "oauth_onboarding", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_show_list"}},
		{Capability: "profile_read", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_show_list", "pages_read_engagement"}},
		{Capability: "inbound_text", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging", "pages_manage_metadata"}},
		{Capability: "outbound_text", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "inbound_media", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "outbound_media", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "interactive_messages", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "message_reactions", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "read_receipts", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "delivery_receipts", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "commerce_actions", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
		{Capability: "human_handoff", Support: ProviderSupportReviewRequired, RequiredScopes: []string{"pages_messaging"}},
	}},
	{Provider: "tiktok", DisplayName: "TikTok", Capabilities: []capabilityDefinition{
		{Capability: "oauth_onboarding", Support: ProviderSupportUnverified, RequiredScopes: []string{"user.info.basic"}, Reason: "Zidi developer-app access is not yet verified"},
		{Capability: "profile_read", Support: ProviderSupportUnverified, RequiredScopes: []string{"user.info.basic"}, Reason: "Zidi developer-app access is not yet verified"},
		{Capability: "content_publishing", Support: ProviderSupportUnverified, RequiredScopes: []string{"video.publish"}, Reason: "Content Posting API approval is not yet verified"},
		{Capability: "inbound_text", Support: ProviderSupportUnavailable, Reason: "Messaging unavailable through the current TikTok API"},
		{Capability: "outbound_text", Support: ProviderSupportUnavailable, Reason: "Messaging unavailable through the current TikTok API"},
		{Capability: "inbound_media", Support: ProviderSupportUnavailable, Reason: "Messaging unavailable through the current TikTok API"},
		{Capability: "outbound_media", Support: ProviderSupportUnavailable, Reason: "Messaging unavailable through the current TikTok API"},
		{Capability: "commerce_actions", Support: ProviderSupportUnavailable, Reason: "Customer conversations are not exposed by the current TikTok API"},
		{Capability: "human_handoff", Support: ProviderSupportUnavailable, Reason: "Customer conversations are not exposed by the current TikTok API"},
	}},
}

type CapabilityResolution struct {
	GrantedScopes  []string
	AssetReady     bool
	ReviewApproved bool
	VerifiedAt     time.Time
}

func ProviderCatalog() []ProviderDefinitionView {
	views := make([]ProviderDefinitionView, 0, len(providerCatalog))
	for _, provider := range providerCatalog {
		view := ProviderDefinitionView{Provider: provider.Provider, DisplayName: provider.DisplayName, Capabilities: make([]ProviderCapabilityView, 0, len(provider.Capabilities))}
		for _, capability := range provider.Capabilities {
			view.Capabilities = append(view.Capabilities, ProviderCapabilityView{Capability: capability.Capability, Support: capability.Support, RequiredScopes: append([]string{}, capability.RequiredScopes...), Reason: capability.Reason})
		}
		views = append(views, view)
	}
	return views
}

func definitionsForProvider(provider string) []capabilityDefinition {
	provider = normalizeKey(provider)
	for _, definition := range providerCatalog {
		if definition.Provider == provider {
			return definition.Capabilities
		}
	}
	return nil
}

func (s *Service) seedConnectionCapabilities(tx *gorm.DB, connection ChannelConnection) error {
	now := s.now()
	for _, definition := range definitionsForProvider(connection.Provider) {
		status, reason := initialCapabilityStatus(definition)
		row := ChannelCapability{ID: uuid.New(), OrganizationID: connection.OrganizationID, ConnectionID: connection.ID, Provider: connection.Provider, Capability: definition.Capability, Status: status, Reason: reason, RequiredScopes: jsonValue(definition.RequiredScopes), GrantedScopes: "[]", CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "capability"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func initialCapabilityStatus(definition capabilityDefinition) (string, string) {
	switch definition.Support {
	case ProviderSupportReviewRequired:
		return CapabilityAwaitingPermissionReview, "Provider review or advanced access is required"
	case ProviderSupportRestricted:
		return CapabilityRestricted, definition.Reason
	case ProviderSupportUnavailable:
		return CapabilityUnsupported, definition.Reason
	case ProviderSupportUnverified:
		return CapabilityUnverified, definition.Reason
	default:
		return CapabilitySetupRequired, "Complete provider onboarding and asset verification"
	}
}

func (s *Service) ResolveConnectionCapabilities(ctx context.Context, organizationID, connectionID uuid.UUID, resolution CapabilityResolution) error {
	connection, err := s.findConnection(ctx, organizationID, connectionID)
	if err != nil {
		return err
	}
	granted := normalizedSet(resolution.GrantedScopes)
	verifiedAt := resolution.VerifiedAt.UTC()
	if verifiedAt.IsZero() {
		verifiedAt = s.now()
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.seedConnectionCapabilities(tx, connection); err != nil {
			return err
		}
		for _, definition := range definitionsForProvider(connection.Provider) {
			status, reason := initialCapabilityStatus(definition)
			if definition.Support == ProviderSupportAvailable || (definition.Support == ProviderSupportReviewRequired && resolution.ReviewApproved) || (definition.Support == ProviderSupportUnverified && resolution.ReviewApproved) {
				missing := missingValues(definition.RequiredScopes, granted)
				switch {
				case len(missing) > 0:
					status, reason = CapabilityMissingPermission, "Required provider permission was not granted"
				case !resolution.AssetReady:
					status, reason = CapabilitySetupRequired, "Select and verify an eligible provider asset"
				default:
					status, reason = CapabilityAvailable, ""
				}
			}
			if err := tx.Model(&ChannelCapability{}).Where("organization_id = ? AND channel_connection_id = ? AND capability = ?", organizationID, connectionID, definition.Capability).Updates(map[string]any{"status": status, "reason": reason, "granted_scopes": jsonValue(sortedKeys(granted)), "verified_at": verifiedAt, "updated_at": s.now()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) capabilityViews(ctx context.Context, organizationID, connectionID uuid.UUID) ([]CapabilityView, error) {
	var rows []ChannelCapability
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ?", organizationID, connectionID).Order("capability ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]CapabilityView, 0, len(rows))
	for _, row := range rows {
		required := stringList(row.RequiredScopes)
		granted := stringList(row.GrantedScopes)
		views = append(views, CapabilityView{Capability: row.Capability, Status: row.Status, Reason: row.Reason, RequiredScopes: required, GrantedScopes: granted, MissingScopes: missingValues(required, normalizedSet(granted)), VerifiedAt: row.VerifiedAt})
	}
	return views, nil
}

func (s *Service) RequireCapability(ctx context.Context, organizationID, connectionID uuid.UUID, capability string) error {
	capability = normalizeKey(capability)
	var row ChannelCapability
	err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND capability = ?", organizationID, connectionID, capability).First(&row).Error
	if err == nil {
		if row.Status == CapabilityAvailable {
			return nil
		}
		return httperror.BadRequest("This channel connection does not currently support " + strings.ReplaceAll(capability, "_", " "))
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	connection, findErr := s.findConnection(ctx, organizationID, connectionID)
	if findErr != nil {
		return findErr
	}
	legacy := normalizedSet(stringList(connection.Capabilities))
	legacyCapability := map[string]string{"outbound_text": "outbound_messages", "outbound_media": "media", "interactive_messages": "outbound_messages"}[capability]
	if legacy[capability] || (legacyCapability != "" && legacy[legacyCapability]) {
		return nil
	}
	return httperror.BadRequest("This channel connection does not support " + strings.ReplaceAll(capability, "_", " "))
}

func normalizedSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		if value = normalizeKey(value); value != "" {
			result[value] = true
		}
	}
	return result
}

func missingValues(required []string, granted map[string]bool) []string {
	missing := []string{}
	for _, value := range required {
		if !granted[normalizeKey(value)] {
			missing = append(missing, value)
		}
	}
	sort.Strings(missing)
	return missing
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
