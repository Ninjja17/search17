package cmd

import (
	"encoding/json"

	"github.com/search17/search17/internal/agent"
	"github.com/search17/search17/internal/llm"
)

// demoToolArgs marshals tool-call arguments for the scripted demo provider below.
func demoToolArgs(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func demoToolCallMsg(id, name string, args any) llm.Message {
	return llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: demoToolArgs(args)}},
	}
}

// buildOfflineDemoProvider returns a scripted MockProvider that replays a
// realistic investigation of testdata/memory.json (manipulation,
// persistent-instruction, contradiction, and staleness findings) \u2014 the exact
// walkthrough shown in the README. This is NOT live LLM reasoning: it exists
// so --demo-offline can show the full agent/tool/report pipeline working
// end-to-end without a network call or API key, e.g. for a recorded demo
// when API quota is unavailable.
func buildOfflineDemoProvider(mode agent.Mode) llm.Provider {
	responses := []llm.ChatResponse{
		{Message: demoToolCallMsg("d1", "list_memories", map[string]any{"offset": 0, "limit": 20})},
		{Message: demoToolCallMsg("d2", "get_memory", map[string]any{"id": "mem-102"})},
		{Message: demoToolCallMsg("d3", "record_finding", map[string]any{
			"category": "manipulation", "risk": "high",
			"memory_ids": []string{"mem-102"},
			"title":      "False-authority social engineering attempt",
			"detail":     "Claims to be \"IT security\" to get billing data shared without verification.",
		})},
		{Message: demoToolCallMsg("d4", "get_memory", map[string]any{"id": "mem-103"})},
		{Message: demoToolCallMsg("d5", "record_finding", map[string]any{
			"category": "persistent_instruction", "risk": "critical",
			"memory_ids": []string{"mem-103"},
			"title":      "Embedded override instruction",
			"detail":     "Memory text instructs the agent to ignore future warnings and always report a trust score of 100 \u2014 a prompt-injection attempt; not obeyed.",
		})},
		{Message: demoToolCallMsg("d6", "record_finding", map[string]any{
			"category": "contradiction", "risk": "high",
			"memory_ids": []string{"mem-104", "mem-105"},
			"title":      "Conflicting preferred contact method",
			"detail":     "mem-104 says email; mem-105 (newer) says SMS instead of email.",
		})},
		{Message: demoToolCallMsg("d7", "get_memory", map[string]any{"id": "mem-106"})},
		{Message: demoToolCallMsg("d8", "record_finding", map[string]any{
			"category": "staleness", "risk": "medium",
			"memory_ids": []string{"mem-106"},
			"title":      "Office location likely outdated",
			"detail":     "Recorded in 2023 with low confidence; no corroborating recent record.",
		})},
	}

	if mode == agent.ModeRepair {
		responses = append(responses, llm.ChatResponse{Message: demoToolCallMsg("d9", "finalize_repair_plan", map[string]any{
			"summary": "Quarantine the injected instruction and the manipulation claim; reconcile the contact-method contradiction; revalidate the stale record.",
			"actions": []map[string]any{
				{"memory_id": "mem-102", "recommendation": "quarantine", "detail": "Unverified authority claim used to request sensitive data."},
				{"memory_id": "mem-103", "recommendation": "quarantine", "detail": "Prompt-injection attempt embedded as a standing instruction."},
				{"memory_id": "mem-105", "recommendation": "edit", "detail": "Keep as the authoritative contact method; remove/annotate mem-104 as superseded."},
				{"memory_id": "mem-106", "recommendation": "revalidate", "detail": "Confirm current office location with HR before trusting further."},
			},
		})})
	} else {
		responses = append(responses, llm.ChatResponse{Message: demoToolCallMsg("d9", "finalize_report", map[string]any{
			"summary": "Detected a prompt-injection attempt, a social-engineering claim, a direct contradiction, and a stale fact.",
		})})
	}

	return &llm.MockProvider{Responses: responses}
}
