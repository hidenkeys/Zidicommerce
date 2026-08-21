package fieldservice

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
)

// seedCustomer is one demo household.
type seedCustomer struct {
	Name    string
	Phone   string
	Area    string
	Address string
}

// seedJob describes a demo request and how far along its lifecycle it should be
// driven. The seeder drives each one through the real service methods rather
// than inserting rows, so the demo data is only ever in states the running
// system can actually produce.
type seedJob struct {
	Customer    seedCustomer
	Pool        string
	Description string
	PreferredAt string
	// Stage is how far to take the job: awaiting_fee, dispatching, assigned,
	// in_progress, quote_sent, or completed.
	Stage       string
	LabourMinor int64
	MaterialMin int64
	QuoteNotes  string
	Rating      int
	Feedback    string
	Chat        []seedChatLine
}

type seedChatLine struct {
	From string // "customer" or "provider"
	Body string
}

func demoCustomers() []seedCustomer {
	return []seedCustomer{
		{"Amaka Obi", "+2348031234567", "Lekki", "12 Admiralty Way, Lekki Phase 1"},
		{"Tobi Adeniran", "+2348051112233", "Ikeja", "7 Isaac John Street, Ikeja GRA"},
		{"Chioma Nwankwo", "+2348097654321", "Yaba", "34 Herbert Macaulay Way, Yaba"},
		{"Bayo Salami", "+2348023334455", "Victoria Island", "1 Adeola Odeku Street, Victoria Island"},
		{"Ngozi Eze", "+2348065556677", "Surulere", "22 Bode Thomas Street, Surulere"},
		{"Uche Okafor", "+2348078889900", "Gbagada", "5 Diya Street, Gbagada Phase 2"},
	}
}

func demoJobs() []seedJob {
	customers := demoCustomers()
	return []seedJob{
		{
			Customer: customers[0], Pool: "Plumber",
			Description: "Leaking pipe under the kitchen sink, water pooling in the cabinet",
			PreferredAt: "today, before 6pm", Stage: "completed",
			LabourMinor: 2000000, MaterialMin: 1250000,
			QuoteNotes: "Replace the trap and both flexible hoses. Parts included.",
			Rating:     5, Feedback: "Arrived early and cleaned up afterwards.",
			Chat: []seedChatLine{
				{"provider", "On my way, should be with you in about 30 minutes."},
				{"customer", "Thank you, the gate is open."},
				{"provider", "The trap is cracked. Sending you a quote now."},
			},
		},
		{
			Customer: customers[5], Pool: "Appliance Repair",
			Description: "Washing machine fills with water but will not spin",
			PreferredAt: "tomorrow morning", Stage: "completed",
			LabourMinor: 1500000, MaterialMin: 800000,
			QuoteNotes: "Drive belt replacement and drum bearing service.",
			Rating:     4, Feedback: "Fixed it, took a little longer than expected.",
			Chat: []seedChatLine{
				{"provider", "Is the machine front loading or top loading?"},
				{"customer", "Front loading, an LG."},
			},
		},
		{
			Customer: customers[1], Pool: "Electrician",
			Description: "Sockets in the living room stopped working after a power surge",
			PreferredAt: "today", Stage: "quote_sent",
			LabourMinor: 1800000, MaterialMin: 1450000,
			QuoteNotes: "Replace the tripped ring circuit breaker and two burnt sockets.",
			Chat: []seedChatLine{
				{"provider", "Are the lights on the same circuit still working?"},
				{"customer", "Lights are fine, only the sockets are dead."},
			},
		},
		{
			Customer: customers[2], Pool: "AC Technician",
			Description: "Bedroom AC is not cooling and makes a rattling noise",
			PreferredAt: "this evening", Stage: "in_progress",
			Chat: []seedChatLine{
				{"provider", "I have arrived, checking the outdoor unit now."},
			},
		},
		{
			Customer: customers[3], Pool: "Carpenter",
			Description: "Wardrobe door came off its hinges and will not close",
			PreferredAt: "any time this week", Stage: "dispatching",
		},
		{
			Customer: customers[4], Pool: "Painter",
			Description: "Repaint two bedrooms, walls already prepared",
			PreferredAt: "next Saturday", Stage: "awaiting_fee",
		},
	}
}

// SeedLifecycle creates the sample customers, requests, conversations, quotes
// and ratings that make the owner dashboard meaningful on a fresh install. It
// is safe to run repeatedly: a job whose customer, service and description
// already exist is skipped rather than duplicated.
func SeedLifecycle(ctx context.Context, db *gorm.DB, commerce *core.Service, field *Service, actor auth.CurrentUser) error {
	pools, err := field.ListPools(ctx, actor)
	if err != nil {
		return err
	}
	poolsByName := map[string]Pool{}
	for _, pool := range pools {
		poolsByName[pool.Name] = pool
	}
	for _, job := range demoJobs() {
		pool, ok := poolsByName[job.Pool]
		if !ok {
			continue
		}
		customer, err := commerce.FindOrCreateCustomer(ctx, actor, core.CustomerInput{
			Name: job.Customer.Name, Phone: job.Customer.Phone, DefaultAddress: job.Customer.Address,
		})
		if err != nil {
			return fmt.Errorf("demo customer %s: %w", job.Customer.Name, err)
		}
		var existing int64
		if err := db.WithContext(ctx).Model(&Request{}).
			Where("organization_id = ? AND customer_id = ? AND pool_id = ? AND description = ?", actor.OrganizationID, customer.ID, pool.ID, job.Description).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			continue
		}
		if err := seedOneJob(ctx, db, commerce, field, actor, job, pool, customer); err != nil {
			return fmt.Errorf("demo job %s: %w", job.Description, err)
		}
	}
	return nil
}

func seedOneJob(ctx context.Context, db *gorm.DB, commerce *core.Service, field *Service, actor auth.CurrentUser, job seedJob, pool Pool, customer core.Customer) error {
	lat, lng, _ := GeocodeLagos(job.Customer.Area)
	request, err := field.CreateRequest(ctx, actor, CreateRequestInput{
		CustomerID: customer.ID, PoolID: pool.ID,
		CustomerName: job.Customer.Name, CustomerPhone: job.Customer.Phone,
		Area: job.Customer.Area, Address: job.Customer.Address,
		Latitude: &lat, Longitude: &lng,
		Description: job.Description, PreferredAt: job.PreferredAt,
	})
	if err != nil {
		return err
	}

	// Every stage begins with the booking fee, exactly as a live customer would.
	_, payment, err := field.InitializeBookingFee(ctx, actor, request.ID)
	if err != nil {
		return err
	}
	if job.Stage == "awaiting_fee" {
		return nil
	}
	if payment != nil {
		// Verification is authoritative and drives matching through the same
		// payment callback the live system uses.
		if _, err := commerce.VerifyPayment(ctx, actor, core.PaymentVerifyInput{Reference: payment.Reference}); err != nil {
			return err
		}
	} else if err := field.StartMatching(ctx, actor, request.ID); err != nil {
		return err
	}
	if job.Stage == "dispatching" {
		return nil
	}

	providerActor, attemptID, err := notifiedProviderActor(ctx, db, field, actor, request.ID)
	if err != nil {
		return err
	}
	if _, err := field.AcceptDispatch(ctx, providerActor, attemptID); err != nil {
		return err
	}
	for _, line := range job.Chat {
		if line.From == "provider" {
			if _, err := field.PostMessage(ctx, providerActor, request.ID, line.Body); err != nil {
				return err
			}
			continue
		}
		field.saveMessage(ctx, actor.OrganizationID, request.ID, "customer", nil, line.Body)
	}
	if job.Stage == "assigned" {
		return nil
	}

	for _, status := range []string{RequestOnTheWay, RequestArrived, RequestInProgress} {
		if _, err := field.TransitionJob(ctx, providerActor, request.ID, status, ""); err != nil {
			return err
		}
	}
	if job.Stage == "in_progress" {
		return nil
	}

	quote, err := field.CreateQuote(ctx, providerActor, request.ID, QuoteInput{
		LabourMinor: job.LabourMinor, MaterialsMinor: job.MaterialMin, Notes: job.QuoteNotes,
	})
	if err != nil {
		return err
	}
	if job.Stage == "quote_sent" {
		return nil
	}

	quotePayment, err := field.ApproveQuote(ctx, actor, quote.ID)
	if err != nil {
		return err
	}
	if _, err := commerce.VerifyPayment(ctx, actor, core.PaymentVerifyInput{Reference: quotePayment.Reference}); err != nil {
		return err
	}
	if _, err := field.TransitionJob(ctx, providerActor, request.ID, RequestCompleted, "Work completed and tested with the customer."); err != nil {
		return err
	}
	if job.Rating > 0 {
		if err := field.SubmitRating(ctx, actor, request.ID, job.Rating, job.Feedback); err != nil {
			return err
		}
	}
	return nil
}

// notifiedProviderActor returns an actor for whichever provider the matching
// engine actually chose, so the seeded assignments reflect real ranking.
func notifiedProviderActor(ctx context.Context, db *gorm.DB, field *Service, actor auth.CurrentUser, requestID uuid.UUID) (auth.CurrentUser, uuid.UUID, error) {
	var attempt DispatchAttempt
	if err := db.WithContext(ctx).Where("request_id = ? AND status = ?", requestID, DispatchNotified).Order("created_at ASC").First(&attempt).Error; err != nil {
		return auth.CurrentUser{}, uuid.Nil, fmt.Errorf("no provider was notified (check that a provider covers this service and area)")
	}
	provider, err := field.getProvider(ctx, actor.OrganizationID, attempt.ProviderID)
	if err != nil {
		return auth.CurrentUser{}, uuid.Nil, err
	}
	if provider.UserID == nil {
		return auth.CurrentUser{}, uuid.Nil, fmt.Errorf("provider %s has no portal login", provider.Name)
	}
	return auth.CurrentUser{ID: *provider.UserID, OrganizationID: actor.OrganizationID, Role: authz.ServiceProvider}, attempt.ID, nil
}

// seedWhatsAppChannel registers a placeholder WhatsApp channel so the operator
// can see where credentials go. It stays in draft: the runtime and outbound
// sender only use active channels, so nothing tries to call Meta until real
// credentials are filled in.
func seedWhatsAppChannel(ctx context.Context, db *gorm.DB, commerce *core.Service, actor auth.CurrentUser) error {
	ensure := func(provider, displayName, status, config string) error {
		var existing int64
		if err := db.WithContext(ctx).Model(&core.Channel{}).
			Where("organization_id = ? AND provider = ?", actor.OrganizationID, provider).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		_, err := commerce.CreateChannel(ctx, actor, core.ChannelInput{
			Provider: provider, DisplayName: displayName, Status: status, Config: config,
		})
		return err
	}
	// The WhatsApp channel stays in draft until real Meta credentials are added:
	// the runtime and the outbound sender only use active channels.
	if err := ensure("whatsapp", DemoCompanyName+" WhatsApp", "draft",
		`{"note":"Add the Meta phone number id, access token and webhook secret, then set this channel active."}`); err != nil {
		return err
	}
	// An active test channel lets the bot simulator and the demo run before those
	// credentials exist. It has no registered sender, so outbound messages are
	// recorded and marked skipped rather than leaving the system.
	return ensure("test", DemoCompanyName+" simulator", core.StatusActive,
		`{"note":"Local demo channel. Messages are recorded, never sent to a real customer."}`)
}
