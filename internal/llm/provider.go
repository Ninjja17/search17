// Package llm abstracts an LLM backend capable of tool/function-calling
// chat completions, so the agent runner can work against Gemini in
// production and a scripted mock in tests.
package llm

import "context"

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall represents a single function/tool invocation requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // raw JSON arguments, as emitted by the model
}

// Message is a single chat message. Assistant messages may carry ToolCalls
// instead of (or alongside) Content. Tool-role messages must set ToolCallID
// to the ToolCall.ID they are responding to.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolDef describes a callable tool using a JSON Schema object for its parameters.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ChatRequest is a single turn request to the provider.
type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolDef
	Temperature float64
}

// ChatResponse is the model's reply for one turn.
type ChatResponse struct {
	Message Message
}

// Provider abstracts an LLM backend capable of tool/function-calling chat completions.
type Provider interface {
	CreateChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
