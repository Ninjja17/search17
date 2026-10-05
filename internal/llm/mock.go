package llm

import (
	"context"
	"fmt"
)

// MockProvider is a scripted, deterministic Provider test double. It returns
// ChatResponses from Responses in order, one per call, so tests can drive
// the agent runner through a canned tool-call sequence without any network
// access or API cost.
type MockProvider struct {
	Responses []ChatResponse
	calls     int
	// Requests records every ChatRequest received, so tests can assert on
	// what the runner sent (e.g. tool results, message history).
	Requests []ChatRequest
}

// CreateChatCompletion returns the next scripted response, or an error if the
// script is exhausted (indicating the runner made more calls than expected).
func (m *MockProvider) CreateChatCompletion(_ context.Context, req ChatRequest) (ChatResponse, error) {
	m.Requests = append(m.Requests, req)
	if m.calls >= len(m.Responses) {
		return ChatResponse{}, fmt.Errorf("mock provider: no scripted response for call %d (script has %d)", m.calls, len(m.Responses))
	}
	resp := m.Responses[m.calls]
	m.calls++
	return resp, nil
}

// CallCount returns how many times CreateChatCompletion has been invoked.
func (m *MockProvider) CallCount() int {
	return m.calls
}
