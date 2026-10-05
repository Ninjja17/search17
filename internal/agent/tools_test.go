package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/search17/search17/internal/memory"
)

func newTestContext() *RunContext {
	return &RunContext{
		Records: []memory.Record{
			{ID: "mem-1", Memory: "This is a fairly long memory text used to test preview truncation behavior in list_memories.", Source: "chat", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Confidence: 0.8, MemoryType: "fact"},
			{ID: "mem-2", Memory: "Second memory.", Source: "chat", Timestamp: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Confidence: 0.7, MemoryType: "fact"},
		},
		Clock: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) },
	}
}

func TestHandleGetCurrentDate(t *testing.T) {
	rc := newTestContext()
	out, err := handleGetCurrentDate(rc, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "2026-10-04") {
		t.Errorf("expected date in output, got %s", out)
	}
}

func TestHandleListMemories_Pagination(t *testing.T) {
	rc := newTestContext()
	out, err := handleListMemories(rc, `{"offset":0,"limit":1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if int(parsed["total"].(float64)) != 2 {
		t.Errorf("expected total 2, got %v", parsed["total"])
	}
	records := parsed["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("expected 1 record on page, got %d", len(records))
	}
}

func TestHandleGetMemory_NotFound(t *testing.T) {
	rc := newTestContext()
	out, err := handleGetMemory(rc, `{"id":"does-not-exist"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "error") {
		t.Errorf("expected error payload, got %s", out)
	}
}

func TestHandleGetMemory_Found(t *testing.T) {
	rc := newTestContext()
	out, err := handleGetMemory(rc, `{"id":"mem-2"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Second memory.") {
		t.Errorf("expected full memory text, got %s", out)
	}
}

func TestHandleRecordFinding_InvalidCategory(t *testing.T) {
	rc := newTestContext()
	out, err := handleRecordFinding(rc, `{"category":"bogus","risk":"low","memory_ids":["mem-1"],"title":"t","detail":"d"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "error") {
		t.Errorf("expected validation error payload, got %s", out)
	}
	if len(rc.Findings) != 0 {
		t.Error("invalid finding should not be recorded")
	}
}

func TestHandleRecordFinding_Valid(t *testing.T) {
	rc := newTestContext()
	_, err := handleRecordFinding(rc, `{"category":"staleness","risk":"low","memory_ids":["mem-1"],"title":"t","detail":"d"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rc.Findings) != 1 {
		t.Fatalf("expected 1 finding recorded, got %d", len(rc.Findings))
	}
}

func TestHandleFinalizeReport(t *testing.T) {
	rc := newTestContext()
	_, err := handleFinalizeReport(rc, `{"summary":"all good"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rc.Done || rc.Summary != "all good" {
		t.Errorf("expected Done=true and summary set, got Done=%v Summary=%q", rc.Done, rc.Summary)
	}
}

func TestHandleFinalizeRepairPlan_InvalidRecommendation(t *testing.T) {
	rc := newTestContext()
	out, err := handleFinalizeRepairPlan(rc, `{"summary":"s","actions":[{"memory_id":"mem-1","recommendation":"delete","detail":"d"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "error") {
		t.Errorf("expected validation error for bad recommendation, got %s", out)
	}
	if rc.Done {
		t.Error("should not finalize on invalid input")
	}
}
