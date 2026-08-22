package fieldservice

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

// bookedJob drives a request to the point where a provider has accepted it, and
// returns the request plus an actor for the provider who took it.
func bookedJob(t *testing.T, fx fieldFixture) (Request, auth.CurrentUser, DispatchAttempt) {
	t.Helper()
	ctx := context.Background()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, CustomerName: "Amaka", CustomerPhone: "+2348011111111",
		Area: "Lekki", Address: "12 Admiralty Way", Description: "Leaking kitchen pipe", PreferredAt: "today",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, payment, err := fx.field.InitializeBookingFee(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.field.ConfirmBookingFromReference(ctx, fx.actor, payment.Reference); err != nil {
		t.Fatal(err)
	}
	var attempt DispatchAttempt
	if err := fx.db.Where("request_id = ? AND status = ?", request.ID, DispatchNotified).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	provider, err := fx.field.getProvider(ctx, fx.actor.OrganizationID, attempt.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	providerActor := auth.CurrentUser{ID: *provider.UserID, OrganizationID: fx.actor.OrganizationID, Role: authz.ServiceProvider}
	if _, err := fx.field.AcceptDispatch(ctx, providerActor, attempt.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := fx.field.getRequest(ctx, fx.actor.OrganizationID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	return updated, providerActor, attempt
}

func statusCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := err.(httperror.APIError)
	if !ok {
		t.Fatalf("expected an API error, got %T: %v", err, err)
	}
	return apiErr.StatusCode
}

func TestProviderCannotApproveOrDeclineTheirOwnQuote(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)
	quote, err := fx.field.CreateQuote(ctx, providerActor, request.ID, QuoteInput{LabourMinor: 2000000, MaterialsMinor: 1250000})
	if err != nil {
		t.Fatal(err)
	}
	if code := statusCode(t, mustErr(fx.field.ApproveQuote(ctx, providerActor, quote.ID))); code != 403 {
		t.Fatalf("expected 403 when a provider approves their own quote, got %d", code)
	}
	if code := statusCode(t, fx.field.DeclineQuote(ctx, providerActor, quote.ID)); code != 403 {
		t.Fatalf("expected 403 when a provider declines their own quote, got %d", code)
	}
	current, _ := fx.field.getQuote(ctx, fx.actor.OrganizationID, quote.ID)
	if current.Status != QuoteSent {
		t.Fatalf("quote status changed to %s", current.Status)
	}
}

func mustErr(_ core.Payment, err error) error { return err }

func TestQuoteCannotBeApprovedAfterDecline(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)
	quote, err := fx.field.CreateQuote(ctx, providerActor, request.ID, QuoteInput{LabourMinor: 1000000})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.field.DeclineQuote(ctx, fx.actor, quote.ID); err != nil {
		t.Fatal(err)
	}
	if code := statusCode(t, mustErr(fx.field.ApproveQuote(ctx, fx.actor, quote.ID))); code != 400 {
		t.Fatalf("expected 400 approving a declined quote, got %d", code)
	}
}

func TestExpiredQuoteCannotBeApproved(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)
	quote, err := fx.field.CreateQuote(ctx, providerActor, request.ID, QuoteInput{LabourMinor: 1000000})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if err := fx.db.Model(&Quote{}).Where("id = ?", quote.ID).Update("expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	if code := statusCode(t, mustErr(fx.field.ApproveQuote(ctx, fx.actor, quote.ID))); code != 400 {
		t.Fatalf("expected 400 approving an expired quote, got %d", code)
	}
	current, _ := fx.field.getQuote(ctx, fx.actor.OrganizationID, quote.ID)
	if current.Status != QuoteExpired {
		t.Fatalf("expected the quote to be marked expired, got %s", current.Status)
	}
}

func TestJobStateMachineRejectsSkippedStates(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)

	if code := statusCode(t, transitionErr(fx.field.TransitionJob(ctx, providerActor, request.ID, RequestCompleted, ""))); code != 400 {
		t.Fatalf("expected 400 jumping from assigned to completed, got %d", code)
	}
	for _, status := range []string{RequestOnTheWay, RequestArrived, RequestInProgress, RequestCompleted} {
		if _, err := fx.field.TransitionJob(ctx, providerActor, request.ID, status, ""); err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}
	final, _ := fx.field.getRequest(ctx, fx.actor.OrganizationID, request.ID)
	if final.Status != RequestCompleted {
		t.Fatalf("expected completed, got %s", final.Status)
	}
}

func transitionErr(_ Assignment, err error) error { return err }

func TestCompletingTwiceDoesNotDoubleCountJobs(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)
	for _, status := range []string{RequestOnTheWay, RequestArrived, RequestInProgress, RequestCompleted} {
		if _, err := fx.field.TransitionJob(ctx, providerActor, request.ID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := fx.field.getRequest(ctx, fx.actor.OrganizationID, request.ID)
	provider, _ := fx.field.getProvider(ctx, fx.actor.OrganizationID, *after.AssignedProviderID)
	first := provider.JobsCompleted

	// A repeated completion is a no-op, not another increment.
	if _, err := fx.field.TransitionJob(ctx, providerActor, request.ID, RequestCompleted, ""); err != nil {
		t.Fatalf("re-completing should be accepted as a no-op: %v", err)
	}
	provider, _ = fx.field.getProvider(ctx, fx.actor.OrganizationID, provider.ID)
	if provider.JobsCompleted != first {
		t.Fatalf("jobs_completed moved from %d to %d on a repeat completion", first, provider.JobsCompleted)
	}
}

func TestProviderCannotRateTheirOwnJob(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)
	for _, status := range []string{RequestOnTheWay, RequestArrived, RequestInProgress, RequestCompleted} {
		if _, err := fx.field.TransitionJob(ctx, providerActor, request.ID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	if code := statusCode(t, fx.field.SubmitRating(ctx, providerActor, request.ID, 5, "great job by me")); code != 403 {
		t.Fatalf("expected 403 when a provider rates their own job, got %d", code)
	}
}

func TestRatingRequiresACompletedJob(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, _, _ := bookedJob(t, fx)
	if code := statusCode(t, fx.field.SubmitRating(ctx, fx.actor, request.ID, 5, "")); code != 400 {
		t.Fatalf("expected 400 rating an unfinished job, got %d", code)
	}
}

func TestProviderSeesRedactedRequestBeforeAccepting(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, CustomerName: "Amaka Obi", CustomerPhone: "+2348011111111",
		Area: "Lekki", Address: "12 Admiralty Way", Description: "Leaking kitchen pipe",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.field.StartMatching(ctx, fx.actor, request.ID); err != nil {
		t.Fatal(err)
	}
	var attempt DispatchAttempt
	if err := fx.db.Where("request_id = ? AND status = ?", request.ID, DispatchNotified).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	provider, _ := fx.field.getProvider(ctx, fx.actor.OrganizationID, attempt.ProviderID)
	providerActor := auth.CurrentUser{ID: *provider.UserID, OrganizationID: fx.actor.OrganizationID, Role: authz.ServiceProvider}

	offered, err := fx.field.GetRequest(ctx, providerActor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if offered.CustomerPhone != "" || offered.CustomerName != "" || offered.Address != "" {
		t.Fatalf("customer contact details leaked before acceptance: %+v", offered)
	}
	if offered.Area == "" || offered.Description == "" {
		t.Fatal("the provider still needs the area and the problem to decide")
	}
	inbox, err := fx.field.ListProviderInbox(ctx, providerActor)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) == 0 {
		t.Fatal("expected a pending request in the inbox")
	}
	if inbox[0].Request.CustomerPhone != "" || inbox[0].Request.Address != "" {
		t.Fatal("customer contact details leaked through the inbox")
	}

	// Once accepted, the assigned provider does get what they need to show up.
	if _, err := fx.field.AcceptDispatch(ctx, providerActor, attempt.ID); err != nil {
		t.Fatal(err)
	}
	assigned, err := fx.field.GetRequest(ctx, providerActor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Address == "" || assigned.CustomerName == "" {
		t.Fatal("the assigned provider should see the address and customer name")
	}
}

func TestOnlyOneProviderCanAcceptARequest(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Area: "Lekki", Description: "Burst pipe", CustomerPhone: "+2348011111111",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.field.StartMatching(ctx, fx.actor, request.ID); err != nil {
		t.Fatal(err)
	}
	var first DispatchAttempt
	if err := fx.db.Where("request_id = ? AND status = ?", request.ID, DispatchNotified).First(&first).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate a broadcast strategy by offering the same job to the runner-up.
	runnerUp := fx.john.ID
	if first.ProviderID == fx.john.ID {
		runnerUp = fx.far.ID
	}
	second := DispatchAttempt{
		ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, RequestID: request.ID, ProviderID: runnerUp,
		Status: DispatchNotified, NotifiedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour), Metadata: "{}",
	}
	if err := fx.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	firstProvider, _ := fx.field.getProvider(ctx, fx.actor.OrganizationID, first.ProviderID)
	secondProvider, _ := fx.field.getProvider(ctx, fx.actor.OrganizationID, runnerUp)
	firstActor := auth.CurrentUser{ID: *firstProvider.UserID, OrganizationID: fx.actor.OrganizationID, Role: authz.ServiceProvider}
	secondActor := auth.CurrentUser{ID: *secondProvider.UserID, OrganizationID: fx.actor.OrganizationID, Role: authz.ServiceProvider}

	if _, err := fx.field.AcceptDispatch(ctx, firstActor, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.field.AcceptDispatch(ctx, secondActor, second.ID); err == nil {
		t.Fatal("the second provider should not be able to accept an already assigned job")
	}
	var assignments int64
	fx.db.Model(&Assignment{}).Where("request_id = ?", request.ID).Count(&assignments)
	if assignments != 1 {
		t.Fatalf("expected exactly one assignment, got %d", assignments)
	}
}

func TestBookingPaymentVerificationIsIdempotent(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Area: "Lekki", Description: "Leak", CustomerPhone: "+2348011111111",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, payment, err := fx.field.InitializeBookingFee(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := fx.field.ConfirmBookingFromReference(ctx, fx.actor, payment.Reference); err != nil {
			t.Fatalf("verification %d: %v", i, err)
		}
	}
	var attempts int64
	fx.db.Model(&DispatchAttempt{}).Where("request_id = ?", request.ID).Count(&attempts)
	if attempts != 1 {
		t.Fatalf("expected a single dispatch attempt after repeated verification, got %d", attempts)
	}
	var orders int64
	fx.db.Model(&core.Order{}).Where("organization_id = ?", fx.actor.OrganizationID).Count(&orders)
	if orders != 1 {
		t.Fatalf("expected a single booking order, got %d", orders)
	}
}

func TestClosingAConversationCancelsRatherThanCompletes(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, _, _ := bookedJob(t, fx)
	if err := fx.field.CloseConversation(ctx, fx.actor, request.ID); err != nil {
		t.Fatal(err)
	}
	closed, _ := fx.field.getRequest(ctx, fx.actor.OrganizationID, request.ID)
	if closed.Status != RequestCancelled {
		t.Fatalf("expected a closed job to be cancelled, got %s", closed.Status)
	}
	overview, err := fx.field.Overview(ctx, fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	if overview["completed_jobs"].(int64) != 0 {
		t.Fatalf("closing a conversation inflated completed jobs: %v", overview["completed_jobs"])
	}
}

func TestRequestsAndProvidersAreTenantIsolated(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, _, _ := bookedJob(t, fx)

	otherOrg := uuid.New()
	other := auth.CurrentUser{ID: uuid.New(), OrganizationID: otherOrg, Role: authz.MerchantAdmin}
	if _, err := fx.field.GetRequest(ctx, other, request.ID); err == nil {
		t.Fatal("another organization could read this request")
	}
	requests, err := fx.field.ListRequests(ctx, other, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("leaked %d requests across tenants", len(requests))
	}
	providers, err := fx.field.ListProviders(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("leaked %d providers across tenants", len(providers))
	}
	if _, err := fx.field.ListMatches(ctx, other, request.ID); err == nil {
		if matches, _ := fx.field.ListMatches(ctx, other, request.ID); len(matches) != 0 {
			t.Fatal("leaked match breakdowns across tenants")
		}
	}
}

func TestCustomerIntentIsNotGuessedFromKeywords(t *testing.T) {
	cases := []struct {
		text string
		want customerIntent
	}{
		{"APPROVE & PAY", intentApprove},
		{"approve and pay", intentApprove},
		{"Approve", intentApprove},
		{"yes", intentApprove},
		{"1", intentApprove},
		{"decline", intentDecline},
		{"No", intentDecline},
		{"2", intentDecline},
		{"ask a question", intentQuestion},
		{"3", intentQuestion},
		{"how do I pay?", intentNone},
		{"can I approve this tomorrow instead?", intentNone},
		{"I would rather not decline yet", intentNone},
		{"the tap is still dripping", intentNone},
		{"", intentNone},
	}
	for _, testCase := range cases {
		if got := parseCustomerIntent(testCase.text); got != testCase.want {
			t.Errorf("parseCustomerIntent(%q) = %v, want %v", testCase.text, got, testCase.want)
		}
	}
}

func TestHandoffPassesOrdinaryMessagesThroughToTheProvider(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	request, providerActor, _ := bookedJob(t, fx)
	quote, err := fx.field.CreateQuote(ctx, providerActor, request.ID, QuoteInput{LabourMinor: 2000000})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := uuid.New()
	if err := fx.db.Model(&Request{}).Where("id = ?", request.ID).Update("conversation_session_id", sessionID).Error; err != nil {
		t.Fatal(err)
	}

	handled, reply, err := fx.field.HandleHandoffInbound(ctx, fx.actor.OrganizationID, sessionID, "how do I pay?")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected the message to be handled by the handoff")
	}
	if reply != "" {
		t.Fatalf("an ordinary question should go to the provider, not trigger a reply: %q", reply)
	}
	current, _ := fx.field.getQuote(ctx, fx.actor.OrganizationID, quote.ID)
	if current.Status != QuoteSent {
		t.Fatalf("an ordinary question changed the quote to %s", current.Status)
	}

	if _, _, err := fx.field.HandleHandoffInbound(ctx, fx.actor.OrganizationID, sessionID, "APPROVE & PAY"); err != nil {
		t.Fatal(err)
	}
	current, _ = fx.field.getQuote(ctx, fx.actor.OrganizationID, quote.ID)
	if current.Status != QuoteApproved {
		t.Fatalf("expected the quote to be approved, got %s", current.Status)
	}
	messages, err := fx.field.ListMessages(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range messages {
		if message.Body == "how do I pay?" && message.AuthorType == "customer" {
			found = true
		}
	}
	if !found {
		t.Fatal("the customer question was not stored for the provider to read")
	}
}

func TestPublicCodesDoNotCollideWithExistingOnes(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	create := func() Request {
		t.Helper()
		request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
			CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Area: "Lekki", Description: "Leak", CustomerPhone: "+2348011111111",
		})
		if err != nil {
			t.Fatal(err)
		}
		return request
	}
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		code := create().PublicCode
		if seen[code] {
			t.Fatalf("public code %s was issued twice", code)
		}
		seen[code] = true
	}
	// A code created out of band (an import, a restore) must not be handed out
	// again. Deriving the next code from a row count would do exactly that.
	outOfBand := Request{
		ID: uuid.New(), OrganizationID: fx.actor.OrganizationID, PublicCode: "REQ-0009",
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, Status: RequestDraft, Metadata: "{}",
	}
	if err := fx.db.Create(&outOfBand).Error; err != nil {
		t.Fatal(err)
	}
	next := create().PublicCode
	if next == "REQ-0009" || seen[next] {
		t.Fatalf("public code %s collides with one already in use", next)
	}
}

func TestSeedLifecycleIsIdempotent(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	if err := SeedLifecycle(ctx, fx.db, fx.commerce, fx.field, fx.actor); err != nil {
		t.Fatal(err)
	}
	count := func(model any) int64 {
		var total int64
		fx.db.Model(model).Where("organization_id = ?", fx.actor.OrganizationID).Count(&total)
		return total
	}
	requests, quotes, ratings, messages := count(&Request{}), count(&Quote{}), count(&Rating{}), count(&Message{})
	if requests == 0 {
		t.Fatal("expected the seeder to create sample requests")
	}
	if quotes == 0 || ratings == 0 || messages == 0 {
		t.Fatalf("expected sample quotes, ratings and conversations; got %d/%d/%d", quotes, ratings, messages)
	}

	if err := SeedLifecycle(ctx, fx.db, fx.commerce, fx.field, fx.actor); err != nil {
		t.Fatal(err)
	}
	if got := count(&Request{}); got != requests {
		t.Fatalf("re-running the seed changed request count from %d to %d", requests, got)
	}
	if got := count(&Quote{}); got != quotes {
		t.Fatalf("re-running the seed changed quote count from %d to %d", quotes, got)
	}
	if got := count(&Rating{}); got != ratings {
		t.Fatalf("re-running the seed changed rating count from %d to %d", ratings, got)
	}
}

func TestSeedLifecycleResumesDispatchAfterLocationCorrection(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	providerIDs := []uuid.UUID{fx.john.ID, fx.far.ID}
	if err := fx.db.Model(&Provider{}).Where("organization_id = ? AND id IN ?", fx.actor.OrganizationID, providerIDs).
		Update("availability", AvailabilityOffline).Error; err != nil {
		t.Fatal(err)
	}
	description := "Seeded request with an initially unsupported location"
	request, err := fx.field.CreateRequest(ctx, fx.actor, CreateRequestInput{
		CustomerID: fx.customer.ID, PoolID: fx.plumber.ID, CustomerName: "Amaka", CustomerPhone: "+2348011111111",
		Area: "Unknown district", Address: "Unknown district", Description: description, PreferredAt: "today",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, payment, err := fx.field.InitializeBookingFee(ctx, fx.actor, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.commerce.VerifyPayment(ctx, fx.actor, core.PaymentVerifyInput{Reference: payment.Reference}); err != nil {
		t.Fatal(err)
	}
	var attempts int64
	if err := fx.db.Model(&DispatchAttempt{}).Where("request_id = ?", request.ID).Count(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatal("expected the unsupported location to produce no dispatch attempt")
	}
	if err := fx.db.Model(&Provider{}).Where("organization_id = ? AND id IN ?", fx.actor.OrganizationID, providerIDs).
		Update("availability", AvailabilityAvailable).Error; err != nil {
		t.Fatal(err)
	}

	jobs := []SeedJob{{
		Customer: SeedCustomer{Name: "Amaka", Phone: "+2348011111111", Area: "Lekki", Address: "12 Admiralty Way"},
		Pool:     "Plumber", Description: description, PreferredAt: "today", Stage: "assigned",
	}}
	if err := SeedLifecycleJobs(ctx, fx.db, fx.commerce, fx.field, fx.actor, jobs); err != nil {
		t.Fatal(err)
	}
	updated, err := fx.field.getRequest(ctx, fx.actor.OrganizationID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != RequestAssigned || updated.AssignedProviderID == nil {
		t.Fatalf("resumed request status = %s, assigned provider = %v", updated.Status, updated.AssignedProviderID)
	}
}

func TestSeededCompletedJobsFeedTheOverview(t *testing.T) {
	fx := newFieldFixture(t)
	ctx := context.Background()
	if err := SeedLifecycle(ctx, fx.db, fx.commerce, fx.field, fx.actor); err != nil {
		t.Fatal(err)
	}
	overview, err := fx.field.Overview(ctx, fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	if overview["completed_jobs"].(int64) == 0 {
		t.Fatal("expected at least one completed job in the overview")
	}
	if overview["revenue_minor"].(int64) == 0 {
		t.Fatal("expected the overview to show revenue")
	}
	earnings, err := fx.field.ProviderEarnings(ctx, fx.actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(earnings) == 0 {
		t.Fatal("expected at least one provider to have earnings")
	}
}

func TestPaymentEmailIsAcceptableToProviders(t *testing.T) {
	cases := []struct {
		phone string
		want  string
	}{
		{"+2348031234567", "2348031234567@customers.zidihq.com"},
		{"+234 803 123 4567", "2348031234567@customers.zidihq.com"},
		{"0803-123-4567", "08031234567@customers.zidihq.com"},
		{"amaka@example.com", "amaka@example.com"},
		{"AMAKA@Example.COM", "amaka@example.com"},
	}
	for _, testCase := range cases {
		got := paymentEmail(Request{CustomerPhone: testCase.phone})
		if got != testCase.want {
			t.Errorf("paymentEmail(%q) = %q, want %q", testCase.phone, got, testCase.want)
		}
		if strings.ContainsAny(got, "+ ") {
			t.Errorf("paymentEmail(%q) = %q still contains a character providers reject", testCase.phone, got)
		}
	}
	// A request with no contact detail at all still yields a valid address.
	fallback := paymentEmail(Request{ID: uuid.New()})
	if !strings.HasSuffix(fallback, "@customers.zidihq.com") || strings.HasPrefix(fallback, "@") {
		t.Fatalf("fallback address is not usable: %q", fallback)
	}
}
