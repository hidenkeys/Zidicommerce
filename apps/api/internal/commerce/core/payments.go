package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SafeTestProvider struct{}

func (SafeTestProvider) Name() string {
	return "test"
}

func (SafeTestProvider) Initialize(_ context.Context, req PaymentInitializeRequest) (PaymentInitializeResponse, error) {
	return PaymentInitializeResponse{
		Reference:        req.Reference,
		AuthorizationURL: "https://payments.zidicommerce.local/pay/" + req.Reference,
		ProviderMetadata: "{}",
	}, nil
}

func (SafeTestProvider) Verify(_ context.Context, reference string) (PaymentVerification, error) {
	return PaymentVerification{Reference: reference, Paid: true, Status: "success"}, nil
}

var errPaystackCheckoutRejected = errors.New("Paystack rejected checkout details")

type PaystackProvider struct {
	secretKey string
	client    *http.Client
}

func NewPaystackProvider(secretKey string) *PaystackProvider {
	return &PaystackProvider{
		secretKey: strings.TrimSpace(secretKey),
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PaystackProvider) Name() string {
	return "paystack"
}

func (p *PaystackProvider) Initialize(ctx context.Context, req PaymentInitializeRequest) (PaymentInitializeResponse, error) {
	if p.secretKey == "" {
		return PaymentInitializeResponse{}, errors.New("paystack secret key is not configured")
	}

	payload := map[string]any{
		"reference":    req.Reference,
		"email":        req.Email,
		"amount":       req.AmountMinor,
		"currency":     req.Currency,
		"callback_url": req.CallbackURL,
		"metadata":     req.Metadata,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return PaymentInitializeResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.paystack.co/transaction/initialize", bytes.NewReader(body))
	if err != nil {
		return PaymentInitializeResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return PaymentInitializeResponse{}, err
	}
	defer resp.Body.Close()

	var decoded struct {
		Status bool   `json:"status"`
		Type   string `json:"type"`
		Code   string `json:"code"`
		Data   struct {
			AuthorizationURL string `json:"authorization_url"`
			Reference        string `json:"reference"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return PaymentInitializeResponse{}, err
	}
	// A duplicate reference may already have a checkout. Only release the
	// reservation for a documented validation rejection unrelated to a reference.
	detail := strings.ToLower(decoded.Code + " " + decoded.Message)
	if resp.StatusCode == http.StatusBadRequest && !decoded.Status &&
		decoded.Data.Reference == "" && decoded.Data.AuthorizationURL == "" &&
		!strings.Contains(detail, "reference") && !strings.Contains(detail, "duplicate") &&
		(decoded.Type == "validation_error" || strings.Contains(detail, "invalid email address") ||
			strings.Contains(detail, "email is not allowed to be empty")) {
		return PaymentInitializeResponse{}, errPaystackCheckoutRejected
	}
	if resp.StatusCode >= 400 || !decoded.Status {
		return PaymentInitializeResponse{}, fmt.Errorf("paystack initialize failed: %s", decoded.Message)
	}
	if decoded.Data.Reference == "" {
		decoded.Data.Reference = req.Reference
	}
	return PaymentInitializeResponse{Reference: decoded.Data.Reference, AuthorizationURL: decoded.Data.AuthorizationURL, ProviderMetadata: "{}"}, nil
}

func (p *PaystackProvider) Verify(ctx context.Context, reference string) (PaymentVerification, error) {
	if p.secretKey == "" {
		return PaymentVerification{}, errors.New("paystack secret key is not configured")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.paystack.co/transaction/verify/"+reference, nil)
	if err != nil {
		return PaymentVerification{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return PaymentVerification{}, err
	}
	defer resp.Body.Close()

	var decoded struct {
		Status bool `json:"status"`
		Data   struct {
			Reference string `json:"reference"`
			Status    string `json:"status"`
			Amount    int64  `json:"amount"`
			Currency  string `json:"currency"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return PaymentVerification{}, err
	}
	if resp.StatusCode >= 400 || !decoded.Status {
		return PaymentVerification{}, fmt.Errorf("paystack verify failed: %s", decoded.Message)
	}
	return PaymentVerification{Reference: decoded.Data.Reference, Paid: decoded.Data.Status == "success", Status: decoded.Data.Status, AmountMinor: decoded.Data.Amount, Currency: strings.ToUpper(decoded.Data.Currency), ProviderMetadata: jsonValue(decoded.Data)}, nil
}
