package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOllamaChatProviderParsesToolCalls(t *testing.T) {
	client := NewOllamaChatProvider("http://ollama.local", "llama3.2:3b")
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_stores","arguments":{}}}]}}`)),
			Header:     make(http.Header),
		}, nil
	})}

	response, err := client.Complete(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "stores"}}})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(response.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(response.Message.ToolCalls))
	}
	if response.Message.ToolCalls[0].Function.Name != "get_stores" {
		t.Fatalf("unexpected tool name %q", response.Message.ToolCalls[0].Function.Name)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
