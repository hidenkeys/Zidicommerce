package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GatewayChatProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewGatewayChatProvider(key, baseURL, model string) *GatewayChatProvider {
	return &GatewayChatProvider{apiKey: strings.TrimSpace(key), baseURL: strings.TrimRight(baseURL, "/"), model: model, client: &http.Client{Timeout: 45 * time.Second}}
}
func (p *GatewayChatProvider) Name() string  { return "vercel_gateway" }
func (p *GatewayChatProvider) Model() string { return p.model }
func (p *GatewayChatProvider) Complete(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if p.apiKey == "" {
		return ChatResponse{}, fmt.Errorf("AI_GATEWAY_API_KEY is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]any{"model": p.model, "messages": request.Messages})
	if err != nil {
		return ChatResponse{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return ChatResponse{}, fmt.Errorf("invalid gateway endpoint")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		response, err := p.client.Do(req)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("AI gateway is unavailable")
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			var result struct {
				Choices []struct {
					Message Message `json:"message"`
				} `json:"choices"`
			}
			err = json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result)
			response.Body.Close()
			if err != nil || len(result.Choices) == 0 {
				return ChatResponse{}, fmt.Errorf("AI gateway returned an invalid response")
			}
			return ChatResponse{Message: result.Choices[0].Message}, nil
		}
		response.Body.Close()
		if attempt == 2 || (response.StatusCode != 429 && response.StatusCode < 500) {
			return ChatResponse{}, fmt.Errorf("AI gateway request failed with status %d", response.StatusCode)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ChatResponse{}, fmt.Errorf("AI gateway request timed out")
		case <-timer.C:
		}
	}
	return ChatResponse{}, fmt.Errorf("AI gateway is unavailable")
}
