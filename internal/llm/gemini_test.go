package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiProvider_CreateChatCompletion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-3.8-flash:generateContent" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("expected API key query parameter")
		}
		var body geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body.SystemInstruction == nil || body.SystemInstruction.Parts[0].Text != "system" {
			t.Errorf("expected system instruction, got %#v", body.SystemInstruction)
		}
		if len(body.Tools) != 1 || len(body.Tools[0].FunctionDeclarations) != 1 {
			t.Errorf("expected one function declaration, got %#v", body.Tools)
		}
		_ = json.NewEncoder(w).Encode(geminiResponse{Candidates: []struct {
			Content geminiContent `json:"content"`
		}{{Content: geminiContent{Role: "model", Parts: []geminiPart{{Text: "hello from Gemini"}}}}}})
	}))
	defer server.Close()

	p := &GeminiProvider{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	resp, err := p.CreateChatCompletion(context.Background(), ChatRequest{
		Model:    "gemini-3.8-flash",
		Messages: []Message{{Role: RoleSystem, Content: "system"}, {Role: RoleUser, Content: "hi"}},
		Tools:    []ToolDef{{Name: "get_current_date", Description: "returns today"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "hello from Gemini" {
		t.Errorf("expected assistant content, got %q", resp.Message.Content)
	}
}

func TestGeminiProvider_CreateChatCompletion_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(geminiResponse{Candidates: []struct {
			Content geminiContent `json:"content"`
		}{{Content: geminiContent{Parts: []geminiPart{{
			FunctionCall:     &geminiFunctionCall{Name: "get_current_date", Args: map[string]any{}},
			ThoughtSignature: "sig-abc",
		}}}}}})
	}))
	defer server.Close()

	p := &GeminiProvider{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	resp, err := p.CreateChatCompletion(context.Background(), ChatRequest{Model: "gemini-3.8-flash", Messages: []Message{{Role: RoleUser, Content: "audit"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Name != "get_current_date" || resp.Message.ToolCalls[0].Arguments != "{}" {
		t.Errorf("unexpected tool calls: %#v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Signature != "sig-abc" {
		t.Errorf("expected thoughtSignature captured as Signature, got %q", resp.Message.ToolCalls[0].Signature)
	}
}

// TestGeminiProvider_EchoesThoughtSignatureOnReplay guards the fix for:
// "Function call is missing a thought_signature in functionCall parts" —
// Gemini's thinking models require the prior turn's signature to be sent
// back verbatim when that assistant message (with its tool call) is
// included in the next request's history.
func TestGeminiProvider_EchoesThoughtSignatureOnReplay(t *testing.T) {
	var gotSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		for _, c := range body.Contents {
			for _, p := range c.Parts {
				if p.FunctionCall != nil {
					gotSignature = p.ThoughtSignature
				}
			}
		}
		content := "done"
		_ = json.NewEncoder(w).Encode(geminiResponse{Candidates: []struct {
			Content geminiContent `json:"content"`
		}{{Content: geminiContent{Parts: []geminiPart{{Text: content}}}}}})
	}))
	defer server.Close()

	p := &GeminiProvider{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := p.CreateChatCompletion(context.Background(), ChatRequest{
		Model: "gemini-3.8-flash",
		Messages: []Message{
			{Role: RoleUser, Content: "audit"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "get_current_date", Arguments: "{}", Signature: "sig-xyz"}}},
			{Role: RoleTool, ToolCallID: "c1", Content: `{"date":"2026-10-05"}`},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSignature != "sig-xyz" {
		t.Errorf("expected thoughtSignature echoed back as sig-xyz, got %q", gotSignature)
	}
}

func TestGeminiProvider_MissingAPIKey(t *testing.T) {
	_, err := (&GeminiProvider{}).CreateChatCompletion(context.Background(), ChatRequest{Model: "gemini-3.8-flash"})
	if err == nil || !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
}

func TestGeminiProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "invalid API key"}})
	}))
	defer server.Close()

	p := &GeminiProvider{APIKey: "bad-key", BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := p.CreateChatCompletion(context.Background(), ChatRequest{Model: "gemini-3.8-flash"})
	if err == nil || !strings.Contains(err.Error(), "invalid API key") {
		t.Fatalf("expected API error surfaced, got %v", err)
	}
}

func TestGeminiProvider_RetriesTemporaryCapacityError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "high demand"}})
			return
		}
		_ = json.NewEncoder(w).Encode(geminiResponse{Candidates: []struct {
			Content geminiContent `json:"content"`
		}{{Content: geminiContent{Parts: []geminiPart{{Text: "retried successfully"}}}}}})
	}))
	defer server.Close()

	p := &GeminiProvider{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()}
	resp, err := p.CreateChatCompletion(context.Background(), ChatRequest{Model: "gemini-3.8-flash", Messages: []Message{{Role: RoleUser, Content: "audit"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 || resp.Message.Content != "retried successfully" {
		t.Errorf("expected one retry and a successful response, got attempts=%d response=%q", attempts, resp.Message.Content)
	}
}
