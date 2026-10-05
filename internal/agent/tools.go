package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/search17/search17/internal/llm"
	"github.com/search17/search17/internal/memory"
)

// RunContext is the per-run scratchpad shared between the agent runner and
// its tool handlers. The LLM never sees this struct directly — only through
// tool call arguments and results. Tools are deliberately limited to
// mechanics and grounding (date, data access, recording what the agent
// decided); the LLM performs all semantic classification itself.
type RunContext struct {
	Records       []memory.Record
	Mode          Mode
	Findings      []Finding
	RepairActions []RepairAction
	Summary       string
	Done          bool // set by a finalize_* tool call; ends the runner loop
	Clock         func() time.Time
}

func (rc *RunContext) now() time.Time {
	if rc.Clock != nil {
		return rc.Clock()
	}
	return time.Now()
}

func (rc *RunContext) findRecord(id string) (memory.Record, bool) {
	for _, r := range rc.Records {
		if r.ID == id {
			return r, true
		}
	}
	return memory.Record{}, false
}

// ToolHandler executes one tool call against the shared RunContext and
// returns the raw JSON string to send back to the model as the tool result.
// A non-nil error indicates an unrecoverable failure (not a user-facing
// validation issue, which should instead be returned as a JSON error payload
// so the model can self-correct).
type ToolHandler func(rc *RunContext, rawArgs string) (string, error)

// ToolSet bundles tool JSON-schema definitions with their Go handlers.
type ToolSet struct {
	Defs     []llm.ToolDef
	Handlers map[string]ToolHandler
}

const previewLen = 160

// BuildToolSet returns the tools available to the agent for the given Mode.
// Investigation tools (date, pagination, lookup, recording findings) are
// always available; the terminal tool differs by mode.
func BuildToolSet(mode Mode) *ToolSet {
	defs := []llm.ToolDef{
		{
			Name:        "get_current_date",
			Description: "Returns today's date in RFC3339 format. Use this as ground truth when judging whether a memory is stale.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "list_memories",
			Description: "Lists a page of memory records (id, source, timestamp, memory_type, confidence, and a short text preview). Use offset/limit to page through large stores instead of requesting everything at once.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"offset": map[string]any{"type": "integer", "description": "0-based starting index"},
					"limit":  map[string]any{"type": "integer", "description": "max records to return (default 20)"},
				},
			},
		},
		{
			Name:        "get_memory",
			Description: "Fetches the full content of a single memory record by id for closer inspection.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "record_finding",
			Description: "Records one confirmed issue found during the audit. Call this once per distinct issue as you find it; do not wait until the end to batch them.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category":   map[string]any{"type": "string", "enum": []string{"manipulation", "persistent_instruction", "contradiction", "staleness"}},
					"risk":       map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
					"memory_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"title":      map[string]any{"type": "string"},
					"detail":     map[string]any{"type": "string"},
				},
				"required": []string{"category", "risk", "memory_ids", "title", "detail"},
			},
		},
	}

	handlers := map[string]ToolHandler{
		"get_current_date": handleGetCurrentDate,
		"list_memories":    handleListMemories,
		"get_memory":       handleGetMemory,
		"record_finding":   handleRecordFinding,
	}

	if mode == ModeRepair {
		defs = append(defs, llm.ToolDef{
			Name:        "finalize_repair_plan",
			Description: "Ends the run and submits the final remediation plan. Call this exactly once, after you've reviewed all flagged issues.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{"type": "string"},
					"actions": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"memory_id":      map[string]any{"type": "string"},
								"recommendation": map[string]any{"type": "string", "enum": []string{"quarantine", "edit", "revalidate"}},
								"detail":         map[string]any{"type": "string"},
							},
							"required": []string{"memory_id", "recommendation", "detail"},
						},
					},
				},
				"required": []string{"summary", "actions"},
			},
		})
		handlers["finalize_repair_plan"] = handleFinalizeRepairPlan
	} else {
		defs = append(defs, llm.ToolDef{
			Name:        "finalize_report",
			Description: "Ends the run and submits the final audit summary. Call this exactly once, after you've finished investigating.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{"type": "string"},
				},
				"required": []string{"summary"},
			},
		})
		handlers["finalize_report"] = handleFinalizeReport
	}

	return &ToolSet{Defs: defs, Handlers: handlers}
}

// TerminalToolName returns the name of the tool that ends the run for mode.
func TerminalToolName(mode Mode) string {
	if mode == ModeRepair {
		return "finalize_repair_plan"
	}
	return "finalize_report"
}

func jsonError(format string, args ...any) string {
	out, _ := json.Marshal(map[string]string{"error": fmt.Sprintf(format, args...)})
	return string(out)
}

func handleGetCurrentDate(rc *RunContext, _ string) (string, error) {
	out, err := json.Marshal(map[string]string{"date": rc.now().UTC().Format(time.RFC3339)})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type listMemoriesArgs struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type memorySummary struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	Timestamp  string  `json:"timestamp"`
	MemoryType string  `json:"memory_type"`
	Confidence float64 `json:"confidence"`
	Preview    string  `json:"preview"`
}

func handleListMemories(rc *RunContext, rawArgs string) (string, error) {
	var args listMemoriesArgs
	if rawArgs != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return jsonError("invalid arguments: %v", err), nil
		}
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}
	if args.Offset < 0 {
		args.Offset = 0
	}

	total := len(rc.Records)
	start := args.Offset
	if start > total {
		start = total
	}
	end := start + args.Limit
	if end > total {
		end = total
	}

	page := make([]memorySummary, 0, end-start)
	for _, r := range rc.Records[start:end] {
		preview := r.Memory
		if len(preview) > previewLen {
			preview = preview[:previewLen] + "..."
		}
		page = append(page, memorySummary{
			ID:         r.ID,
			Source:     r.Source,
			Timestamp:  r.Timestamp.UTC().Format(time.RFC3339),
			MemoryType: r.MemoryType,
			Confidence: r.Confidence,
			Preview:    preview,
		})
	}

	out, err := json.Marshal(map[string]any{
		"total":   total,
		"offset":  start,
		"records": page,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type getMemoryArgs struct {
	ID string `json:"id"`
}

func handleGetMemory(rc *RunContext, rawArgs string) (string, error) {
	var args getMemoryArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return jsonError("invalid arguments: %v", err), nil
	}
	rec, ok := rc.findRecord(args.ID)
	if !ok {
		return jsonError("no memory record with id %q", args.ID), nil
	}
	out, err := json.Marshal(map[string]any{
		"id":          rec.ID,
		"memory":      rec.Memory,
		"source":      rec.Source,
		"timestamp":   rec.Timestamp.UTC().Format(time.RFC3339),
		"confidence":  rec.Confidence,
		"memory_type": rec.MemoryType,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type recordFindingArgs struct {
	Category  string   `json:"category"`
	Risk      string   `json:"risk"`
	MemoryIDs []string `json:"memory_ids"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
}

var validCategories = map[string]bool{
	string(CategoryManipulation):          true,
	string(CategoryPersistentInstruction): true,
	string(CategoryContradiction):         true,
	string(CategoryStaleness):             true,
}

var validRisks = map[string]bool{
	string(RiskLow): true, string(RiskMedium): true, string(RiskHigh): true, string(RiskCritical): true,
}

func handleRecordFinding(rc *RunContext, rawArgs string) (string, error) {
	var args recordFindingArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return jsonError("invalid arguments: %v", err), nil
	}
	if !validCategories[args.Category] {
		return jsonError("invalid category %q", args.Category), nil
	}
	if !validRisks[args.Risk] {
		return jsonError("invalid risk %q", args.Risk), nil
	}
	if len(args.MemoryIDs) == 0 {
		return jsonError("memory_ids must not be empty"), nil
	}
	if args.Title == "" || args.Detail == "" {
		return jsonError("title and detail are required"), nil
	}

	rc.Findings = append(rc.Findings, Finding{
		MemoryIDs: args.MemoryIDs,
		Category:  Category(args.Category),
		Title:     args.Title,
		Risk:      Risk(args.Risk),
		Detail:    args.Detail,
	})

	out, _ := json.Marshal(map[string]any{"status": "recorded", "total_findings": len(rc.Findings)})
	return string(out), nil
}

type finalizeReportArgs struct {
	Summary string `json:"summary"`
}

func handleFinalizeReport(rc *RunContext, rawArgs string) (string, error) {
	var args finalizeReportArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return jsonError("invalid arguments: %v", err), nil
	}
	rc.Summary = args.Summary
	rc.Done = true
	out, _ := json.Marshal(map[string]string{"status": "finalized"})
	return string(out), nil
}

type finalizeRepairPlanArgs struct {
	Summary string `json:"summary"`
	Actions []struct {
		MemoryID       string `json:"memory_id"`
		Recommendation string `json:"recommendation"`
		Detail         string `json:"detail"`
	} `json:"actions"`
}

var validRecommendations = map[string]bool{"quarantine": true, "edit": true, "revalidate": true}

func handleFinalizeRepairPlan(rc *RunContext, rawArgs string) (string, error) {
	var args finalizeRepairPlanArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return jsonError("invalid arguments: %v", err), nil
	}
	for _, a := range args.Actions {
		if !validRecommendations[a.Recommendation] {
			return jsonError("invalid recommendation %q for memory_id %q", a.Recommendation, a.MemoryID), nil
		}
	}

	rc.Summary = args.Summary
	for _, a := range args.Actions {
		rc.RepairActions = append(rc.RepairActions, RepairAction{
			MemoryID:       a.MemoryID,
			Recommendation: a.Recommendation,
			Detail:         a.Detail,
		})
	}
	rc.Done = true
	out, _ := json.Marshal(map[string]string{"status": "finalized"})
	return string(out), nil
}
