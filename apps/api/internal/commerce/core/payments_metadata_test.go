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
