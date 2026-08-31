package provider

import "context"

type ChatProvider interface {
	Name() string
	Model() string
	Complete(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type ChatResponse struct {
	Message Message `json:"message"`
}

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
