package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	geminiMaxAttempts    = 3
)

// GeminiProvider implements Provider against the Gemini generateContent API.
type GeminiProvider struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewGeminiProvider constructs a GeminiProvider with sane defaults.
func NewGeminiProvider(apiKey string) *GeminiProvider {
	return &GeminiProvider{
		APIKey:     apiKey,
		BaseURL:    defaultGeminiBaseURL,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
	// ThoughtSignature must be echoed back verbatim on the function call part
	// in later turns, or "thinking" models reject the request (see
	// https://ai.google.dev/gemini-api/docs/thought-signatures).
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	GenerationConfig  struct {
		Temperature float64 `json:"temperature"`
	} `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func toGeminiRequest(req ChatRequest) (geminiRequest, error) {
	callNames := make(map[string]string)
	for _, message := range req.Messages {
		for _, call := range message.ToolCalls {
			callNames[call.ID] = call.Name
		}
	}

	var wire geminiRequest
	var pendingToolParts []geminiPart
	flushToolParts := func() {
		if len(pendingToolParts) > 0 {
			wire.Contents = append(wire.Contents, geminiContent{Role: "user", Parts: pendingToolParts})
			pendingToolParts = nil
		}
	}

	for _, message := range req.Messages {
		if message.Role == RoleSystem {
			if message.Content != "" {
				wire.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: message.Content}}}
			}
			continue
		}
		if message.Role == RoleTool {
			name := callNames[message.ToolCallID]
			if name == "" {
				return geminiRequest{}, fmt.Errorf("gemini provider: tool result has no matching function call")
			}
			response := map[string]any{"result": message.Content}
			var decoded any
			if json.Unmarshal([]byte(message.Content), &decoded) == nil {
				response["result"] = decoded
			}
			pendingToolParts = append(pendingToolParts, geminiPart{FunctionResponse: &geminiFunctionResponse{
				Name: name, Response: response,
			}})
			continue
		}

		flushToolParts()
		content := geminiContent{Role: "user", Parts: make([]geminiPart, 0, 1+len(message.ToolCalls))}
		if message.Role == RoleAssistant {
			content.Role = "model"
		}
		if message.Content != "" {
			content.Parts = append(content.Parts, geminiPart{Text: message.Content})
		}
		for _, call := range message.ToolCalls {
			args := map[string]any{}
			if call.Arguments != "" {
				if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
					return geminiRequest{}, fmt.Errorf("gemini provider: decoding tool arguments: %w", err)
				}
			}
			content.Parts = append(content.Parts, geminiPart{
				FunctionCall:     &geminiFunctionCall{Name: call.Name, Args: args},
				ThoughtSignature: call.Signature,
			})
		}
		if len(content.Parts) > 0 {
			wire.Contents = append(wire.Contents, content)
		}
	}
	flushToolParts()

	if len(req.Tools) > 0 {
		declarations := make([]geminiFunctionDeclaration, 0, len(req.Tools))
		for _, tool := range req.Tools {
			declarations = append(declarations, geminiFunctionDeclaration{
				Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters,
			})
		}
		wire.Tools = []geminiTool{{FunctionDeclarations: declarations}}
	}
	wire.GenerationConfig.Temperature = req.Temperature
	return wire, nil
}

// CreateChatCompletion sends a single chat completion request to Gemini.
func (p *GeminiProvider) CreateChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if p.APIKey == "" {
		return ChatResponse{}, fmt.Errorf("gemini provider: missing API key")
	}
	if req.Model == "" {
		return ChatResponse{}, fmt.Errorf("gemini provider: model is required")
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	baseURL := strings.TrimRight(p.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}

	wireReq, err := toGeminiRequest(req)
	if err != nil {
		return ChatResponse{}, err
	}
	body, err := json.Marshal(wireReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("gemini provider: encoding request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, url.PathEscape(req.Model), url.QueryEscape(p.APIKey))
	var respBody []byte
	statusCode := 0
	for attempt := 0; attempt < geminiMaxAttempts; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return ChatResponse{}, fmt.Errorf("gemini provider: building request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("gemini provider: request failed: %w", err)
		}
		respBody, err = io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			return ChatResponse{}, fmt.Errorf("gemini provider: reading response: %w", err)
		}
		statusCode = httpResp.StatusCode
		if !geminiRetryableStatus(statusCode) || attempt == geminiMaxAttempts-1 {
			break
		}

		delay := 500 * time.Millisecond * time.Duration(1<<attempt)
		select {
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		case <-time.After(delay):
		}
	}

	var wireResp geminiResponse
	if err := json.Unmarshal(respBody, &wireResp); err != nil {
		return ChatResponse{}, fmt.Errorf("gemini provider: decoding response (status %d): %w", statusCode, err)
	}
	if wireResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("gemini provider: API error: %s", wireResp.Error.Message)
	}
	if statusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("gemini provider: unexpected status %d: %s", statusCode, string(respBody))
	}
	if len(wireResp.Candidates) == 0 {
		return ChatResponse{}, fmt.Errorf("gemini provider: no candidates in response")
	}

	message := Message{Role: RoleAssistant}
	for index, part := range wireResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			message.Content += part.Text
		}
		if part.FunctionCall != nil {
			args, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				return ChatResponse{}, fmt.Errorf("gemini provider: encoding function call arguments: %w", err)
			}
			message.ToolCalls = append(message.ToolCalls, ToolCall{
				ID: fmt.Sprintf("gemini_call_%d", index), Name: part.FunctionCall.Name, Arguments: string(args),
				Signature: part.ThoughtSignature,
			})
		}
	}
	return ChatResponse{Message: message}, nil
}

func geminiRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}
