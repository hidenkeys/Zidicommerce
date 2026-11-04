package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type paymentRoundTripFunc func(*http.Request) (*http.Response, error)

func (f paymentRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type capturingPaymentProvider struct {
	SafeTestProvider
	request PaymentInitializeRequest
	calls   int
}

func (p *capturingPaymentProvider) Initialize(ctx context.Context, request PaymentInitializeRequest) (PaymentInitializeResponse, error) {
	p.request = request
	p.calls++
	return p.SafeTestProvider.Initialize(ctx, request)
}

func TestPaymentMetadataUsesSavedOrderAndReusesCheckout(t *testing.T) {
	fx := newCommerceFixture(t, 5)
	provider := &capturingPaymentProvider{}
	fx.service.paymentProvider = provider
	order, err := fx.service.CreateOrder(context.Background(), fx.actor, OrderInput{CustomerID: fx.customer.ID, StoreID: fx.store.ID, FulfilmentType: FulfilmentPickup, Items: []OrderItemInput{{VariantID: fx.variant.ID, Quantity: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	input := PaymentInput{OrderID: order.ID, Provider: "test", Email: "customer@example.test", IdempotencyKey: "metadata-checkout"}
	first, err := fx.service.InitializePayment(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fx.service.InitializePayment(context.Background(), fx.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || provider.calls != 1 {
		t.Fatalf("checkout was not reused: calls=%d", provider.calls)
	}
	metadata := provider.request.Metadata
	if provider.request.AmountMinor != order.TotalMinor || provider.request.Currency != order.Currency || metadata["organization_id"] != fx.actor.OrganizationID.String() || metadata["order_id"] != order.ID.String() || metadata["order_number"] != order.OrderNumber || metadata["customer_id"] != order.CustomerID.String() {
		t.Fatalf("payment did not use saved order facts: %+v", provider.request)
	}
}

func TestPaystackInitializeSendsOrderMetadata(t *testing.T) {
	p := NewPaystackProvider("test-secret")
	p.client.Transport = paymentRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://api.paystack.co/transaction/initialize" {
			t.Fatalf("unexpected payment endpoint: %s %s", r.Method, r.URL)
		}
		var payload struct {
			Amount   int64             `json:"amount"`
			Metadata map[string]string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Amount != 450000 || payload.Metadata["organization_id"] != "merchant-a" || payload.Metadata["order_id"] != "order-a" || payload.Metadata["customer_id"] != "customer-a" {
			t.Fatalf("incorrect checkout payload: %+v", payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":true,"data":{"reference":"ref-a","authorization_url":"https://checkout.paystack.com/example"}}`)), Header: make(http.Header)}, nil
	})
	result, err := p.Initialize(context.Background(), PaymentInitializeRequest{Reference: "ref-a", Email: "customer@example.test", AmountMinor: 450000, Currency: "NGN", Metadata: map[string]string{"organization_id": "merchant-a", "order_id": "order-a", "customer_id": "customer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reference != "ref-a" || result.AuthorizationURL != "https://checkout.paystack.com/example" {
		t.Fatalf("unexpected checkout: %+v", result)
	}
}
