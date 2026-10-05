package llm

import (
	"context"
	"testing"
)

func TestMockProvider_ScriptedSequence(t *testing.T) {
	m := &MockProvider{
		Responses: []ChatResponse{
			{Message: Message{Role: RoleAssistant, Content: "first"}},
			{Message: Message{Role: RoleAssistant, Content: "second"}},
		},
	}

	resp1, err := m.CreateChatCompletion(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp1.Message.Content != "first" {
		t.Errorf("expected first, got %s", resp1.Message.Content)
	}

	resp2, err := m.CreateChatCompletion(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.Message.Content != "second" {
		t.Errorf("expected second, got %s", resp2.Message.Content)
	}

	if m.CallCount() != 2 {
		t.Errorf("expected call count 2, got %d", m.CallCount())
	}
}

func TestMockProvider_ScriptExhausted(t *testing.T) {
	m := &MockProvider{Responses: []ChatResponse{{Message: Message{Content: "only"}}}}

	if _, err := m.CreateChatCompletion(context.Background(), ChatRequest{}); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if _, err := m.CreateChatCompletion(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error when script is exhausted")
	}
}
