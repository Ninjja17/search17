package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.openai.com/v1"

// OpenAIProvider implements Provider against OpenAI's Chat Completions API
// using raw net/http calls (no SDK dependency).
type OpenAIProvider struct {
	APIKey     string
	BaseURL    string // defaults to https://api.openai.com/v1 when empty
	HTTPClient *http.Client
}

// NewOpenAIProvider constructs an OpenAIProvider with sane defaults.
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:     apiKey,
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// --- OpenAI wire format ---

type oaiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"` // always "function"
	Function oaiFunction `json:"function"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	Temperature float64      `json:"temperature"`
}

type oaiChoice struct {
	Message oaiMessage `json:"message"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func toWireMessages(msgs []Message) []oaiMessage {
	out := make([]oaiMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := oaiMessage{Role: string(m.Role), ToolCallID: m.ToolCallID}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			content := m.Content
			wm.Content = &content
		}
		for _, tc := range m.ToolCalls {
			wtc := oaiToolCall{ID: tc.ID, Type: "function"}
			wtc.Function.Name = tc.Name
			wtc.Function.Arguments = tc.Arguments
			wm.ToolCalls = append(wm.ToolCalls, wtc)
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(tools []ToolDef) []oaiTool {
	out := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return out
}

func fromWireMessage(wm oaiMessage) Message {
	m := Message{Role: Role(wm.Role), ToolCallID: wm.ToolCallID}
	if wm.Content != nil {
		m.Content = *wm.Content
	}
	for _, tc := range wm.ToolCalls {
		m.ToolCalls = append(m.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return m
}

// CreateChatCompletion sends a single chat completion request to OpenAI.
func (p *OpenAIProvider) CreateChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if p.APIKey == "" {
		return ChatResponse{}, fmt.Errorf("openai provider: missing API key")
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	wireReq := oaiRequest{
		Model:       req.Model,
		Messages:    toWireMessages(req.Messages),
		Tools:       toWireTools(req.Tools),
		Temperature: req.Temperature,
	}

	body, err := json.Marshal(wireReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider: encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider: building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider: reading response: %w", err)
	}

	var wireResp oaiResponse
	if err := json.Unmarshal(respBody, &wireResp); err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider: decoding response (status %d): %w", httpResp.StatusCode, err)
	}

	if wireResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("openai provider: API error: %s", wireResp.Error.Message)
	}
	if httpResp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("openai provider: unexpected status %d: %s", httpResp.StatusCode, string(respBody))
	}
	if len(wireResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("openai provider: no choices in response")
	}

	return ChatResponse{Message: fromWireMessage(wireResp.Choices[0].Message)}, nil
}
