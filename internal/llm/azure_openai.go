package llm

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

const defaultAzureAPIVersion = "2024-02-15-preview"

// AzureOpenAIProvider implements Provider against an Azure OpenAI deployment.
// It reuses the OpenAI wire format (internal/llm/openai.go) since Azure's
// Chat Completions payload is wire-compatible; only the URL shape and the
// auth header differ. This exists to prove the Provider abstraction is
// genuinely swappable, not just an interface on paper.
type AzureOpenAIProvider struct {
	APIKey     string
	Endpoint   string // e.g. https://<resource>.openai.azure.com
	Deployment string // Azure deployment name (used instead of Model in the URL)
	APIVersion string // defaults to 2024-02-15-preview when empty
	HTTPClient *http.Client
}

// NewAzureOpenAIProvider constructs an AzureOpenAIProvider with sane defaults.
func NewAzureOpenAIProvider(apiKey, endpoint, deployment string) *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		APIKey:     apiKey,
		Endpoint:   endpoint,
		Deployment: deployment,
		APIVersion: defaultAzureAPIVersion,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// CreateChatCompletion sends a single chat completion request to an Azure
// OpenAI deployment.
func (p *AzureOpenAIProvider) CreateChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if p.APIKey == "" {
		return ChatResponse{}, fmt.Errorf("azure openai provider: missing API key")
	}
	if p.Endpoint == "" || p.Deployment == "" {
		return ChatResponse{}, fmt.Errorf("azure openai provider: endpoint and deployment are required")
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	apiVersion := p.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAzureAPIVersion
	}

	wireReq := oaiRequest{
		Model:       req.Model,
		Messages:    toWireMessages(req.Messages),
		Tools:       toWireTools(req.Tools),
		Temperature: req.Temperature,
	}
	body, err := json.Marshal(wireReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("azure openai provider: encoding request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		strings.TrimRight(p.Endpoint, "/"), p.Deployment, apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("azure openai provider: building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.APIKey) // Azure uses api-key, not Authorization: Bearer

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("azure openai provider: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("azure openai provider: reading response: %w", err)
	}

	var wireResp oaiResponse
	if err := json.Unmarshal(respBody, &wireResp); err != nil {
		return ChatResponse{}, fmt.Errorf("azure openai provider: decoding response (status %d): %w", httpResp.StatusCode, err)
	}
	if wireResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("azure openai provider: API error: %s", wireResp.Error.Message)
	}
	if httpResp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("azure openai provider: unexpected status %d: %s", httpResp.StatusCode, string(respBody))
	}
	if len(wireResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("azure openai provider: no choices in response")
	}

	return ChatResponse{Message: fromWireMessage(wireResp.Choices[0].Message)}, nil
}
