package fieldservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	DemoOrgSlug     = "lagos-home-services"
	DemoOwnerEmail  = "owner@lagoshome.demo"
	DemoPassword    = "HomeServices1!"
	DemoCompanyName = "Lagos Home Services"
)

type SeedResult struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	OwnerEmail     string    `json:"owner_email"`
	ProviderEmail  string    `json:"sample_provider_email"`
	ProviderCount  int       `json:"provider_count"`
	PoolCount      int       `json:"pool_count"`
	BotID          uuid.UUID `json:"bot_id"`
	BotStatus      string    `json:"bot_status"`
	ChannelStatus  string    `json:"whatsapp_channel_status"`
	Password       string    `json:"-"`
}

// SeedOptions lets a deployment override the demo credentials and the provider
// portal link that goes out in dispatch notifications. An empty Password falls
// back to the local development default, which must not be used in production.
type SeedOptions struct {
	Password          string
	ProviderPortalURL string
}

func (o SeedOptions) password() string {
	if strings.TrimSpace(o.Password) != "" {
		return strings.TrimSpace(o.Password)
	}
	return DemoPassword
}

func (o SeedOptions) portalURL() string {
	if strings.TrimSpace(o.ProviderPortalURL) != "" {
		return strings.TrimRight(strings.TrimSpace(o.ProviderPortalURL), "/")
	}
	return "http://localhost:3010"
}

func SeedDemo(ctx context.Context, db *gorm.DB, commerce *core.Service, bots *bot.Service, field *Service, options SeedOptions) (SeedResult, error) {
	return SeedTenant(ctx, db, commerce, bots, field, defaultDemoProfile(), options)
}

// SeedTenant provisions one organization from a data profile. All records are
// matched by tenant-scoped natural keys and are created through the existing
// commerce, bot, and field-service boundaries.
func SeedTenant(ctx context.Context, db *gorm.DB, commerce *core.Service, bots *bot.Service, field *Service, profile SeedProfile, options SeedOptions) (SeedResult, error) {
	if err := profile.Validate(); err != nil {
		return SeedResult{}, err
	}
	password := options.password()
	var org organization.Organization
	err := db.WithContext(ctx).Where("slug = ?", profile.OrganizationSlug).First(&org).Error
	if err == gorm.ErrRecordNotFound {
		org = organization.Organization{
			ID: uuid.New(), Name: profile.OrganizationName, Slug: profile.OrganizationSlug, Currency: "NGN", Timezone: "Africa/Lagos",
			Country: "NG", Status: "active", OnboardingState: `{"complete":true}`, Metadata: fieldServiceOrganizationMetadata(""),
		}
		if err := db.WithContext(ctx).Create(&org).Error; err != nil {
			return SeedResult{}, err
		}
	} else if err != nil {
		return SeedResult{}, err
	} else if err := db.WithContext(ctx).Model(&organization.Organization{}).Where("id = ?", org.ID).Updates(map[string]any{
		"name": profile.OrganizationName, "status": "active", "currency": "NGN", "timezone": "Africa/Lagos", "country": "NG",
		"metadata": fieldServiceOrganizationMetadata(org.Metadata),
	}).Error; err != nil {
		return SeedResult{}, err
	}
	actor := auth.CurrentUser{OrganizationID: org.ID, Role: authz.MerchantAdmin}
	ownerID, err := upsertUser(ctx, db, org.ID, profile.Owner.Email, profile.Owner.FirstName, profile.Owner.LastName, authz.MerchantAdmin, true, password)
	if err != nil {
		return SeedResult{}, err
	}
	actor.ID = ownerID

	if _, err := field.UpdateSettings(ctx, actor, profile.settings(options.portalURL())); err != nil {
		return SeedResult{}, err
	}

	pools := map[string]Pool{}
	for index, name := range profile.Pools {
		pool, err := upsertPool(ctx, db, org.ID, name, index)
		if err != nil {
			return SeedResult{}, err
		}
		pools[strings.ToLower(name)] = pool
	}

	var sampleEmail string
	for _, row := range profile.Providers {
		email := strings.ToLower(strings.TrimSpace(row.Email))
		if sampleEmail == "" {
			sampleEmail = email
		}
		userID, err := upsertUser(ctx, db, org.ID, email, row.Name, "", authz.ServiceProvider, false, password)
		if err != nil {
			return SeedResult{}, err
		}
		lat, lng := row.Latitude, row.Longitude
		metadata, _ := json.Marshal(map[string]any{"rating_count": row.RatingCount, "seed_profile": profile.OrganizationSlug})
		provider := Provider{
			UserID: &userID, PublicCode: row.Code, Name: row.Name, Phone: row.Phone, WhatsAppNumber: firstNonEmpty(row.WhatsApp, row.Phone),
			Area: row.Area, Address: firstNonEmpty(row.Address, row.Area+", Lagos"), Latitude: &lat, Longitude: &lng,
			RatingAverage: row.Rating, JobsCompleted: row.JobsCompleted, Availability: row.Availability, Status: "active", Metadata: string(metadata),
		}
		var poolIDs []uuid.UUID
		for _, name := range row.Pools {
			poolIDs = append(poolIDs, pools[strings.ToLower(name)].ID)
		}
		existing, err := findProvider(ctx, db, org.ID, row.Code)
		if err == nil {
			provider.ID = existing.ID
		}
		saved, err := field.UpsertProvider(ctx, actor, provider, poolIDs, "")
		if err != nil {
			return SeedResult{}, err
		}
		if err := db.WithContext(ctx).Model(&Provider{}).Where("organization_id = ? AND id = ?", org.ID, saved.ID).Updates(map[string]any{
			"user_id": userID, "rating_average": row.Rating, "jobs_completed": row.JobsCompleted, "metadata": string(metadata),
		}).Error; err != nil {
			return SeedResult{}, err
		}
	}

	if _, err := commerce.ListStores(ctx, actor); err != nil {
		return SeedResult{}, err
	}
	stores, _ := commerce.ListStores(ctx, actor)
	if len(stores) == 0 {
		_, err := commerce.CreateStore(ctx, actor, core.StoreInput{Name: profile.OrganizationName + " operations", Code: "FIELD", City: "Lagos", Country: "NG", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
		if err != nil {
			return SeedResult{}, err
		}
	}
	if _, _, err := field.ensureFeeCatalog(ctx, actor, "NGN"); err != nil {
		return SeedResult{}, err
	}

	serviceBot, err := ensurePublishedServiceBot(ctx, db, bots, actor, profile)
	if err != nil {
		return SeedResult{}, err
	}

	if err := seedWhatsAppChannel(ctx, db, commerce, actor, profile.OrganizationName, serviceBot.ID); err != nil {
		return SeedResult{}, err
	}
	if err := SeedLifecycleJobs(ctx, db, commerce, field, actor, profile.Jobs); err != nil {
		return SeedResult{}, err
	}

	var channel core.Channel
	_ = db.WithContext(ctx).Where("organization_id = ? AND provider = ?", org.ID, "whatsapp").First(&channel).Error
	return SeedResult{
		OrganizationID: org.ID, OwnerEmail: profile.Owner.Email, ProviderEmail: sampleEmail, Password: password,
		ProviderCount: len(profile.Providers), PoolCount: len(profile.Pools), BotID: serviceBot.ID, BotStatus: serviceBot.Status, ChannelStatus: channel.Status,
	}, nil
}

func fieldServiceOrganizationMetadata(existing string) string {
	metadata := map[string]any{}
	_ = json.Unmarshal([]byte(existing), &metadata)
	metadata["seed"] = "field_service"
	metadata["use_case"] = "handyman"
	body, _ := json.Marshal(metadata)
	return string(body)
}

func ensurePublishedServiceBot(ctx context.Context, db *gorm.DB, bots *bot.Service, actor auth.CurrentUser, profile SeedProfile) (bot.Bot, error) {
	var existing []bot.Bot
	if err := db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Find(&existing).Error; err != nil {
		return bot.Bot{}, err
	}
	var serviceBot bot.Bot
	for _, candidate := range existing {
		if strings.Contains(candidate.Metadata, "field_service") {
			serviceBot = candidate
			break
		}
	}
	if serviceBot.ID == uuid.Nil {
		created, err := bots.CreateServiceBookingBot(ctx, actor, profile.OrganizationName+" assistant", profile.WelcomeMessage)
		if err != nil {
			return bot.Bot{}, err
		}
		serviceBot = created
	}
	if serviceBot.PublishedVersionID == nil || serviceBot.Status != bot.BotStatusActive {
		versions, err := bots.ListVersions(ctx, actor, serviceBot.ID)
		if err != nil || len(versions) == 0 {
			return bot.Bot{}, fmt.Errorf("service bot version missing")
		}
		validated, err := bots.ValidateVersion(ctx, actor, versions[0].ID)
		if err != nil {
			return bot.Bot{}, err
		}
		if _, err := bots.PublishVersion(ctx, actor, versions[0].ID); err != nil {
			return bot.Bot{}, fmt.Errorf("publish service bot: %w (issues: %+v)", err, validated.Issues)
		}
		if err := db.WithContext(ctx).Where("id = ? AND organization_id = ?", serviceBot.ID, actor.OrganizationID).First(&serviceBot).Error; err != nil {
			return bot.Bot{}, err
		}
	}
	return serviceBot, nil
}

func upsertPool(ctx context.Context, db *gorm.DB, orgID uuid.UUID, name string, order int) (Pool, error) {
	slug := slugify("", name)
	var pool Pool
	err := db.WithContext(ctx).Where("organization_id = ? AND slug = ?", orgID, slug).First(&pool).Error
	if err == nil {
		if err := db.WithContext(ctx).Model(&Pool{}).Where("organization_id = ? AND id = ?", orgID, pool.ID).Updates(map[string]any{
			"name": strings.TrimSpace(name), "status": "active", "sort_order": order,
		}).Error; err != nil {
			return Pool{}, err
		}
		return pool, db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, pool.ID).First(&pool).Error
	}
	if err != gorm.ErrRecordNotFound {
		return Pool{}, err
	}
	pool = Pool{ID: uuid.New(), OrganizationID: orgID, Slug: slug, Name: name, Status: "active", SortOrder: order, Metadata: "{}"}
	return pool, db.WithContext(ctx).Create(&pool).Error
}

func findProvider(ctx context.Context, db *gorm.DB, orgID uuid.UUID, code string) (Provider, error) {
	var provider Provider
	err := db.WithContext(ctx).Where("organization_id = ? AND public_code = ?", orgID, code).First(&provider).Error
	return provider, err
}

func upsertUser(ctx context.Context, db *gorm.DB, orgID uuid.UUID, email, first, last string, role authz.Role, owner bool, password string) (uuid.UUID, error) {
	email = strings.ToLower(email)
	var user organization.User
	err := db.WithContext(ctx).Where("lower(email) = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return uuid.Nil, hashErr
		}
		user = organization.User{ID: uuid.New(), OrganizationID: &orgID, Email: email, FirstName: first, LastName: last, PasswordHash: string(hash), Role: role, Status: "active"}
		if err := db.WithContext(ctx).Create(&user).Error; err != nil {
			return uuid.Nil, err
		}
	} else if err != nil {
		return uuid.Nil, err
	} else {
		if user.OrganizationID != nil && *user.OrganizationID != orgID {
			return uuid.Nil, fmt.Errorf("user %s already belongs to another organization", email)
		}
		updates := map[string]any{"organization_id": orgID, "first_name": first, "last_name": last, "role": role, "status": "active"}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if hashErr != nil {
				return uuid.Nil, hashErr
			}
			updates["password_hash"] = string(hash)
		}
		if err := db.WithContext(ctx).Model(&organization.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return uuid.Nil, err
		}
	}
	membership := organization.OrganizationMembership{ID: uuid.New(), OrganizationID: orgID, UserID: user.ID, Role: role, Status: "active", IsOwner: owner}
	var existing organization.OrganizationMembership
	if err := db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", orgID, user.ID).First(&existing).Error; err == gorm.ErrRecordNotFound {
		if err := db.WithContext(ctx).Create(&membership).Error; err != nil {
			return uuid.Nil, err
		}
	} else if err != nil {
		return uuid.Nil, err
	} else if err := db.WithContext(ctx).Model(&organization.OrganizationMembership{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"role": role, "status": "active", "is_owner": owner,
	}).Error; err != nil {
		return uuid.Nil, err
	}
	return user.ID, nil
}
