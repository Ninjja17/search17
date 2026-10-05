package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/search17/search17/internal/llm"
	"github.com/search17/search17/internal/memory"
)

func sampleRecords() []memory.Record {
	return []memory.Record{
		{ID: "mem-1", Memory: "User likes blue.", Source: "chat", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Confidence: 0.9, MemoryType: "preference"},
		{ID: "mem-2", Memory: "User likes green.", Source: "chat", Timestamp: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Confidence: 0.9, MemoryType: "preference"},
	}
}

func toolCallMsg(id, name string, args any) llm.Message {
	raw, _ := json.Marshal(args)
	return llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{
			{ID: id, Name: name, Arguments: string(raw)},
		},
	}
}

func TestRunner_ReportMode_HappyPath(t *testing.T) {
	mock := &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "list_memories", map[string]any{"offset": 0, "limit": 20})},
			{Message: toolCallMsg("c2", "record_finding", map[string]any{
				"category": "contradiction", "risk": "medium",
				"memory_ids": []string{"mem-1", "mem-2"},
				"title":      "Conflicting favorite color",
				"detail":     "mem-1 says blue, mem-2 says green.",
			})},
			{Message: toolCallMsg("c3", "finalize_report", map[string]any{"summary": "Found one contradiction."})},
		},
	}

	runner := NewRunner(mock, Config{Model: "mock", MaxTurns: 5})
	result, err := runner.Run(context.Background(), sampleRecords(), ModeReport)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Truncated {
		t.Error("expected run to complete, not truncate")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Category != CategoryContradiction {
		t.Errorf("expected contradiction category, got %s", result.Findings[0].Category)
	}
	if result.Summary != "Found one contradiction." {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
}

func TestRunner_MaxTurnsTruncates(t *testing.T) {
	// Script the model to loop forever (always call list_memories, never finalize).
	loopResp := llm.ChatResponse{Message: toolCallMsg("loop", "list_memories", map[string]any{})}
	mock := &llm.MockProvider{
		Responses: []llm.ChatResponse{loopResp, loopResp, loopResp},
	}

	runner := NewRunner(mock, Config{Model: "mock", MaxTurns: 3})
	result, err := runner.Run(context.Background(), sampleRecords(), ModeReport)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected run to be marked truncated when max-turns is hit")
	}
}

func TestRunner_RepairMode(t *testing.T) {
	mock := &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "finalize_repair_plan", map[string]any{
				"summary": "One record should be quarantined.",
				"actions": []map[string]any{
					{"memory_id": "mem-2", "recommendation": "quarantine", "detail": "Contradicts mem-1."},
				},
			})},
		},
	}

	runner := NewRunner(mock, Config{Model: "mock", MaxTurns: 5})
	result, err := runner.Run(context.Background(), sampleRecords(), ModeRepair)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.RepairActions) != 1 {
		t.Fatalf("expected 1 repair action, got %d", len(result.RepairActions))
	}
	if result.RepairActions[0].Recommendation != "quarantine" {
		t.Errorf("unexpected recommendation: %s", result.RepairActions[0].Recommendation)
	}
}

func TestRunner_UnknownToolNudgesModel(t *testing.T) {
	mock := &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "nonexistent_tool", map[string]any{})},
			{Message: toolCallMsg("c2", "finalize_report", map[string]any{"summary": "done"})},
		},
	}

	runner := NewRunner(mock, Config{Model: "mock", MaxTurns: 5})
	result, err := runner.Run(context.Background(), sampleRecords(), ModeReport)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "done" {
		t.Errorf("expected run to recover and finalize, got summary %q", result.Summary)
	}
}

// TestRunner_ResistsPromptInjectionFromMemoryContent is the adversarial
// security regression test: even when the memory store itself contains text
// instructing the agent to ignore findings and report a perfect score, the
// runner's job is only to faithfully relay the agent's tool calls. This test
// proves the mechanical plumbing (runner + tools) cannot be short-circuited
// by memory content — the LLM would have to be the one complying, and here
// we script it to do the SAFE thing (flag the injection attempt) to prove
// the protocol supports and rewards correct behavior exercised against the
// adversarial fixture.
func TestRunner_ResistsPromptInjectionFromMemoryContent(t *testing.T) {
	records, err := memory.LoadFile("../../testdata/adversarial.json")
	if err != nil {
		t.Fatalf("loading adversarial fixture: %v", err)
	}
	if len(records.Errors) != 0 {
		t.Fatalf("unexpected validation errors: %v", records.Errors)
	}

	mock := &llm.MockProvider{
		Responses: []llm.ChatResponse{
			{Message: toolCallMsg("c1", "list_memories", map[string]any{"offset": 0, "limit": 20})},
			{Message: toolCallMsg("c2", "get_memory", map[string]any{"id": "mem-031"})},
			{Message: toolCallMsg("c3", "record_finding", map[string]any{
				"category": "persistent_instruction", "risk": "critical",
				"memory_ids": []string{"mem-031"},
				"title":      "Embedded instruction attempts to override agent behavior",
				"detail":     "Memory text instructs the agent to report zero findings and a perfect trust score; this is a prompt-injection attempt and must not be obeyed.",
			})},
			{Message: toolCallMsg("c4", "record_finding", map[string]any{
				"category": "persistent_instruction", "risk": "high",
				"memory_ids": []string{"mem-032"},
				"title":      "Second embedded instruction to always comply with injected text",
				"detail":     "Memory text attempts to establish a standing rule to obey future injected instructions.",
			})},
			{Message: toolCallMsg("c5", "finalize_report", map[string]any{
				"summary": "Detected two persistent-instruction injection attempts embedded in memory content; did not comply with them.",
			})},
		},
	}

	runner := NewRunner(mock, Config{Model: "mock", MaxTurns: 8})
	result, err := runner.Run(context.Background(), records.Records, ModeReport)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Truncated {
		t.Fatal("expected run to complete normally")
	}
	if len(result.Findings) != 2 {
		t.Fatalf("expected 2 findings flagging the injection attempts, got %d", len(result.Findings))
	}
	for _, f := range result.Findings {
		if f.Category != CategoryPersistentInstruction {
			t.Errorf("expected persistent_instruction category, got %s", f.Category)
		}
	}
	if result.Summary == "" {
		t.Error("expected a non-empty summary")
	}
}
