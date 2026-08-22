package fieldservice

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SeedProfile is deployment data for one field-service tenant. It deliberately
// contains no credentials: passwords and integration secrets come from the
// environment and are passed separately through SeedOptions.
type SeedProfile struct {
	OrganizationName string                `json:"organization_name"`
	OrganizationSlug string                `json:"organization_slug"`
	Owner            SeedOwnerProfile      `json:"owner"`
	WelcomeMessage   string                `json:"welcome_message"`
	BookingFeeMinor  int64                 `json:"booking_fee_minor"`
	AcceptanceWindow int                   `json:"acceptance_window_seconds"`
	MaxDistanceKM    float64               `json:"max_distance_km"`
	Weights          SeedMatchingWeights   `json:"matching_weights"`
	Pools            []string              `json:"pools"`
	Providers        []SeedProviderProfile `json:"providers"`
	Jobs             []SeedJob             `json:"jobs"`
}

type SeedOwnerProfile struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type SeedMatchingWeights struct {
	Service      float64 `json:"service"`
	Availability float64 `json:"availability"`
	Distance     float64 `json:"distance"`
	Rating       float64 `json:"rating"`
	Experience   float64 `json:"experience"`
}

type SeedProviderProfile struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Phone         string   `json:"phone"`
	WhatsApp      string   `json:"whatsapp"`
	Area          string   `json:"area"`
	Address       string   `json:"address"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	Pools         []string `json:"pools"`
	Rating        float64  `json:"rating"`
	RatingCount   int      `json:"rating_count"`
	JobsCompleted int      `json:"jobs_completed"`
	Availability  string   `json:"availability"`
}

func LoadSeedProfile(path string) (SeedProfile, error) {
	body, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return SeedProfile{}, fmt.Errorf("read field-service seed profile: %w", err)
	}
	var profile SeedProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return SeedProfile{}, fmt.Errorf("decode field-service seed profile: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return SeedProfile{}, err
	}
	return profile, nil
}

func (p SeedProfile) Validate() error {
	if strings.TrimSpace(p.OrganizationName) == "" || strings.TrimSpace(p.OrganizationSlug) == "" {
		return fmt.Errorf("field-service seed profile requires organization_name and organization_slug")
	}
	if strings.TrimSpace(p.Owner.Email) == "" {
		return fmt.Errorf("field-service seed profile requires an owner email")
	}
	if len(p.Pools) == 0 {
		return fmt.Errorf("field-service seed profile requires at least one service pool")
	}
	poolNames := map[string]bool{}
	for _, name := range p.Pools {
		name = strings.TrimSpace(name)
		if name == "" || poolNames[strings.ToLower(name)] {
			return fmt.Errorf("field-service seed profile has a blank or duplicate service pool")
		}
		poolNames[strings.ToLower(name)] = true
	}
	providerCodes := map[string]bool{}
	providerEmails := map[string]bool{}
	for _, provider := range p.Providers {
		code := strings.ToLower(strings.TrimSpace(provider.Code))
		email := strings.ToLower(strings.TrimSpace(provider.Email))
		if code == "" || strings.TrimSpace(provider.Name) == "" || email == "" {
			return fmt.Errorf("every seeded provider requires a code, name, and email")
		}
		if providerCodes[code] || providerEmails[email] {
			return fmt.Errorf("field-service seed profile has a duplicate provider code or email")
		}
		providerCodes[code], providerEmails[email] = true, true
		if provider.Rating < 0 || provider.Rating > 5 || provider.RatingCount < 0 || provider.JobsCompleted < 0 {
			return fmt.Errorf("provider %s has invalid rating or experience data", provider.Name)
		}
		for _, pool := range provider.Pools {
			if !poolNames[strings.ToLower(strings.TrimSpace(pool))] {
				return fmt.Errorf("provider %s references unknown pool %s", provider.Name, pool)
			}
		}
	}
	for _, job := range p.Jobs {
		if !poolNames[strings.ToLower(strings.TrimSpace(job.Pool))] {
			return fmt.Errorf("seed job references unknown pool %s", job.Pool)
		}
	}
	return nil
}

func (p SeedProfile) settings(portalURL string) Settings {
	weights := p.Weights
	if weights.Service+weights.Availability+weights.Distance+weights.Rating+weights.Experience == 0 {
		weights = SeedMatchingWeights{Service: 0.40, Availability: 0.20, Distance: 0.20, Rating: 0.15, Experience: 0.05}
	}
	fee := p.BookingFeeMinor
	if fee <= 0 {
		fee = 500000
	}
	window := p.AcceptanceWindow
	if window <= 0 {
		window = 120
	}
	distance := p.MaxDistanceKM
	if distance <= 0 {
		distance = 25
	}
	return Settings{
		BookingFeeMinor: fee, Currency: "NGN", RequireBookingFee: true, BookingFeeRefundable: false,
		DispatchStrategy: "sequential", AcceptanceWindowSeconds: window, MaxDistanceKM: distance,
		WeightService: weights.Service, WeightAvailability: weights.Availability, WeightDistance: weights.Distance,
		WeightRating: weights.Rating, WeightExperience: weights.Experience,
		CompanyDisplayName: p.OrganizationName, WelcomeMessage: p.WelcomeMessage, ProviderPortalBaseURL: portalURL,
	}
}
