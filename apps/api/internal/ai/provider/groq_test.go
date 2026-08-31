package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGroqChatProviderSendsOpenAICompatibleRequest(t *testing.T) {
	client := NewGroqChatProvider("test-key", "https://groq.local/openai/v1", "llama-test")
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://groq.local/openai/v1/chat/completions" {
			t.Fatalf("unexpected url %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"Grounded answer."}}]}`)),
			Header:     make(http.Header),
		}, nil
	})}

	response, err := client.Complete(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if response.Message.Content != "Grounded answer." {
		t.Fatalf("unexpected response %q", response.Message.Content)
	}
}

func TestGroqChatProviderRequiresAPIKey(t *testing.T) {
	client := NewGroqChatProvider("", "", "")
	_, err := client.Complete(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err == nil || !strings.Contains(err.Error(), "GROQ_API_KEY") {
		t.Fatalf("expected missing key error, got %v", err)
	}
}
