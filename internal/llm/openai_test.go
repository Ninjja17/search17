package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProvider_CreateChatCompletion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %q", got)
		}
		var body oaiRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body.Model != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %s", body.Model)
		}

		content := "hello from assistant"
		resp := oaiResponse{
			Choices: []oaiChoice{
				{Message: oaiMessage{Role: "assistant", Content: &content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenAIProvider{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	resp, err := p.CreateChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "hello from assistant" {
		t.Errorf("expected assistant content, got %q", resp.Message.Content)
	}
}

func TestOpenAIProvider_CreateChatCompletion_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tc oaiToolCall
		tc.ID = "call_1"
		tc.Type = "function"
		tc.Function.Name = "get_current_date"
		tc.Function.Arguments = "{}"

		resp := oaiResponse{
			Choices: []oaiChoice{
				{Message: oaiMessage{Role: "assistant", ToolCalls: []oaiToolCall{tc}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenAIProvider{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	resp, err := p.CreateChatCompletion(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: RoleUser, Content: "audit this"}},
		Tools:    []ToolDef{{Name: "get_current_date", Description: "returns today"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Name != "get_current_date" {
		t.Errorf("expected get_current_date, got %s", resp.Message.ToolCalls[0].Name)
	}
}

func TestOpenAIProvider_MissingAPIKey(t *testing.T) {
	p := &OpenAIProvider{}
	_, err := p.CreateChatCompletion(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
}

func TestOpenAIProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "invalid api key"},
		})
	}))
	defer server.Close()

	p := &OpenAIProvider{APIKey: "bad-key", BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := p.CreateChatCompletion(context.Background(), ChatRequest{Model: "gpt-4o-mini"})
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("expected API error surfaced, got %v", err)
	}
}
