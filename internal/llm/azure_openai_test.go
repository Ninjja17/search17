package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAzureOpenAIProvider_CreateChatCompletion_Success(t *testing.T) {
	var gotPath, gotAPIKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotAPIKeyHeader = r.Header.Get("api-key")

		content := "azure response"
		resp := oaiResponse{Choices: []oaiChoice{{Message: oaiMessage{Role: "assistant", Content: &content}}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AzureOpenAIProvider{
		APIKey: "azure-key", Endpoint: server.URL, Deployment: "my-deployment", HTTPClient: server.Client(),
	}
	resp, err := p.CreateChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "azure response" {
		t.Errorf("expected azure response content, got %q", resp.Message.Content)
	}
	if gotAPIKeyHeader != "azure-key" {
		t.Errorf("expected api-key header set, got %q", gotAPIKeyHeader)
	}
	if !strings.Contains(gotPath, "/openai/deployments/my-deployment/chat/completions") {
		t.Errorf("expected deployment in URL path, got %q", gotPath)
	}
	if !strings.Contains(gotPath, "api-version=") {
		t.Errorf("expected api-version query param, got %q", gotPath)
	}
}

func TestAzureOpenAIProvider_MissingConfig(t *testing.T) {
	if _, err := (&AzureOpenAIProvider{}).CreateChatCompletion(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error for missing API key")
	}
	if _, err := (&AzureOpenAIProvider{APIKey: "k"}).CreateChatCompletion(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error for missing endpoint/deployment")
	}
}
