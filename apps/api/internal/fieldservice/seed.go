package fieldservice

import (
	"context"
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
	password := options.password()
	var org organization.Organization
	err := db.WithContext(ctx).Where("slug = ?", DemoOrgSlug).First(&org).Error
	if err == gorm.ErrRecordNotFound {
		org = organization.Organization{
			ID: uuid.New(), Name: DemoCompanyName, Slug: DemoOrgSlug, Currency: "NGN", Timezone: "Africa/Lagos",
			Country: "NG", Status: "active", OnboardingState: `{"complete":true}`, Metadata: `{"demo":"field_service"}`,
		}
		if err := db.WithContext(ctx).Create(&org).Error; err != nil {
			return SeedResult{}, err
		}
	} else if err != nil {
		return SeedResult{}, err
	}
	actor := auth.CurrentUser{OrganizationID: org.ID, Role: authz.MerchantAdmin}
	ownerID, err := upsertUser(ctx, db, org.ID, DemoOwnerEmail, "Ada", "Okoye", authz.MerchantAdmin, true, password)
	if err != nil {
		return SeedResult{}, err
	}
	actor.ID = ownerID

	_, _ = field.UpdateSettings(ctx, actor, Settings{
		BookingFeeMinor: 500000, Currency: "NGN", RequireBookingFee: true, BookingFeeRefundable: false,
		DispatchStrategy: "sequential", AcceptanceWindowSeconds: 120, MaxDistanceKM: 25,
		WeightService: 0.40, WeightAvailability: 0.20, WeightDistance: 0.20, WeightRating: 0.15, WeightExperience: 0.05,
		CompanyDisplayName:    DemoCompanyName,
		WelcomeMessage:        "Hi 👋 Welcome to Lagos Home Services.\nWe help you find trusted professionals for home services across Lagos.\n\nWhat service do you need today?",
		ProviderPortalBaseURL: options.portalURL(),
	})

	poolNames := []string{"Plumber", "Electrician", "Carpenter", "Painter", "AC Technician", "Appliance Repair", "General Handyman", "Other"}
	pools := map[string]Pool{}
	for index, name := range poolNames {
		pool, err := upsertPool(ctx, db, org.ID, name, index)
		if err != nil {
			return SeedResult{}, err
		}
		pools[name] = pool
	}

	type seedProvider struct {
		Code, Name, Area string
		Lat, Lng         float64
		Pools            []string
		Rating           float64
		Jobs             int
		Availability     string
	}
	directory := []seedProvider{
		{"john-lekki", "John Adeyemi", "Lekki", 6.4474, 3.4723, []string{"Plumber", "AC Technician"}, 4.8, 42, AvailabilityAvailable},
		{"michael-lekki", "Michael Okonkwo", "Lekki", 6.4490, 3.4800, []string{"Electrician"}, 4.6, 28, AvailabilityAvailable},
		{"samuel-ajah", "Samuel Bello", "Ajah", 6.4698, 3.5646, []string{"Plumber"}, 4.4, 19, AvailabilityAvailable},
		{"david-vi", "David Eze", "Victoria Island", 6.4281, 3.4219, []string{"Electrician", "Appliance Repair"}, 4.9, 61, AvailabilityAvailable},
		{"chidi-ikoyi", "Chidi Nwosu", "Ikoyi", 6.4541, 3.4358, []string{"Painter"}, 4.2, 15, AvailabilityAvailable},
		{"tunde-yaba", "Tunde Balogun", "Yaba", 6.5095, 3.3711, []string{"Carpenter", "General Handyman"}, 4.7, 33, AvailabilityBusy},
		{"amina-surulere", "Amina Lawal", "Surulere", 6.5000, 3.3500, []string{"Appliance Repair"}, 4.5, 24, AvailabilityAvailable},
		{"ibrahim-ikeja", "Ibrahim Musa", "Ikeja", 6.6018, 3.3515, []string{"Plumber", "General Handyman"}, 4.3, 18, AvailabilityAvailable},
		{"grace-maryland", "Grace Umeh", "Maryland", 6.5764, 3.3682, []string{"AC Technician"}, 4.8, 47, AvailabilityAvailable},
		{"peter-gbagada", "Peter Obioma", "Gbagada", 6.5447, 3.3892, []string{"Electrician"}, 3.9, 9, AvailabilityOffline},
		{"nkechi-magodo", "Nkechi Okafor", "Magodo", 6.6183, 3.3823, []string{"Painter", "Carpenter"}, 4.6, 22, AvailabilityAvailable},
		{"femi-mainland", "Femi Adebayo", "Lagos Mainland", 6.5080, 3.3710, []string{"Plumber"}, 4.1, 12, AvailabilityAvailable},
		{"halima-island", "Halima Sule", "Lagos Island", 6.4549, 3.3947, []string{"AC Technician", "Electrician"}, 4.7, 38, AvailabilityAvailable},
		{"kunle-lekki", "Kunle Ajayi", "Lekki", 6.4410, 3.4680, []string{"Carpenter"}, 4.0, 8, AvailabilityAvailable},
		{"blessing-ajah", "Blessing Etuk", "Ajah", 6.4720, 3.5710, []string{"Appliance Repair", "Other"}, 4.4, 16, AvailabilityAvailable},
		{"emeka-ikeja", "Emeka Chukwu", "Ikeja", 6.6050, 3.3480, []string{"Electrician", "AC Technician"}, 4.9, 72, AvailabilityBusy},
		{"zainab-yaba", "Zainab Abdullahi", "Yaba", 6.5120, 3.3770, []string{"Painter"}, 4.3, 14, AvailabilityAvailable},
		{"segun-surulere", "Segun Adeleke", "Surulere", 6.4960, 3.3550, []string{"General Handyman", "Plumber"}, 4.5, 29, AvailabilityAvailable},
		{"rita-gbagada", "Rita Mensah", "Gbagada", 6.5480, 3.3930, []string{"Other", "Appliance Repair"}, 4.2, 11, AvailabilityAvailable},
		{"daniel-ikoyi", "Daniel Wright", "Ikoyi", 6.4500, 3.4310, []string{"Plumber", "Electrician"}, 4.8, 54, AvailabilityAvailable},
	}
	var sampleEmail string
	for _, row := range directory {
		email := row.Code + "@providers.zidicommerce.local"
		if sampleEmail == "" {
			sampleEmail = email
		}
		lat, lng := row.Lat, row.Lng
		provider := Provider{
			PublicCode: row.Code, Name: row.Name, Phone: "+2348000000000", WhatsAppNumber: "+2348000000000",
			Area: row.Area, Address: row.Area + ", Lagos", Latitude: &lat, Longitude: &lng,
			RatingAverage: row.Rating, JobsCompleted: row.Jobs, Availability: row.Availability, Status: "active",
		}
		var poolIDs []uuid.UUID
		for _, name := range row.Pools {
			poolIDs = append(poolIDs, pools[name].ID)
		}
		existing, err := findProvider(ctx, db, org.ID, row.Code)
		if err == nil {
			provider.ID = existing.ID
		}
		if _, err := field.UpsertProvider(ctx, actor, provider, poolIDs, password); err != nil {
			return SeedResult{}, err
		}
		_ = email
	}

	if _, err := commerce.ListStores(ctx, actor); err != nil {
		return SeedResult{}, err
	}
	stores, _ := commerce.ListStores(ctx, actor)
	if len(stores) == 0 {
		_, err := commerce.CreateStore(ctx, actor, core.StoreInput{Name: "Field operations", Code: "FIELD", City: "Lagos", Country: "NG", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: core.FulfilmentPickup, Enabled: true}}})
		if err != nil {
			return SeedResult{}, err
		}
	}
	if _, _, err := field.ensureFeeCatalog(ctx, actor, "NGN"); err != nil {
		return SeedResult{}, err
	}

	// metadata is JSONB, so it is filtered in Go rather than with a SQL LIKE,
	// which Postgres rejects against a jsonb column.
	var existingBots []bot.Bot
	if err := db.WithContext(ctx).Where("organization_id = ?", org.ID).Find(&existingBots).Error; err != nil {
		return SeedResult{}, err
	}
	hasServiceBot := false
	for _, candidate := range existingBots {
		if strings.Contains(candidate.Metadata, "field_service") {
			hasServiceBot = true
			break
		}
	}
	if !hasServiceBot {
		created, err := bots.CreateServiceBookingBot(ctx, actor, DemoCompanyName+" assistant", "")
		if err != nil {
			return SeedResult{}, err
		}
		versions, err := bots.ListVersions(ctx, actor, created.ID)
		if err != nil || len(versions) == 0 {
			return SeedResult{}, fmt.Errorf("service bot version missing")
		}
		validated, err := bots.ValidateVersion(ctx, actor, versions[0].ID)
		if err != nil {
			return SeedResult{}, err
		}
		if _, err := bots.PublishVersion(ctx, actor, versions[0].ID); err != nil {
			return SeedResult{}, fmt.Errorf("publish service bot: %w (issues: %+v)", err, validated.Issues)
		}
	}

	if err := seedWhatsAppChannel(ctx, db, commerce, actor); err != nil {
		return SeedResult{}, err
	}
	if err := SeedLifecycle(ctx, db, commerce, field, actor); err != nil {
		return SeedResult{}, err
	}

	return SeedResult{OrganizationID: org.ID, OwnerEmail: DemoOwnerEmail, ProviderEmail: sampleEmail, Password: password}, nil
}

func upsertPool(ctx context.Context, db *gorm.DB, orgID uuid.UUID, name string, order int) (Pool, error) {
	slug := slugify("", name)
	var pool Pool
	err := db.WithContext(ctx).Where("organization_id = ? AND slug = ?", orgID, slug).First(&pool).Error
	if err == nil {
		return pool, nil
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
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return uuid.Nil, err
	}
	var user organization.User
	err = db.WithContext(ctx).Where("lower(email) = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		user = organization.User{ID: uuid.New(), OrganizationID: &orgID, Email: email, FirstName: first, LastName: last, PasswordHash: string(hash), Role: role, Status: "active"}
		if err := db.WithContext(ctx).Create(&user).Error; err != nil {
			return uuid.Nil, err
		}
	}
	membership := organization.OrganizationMembership{ID: uuid.New(), OrganizationID: orgID, UserID: user.ID, Role: role, Status: "active", IsOwner: owner}
	if err := db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", orgID, user.ID).First(&organization.OrganizationMembership{}).Error; err == gorm.ErrRecordNotFound {
		if err := db.WithContext(ctx).Create(&membership).Error; err != nil {
			return uuid.Nil, err
		}
	}
	return user.ID, nil
}
