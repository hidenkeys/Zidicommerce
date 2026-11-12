package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayAuthenticationAndSafeErrors(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-key" || r.URL.Path != "/chat/completions" {
			t.Error("invalid gateway request")
		}
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Grounded answer"}}]}`))
			return
		}
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"fixture-key private provider diagnostics"}`))
	}))
	defer server.Close()
	p := NewGatewayChatProvider("fixture-key", server.URL, "test-model")
	result, err := p.Complete(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "Question"}}})
	if err != nil || result.Message.Content != "Grounded answer" || p.Name() != "vercel_gateway" {
		t.Fatalf("unexpected response %v %v", result, err)
	}
	_, err = p.Complete(context.Background(), ChatRequest{})
	if err == nil || strings.Contains(err.Error(), "fixture-key") || strings.Contains(err.Error(), "private") || calls != 2 {
		t.Fatalf("unsafe error or unexpected retry: %v calls=%d", err, calls)
	}
}
