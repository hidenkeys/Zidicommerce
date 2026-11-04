package core

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestJourneyFlowPilotRequiresPaystackTestCredential(t *testing.T) {
	for _, tc := range []struct {
		secret  string
		allowed bool
	}{
		{"sk_test_local-fixture", true},
		{"sk_live_local-fixture", false},
		{"", false},
	} {
		s := NewService(nil, NewPaystackProvider(tc.secret))
		err := s.RequireSandboxPaymentProvider(context.Background(), uuid.New(), "paystack")
		if (err == nil) != tc.allowed {
			t.Fatalf("credential classification: allowed=%v err=%v", tc.allowed, err)
		}
	}
	if err := NewService(nil, SafeTestProvider{}).RequireSandboxPaymentProvider(context.Background(), uuid.New(), "test"); err != nil {
		t.Fatal(err)
	}
}
