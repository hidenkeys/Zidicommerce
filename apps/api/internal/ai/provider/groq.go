package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GroqChatProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewGroqChatProvider(apiKey, baseURL, model string) *GroqChatProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	return &GroqChatProvider{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (p *GroqChatProvider) Name() string { return "groq" }

func (p *GroqChatProvider) Model() string { return p.model }

func (p *GroqChatProvider) Complete(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if p.apiKey == "" {
		return ChatResponse{}, fmt.Errorf("GROQ_API_KEY is required when AI_PROVIDER=groq")
	}
	payload := map[string]any{
		"model":    p.model,
		"messages": req.Messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("groq chat request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		return ChatResponse{}, fmt.Errorf("groq chat failed with status %d: %v", resp.StatusCode, failure)
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatResponse{}, err
	}
	if len(out.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("groq chat returned no choices")
	}
	return ChatResponse{Message: out.Choices[0].Message}, nil
}
