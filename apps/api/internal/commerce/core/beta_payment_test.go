package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type checkoutProbe struct {
	namedPaymentProvider
	started chan struct{}
	release chan struct{}
	calls   int
}

func (p *checkoutProbe) Initialize(ctx context.Context, req PaymentInitializeRequest) (PaymentInitializeResponse, error) {
	p.calls++
	close(p.started)
	<-p.release
	return p.namedPaymentProvider.Initialize(ctx, req)
}

func TestCheckoutReservesReferenceBeforeProviderCall(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	ctx := context.Background()
	probe := &checkoutProbe{namedPaymentProvider: namedPaymentProvider{name: "paystack"}, started: make(chan struct{}), release: make(chan struct{})}
	fx.service.paymentProvider = probe
	order, err := fx.service.CreateOrder(ctx, fx.actor, OrderInput{
		CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup,
		Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := PaymentInput{OrderID: order.ID, IdempotencyKey: "checkout"}
	done := make(chan error, 1)
	go func() {
		_, err := fx.service.InitializePayment(ctx, fx.actor, input)
		done <- err
	}()
	select {
	case <-probe.started:
	case err := <-done:
		t.Fatalf("checkout never reached provider: %v", err)
	}
	_, concurrentErr := fx.service.InitializePayment(ctx, fx.actor, input)
	close(probe.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if concurrentErr == nil || probe.calls != 1 {
		t.Fatalf("expected one remote initialization and a blocked concurrent retry, calls=%d error=%v", probe.calls, concurrentErr)
	}
	if payment, err := fx.service.InitializePayment(ctx, fx.actor, input); err != nil || payment.AuthorizationURL == "" {
		t.Fatalf("expected saved checkout on retry: %v", err)
	}
}

func TestPaystackCheckoutRetryAfterRejection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  bool
	}{
		{"validation rejection", 400, `{"status":false,"type":"validation_error","message":"Invalid Email Address Passed"}`, true},
		{"duplicate reference", 400, `{"status":false,"type":"validation_error","message":"Duplicate Transaction Reference"}`, false},
		{"provider failure", 503, `{"status":false}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newCommerceFixture(t, 5)
			ctx := context.Background()
			provider := NewPaystackProvider("sk_test_fixture")
			var refs []string
			provider.client.Transport = paymentRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				var request struct {
					Reference string
					Email     string
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				refs = append(refs, request.Reference)
				status, body := tc.status, tc.body
				if request.Email == "buyer@example.com" {
					status = http.StatusOK
					body = `{"status":true,"data":{"authorization_url":"https://checkout.paystack.com/test","reference":"` + request.Reference + `"}}`
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			fx.service.paymentProvider = provider
			fx.service.EnableControlledBeta()
			order, err := fx.service.CreateOrder(ctx, fx.actor, OrderInput{
				CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup,
				Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
			})
			if err != nil {
				t.Fatal(err)
			}
			input := PaymentInput{OrderID: order.ID, IdempotencyKey: "retry-checkout", Email: "invalid"}
			if _, err := fx.service.InitializePayment(ctx, fx.actor, input); err == nil {
				t.Fatal("expected first checkout to fail")
			}
			input.Email = "buyer@example.com"
			payment, err := fx.service.InitializePayment(ctx, fx.actor, input)
			if !tc.retry {
				if err == nil || len(refs) != 1 {
					t.Fatalf("uncertain checkout allowed another provider call: calls=%d err=%v", len(refs), err)
				}
				return
			}
			if err != nil || payment.AuthorizationURL == "" || len(refs) != 2 || refs[0] == refs[1] {
				t.Fatalf("corrected checkout did not succeed with a new reference: calls=%v err=%v", refs, err)
			}
			replay, err := fx.service.InitializePayment(ctx, fx.actor, input)
			if err != nil || replay.ID != payment.ID || len(refs) != 2 {
				t.Fatalf("successful checkout was not reused: calls=%d err=%v", len(refs), err)
			}
		})
	}
}

type retryVerificationProvider struct {
	namedPaymentProvider
	unavailable bool
}

func (p *retryVerificationProvider) Verify(ctx context.Context, reference string) (PaymentVerification, error) {
	if p.unavailable {
		return PaymentVerification{}, errors.New("provider timeout")
	}
	return p.namedPaymentProvider.Verify(ctx, reference)
}

func TestPaystackWebhookRetriesAfterVerificationTimeout(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	ctx := context.Background()
	probe := &retryVerificationProvider{namedPaymentProvider: namedPaymentProvider{name: "paystack", amountMinor: fx.variant.PriceMinor, currency: "NGN"}, unavailable: true}
	fx.service.paymentProvider = probe
	const fixtureKey = "mock-webhook-key"
	fx.service.ConfigurePaymentWebhooks(fixtureKey)
	order, err := fx.service.CreateOrder(ctx, fx.actor, OrderInput{
		CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup,
		Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	payment, err := fx.service.InitializePayment(ctx, fx.actor, PaymentInput{OrderID: order.ID, IdempotencyKey: "retry-webhook"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"event":"charge.success","data":{"id":123,"reference":"` + payment.Reference + `","status":"success"}}`)
	signature := paystackTestSignature(fixtureKey, body)
	if _, err := fx.service.HandlePaystackWebhook(ctx, body, signature); err == nil {
		t.Fatal("expected retryable error")
	}
	current, err := fx.service.GetOrder(ctx, fx.actor, order.ID)
	if err != nil || current.Status != OrderAwaitingPayment {
		t.Fatalf("unverified order advanced: %s, %v", current.Status, err)
	}
	probe.unavailable = false
	first, err := fx.service.HandlePaystackWebhook(ctx, body, signature)
	if err != nil || first.Status != paymentWebhookProcessed {
		t.Fatalf("retry did not process: %+v %v", first, err)
	}
	replay, err := fx.service.HandlePaystackWebhook(ctx, body, signature)
	if err != nil || replay.EventID != first.EventID {
		t.Fatalf("processed replay did not reuse receipt: %+v %v", replay, err)
	}
}

func TestBetaRejectsLivePaystackAndIncompleteVerification(t *testing.T) {
	ctx := context.Background()
	s := NewService(nil, NewPaystackProvider("sk_live_fixture"))
	s.EnableControlledBeta()
	if _, err := s.resolveUsablePaymentProvider(ctx, uuid.New(), "paystack"); err == nil {
		t.Fatal("accepted live Paystack key")
	}
	s.paymentProvider = NewPaystackProvider("sk_test_fixture")
	if _, err := s.resolveUsablePaymentProvider(ctx, uuid.New(), "paystack"); err != nil {
		t.Fatal(err)
	}
	payment := Payment{Provider: "paystack", Reference: "expected", AmountMinor: 100, Currency: "NGN"}
	for _, proof := range []PaymentVerification{
		{Paid: true, AmountMinor: 100, Currency: "NGN"},
		{Paid: true, Reference: "expected", Currency: "NGN"},
		{Paid: true, Reference: "expected", AmountMinor: 100},
	} {
		if err := verifyPaymentMatches(payment, proof); err == nil {
			t.Fatal("accepted incomplete Paystack proof")
		}
	}
}

func TestBetaCannotMarkOrderPaidThroughGenericTransition(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	fx.service.EnableControlledBeta()
	ctx := context.Background()
	order, err := fx.service.CreateOrder(ctx, fx.actor, OrderInput{
		CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup,
		Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.TransitionOrder(ctx, fx.actor, order.ID, TransitionInput{Status: OrderPaid}); err == nil {
		t.Fatal("marked an unverified order paid")
	}
	current, err := fx.service.GetOrder(ctx, fx.actor, order.ID)
	if err != nil || current.Status != OrderAwaitingPayment {
		t.Fatalf("expected order awaiting payment: %s, %v", current.Status, err)
	}
}

func TestPaymentMutationRoutesRequirePaymentPermission(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	app.Use(func(c *fiber.Ctx) error {
		auth.SetCurrentUser(c, auth.CurrentUser{OrganizationID: uuid.New(), Role: authz.Viewer})
		return c.Next()
	})
	NewHandler(nil, nil).Register(app)
	for _, path := range []string{"/payments/initialize", "/payments/verify"} {
		response, err := app.Test(httptest.NewRequest("POST", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != fiber.StatusForbidden {
			t.Fatalf("%s returned %d, want forbidden", path, response.StatusCode)
		}
	}
}
