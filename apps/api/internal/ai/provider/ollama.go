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

type OllamaChatProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaChatProvider(baseURL, model string) *OllamaChatProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "llama3.2:3b"
	}
	return &OllamaChatProvider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (p *OllamaChatProvider) Name() string { return "ollama" }

func (p *OllamaChatProvider) Model() string { return p.model }

func (p *OllamaChatProvider) Complete(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	payload := map[string]any{
		"model":    p.model,
		"stream":   false,
		"messages": req.Messages,
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		return ChatResponse{}, fmt.Errorf("ollama chat failed with status %d: %v", resp.StatusCode, failure)
	}
	var out struct {
		Message Message `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{Message: out.Message}, nil
}
