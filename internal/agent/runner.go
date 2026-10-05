package agent

import (
	"context"
	"fmt"

	"github.com/search17/search17/internal/llm"
	"github.com/search17/search17/internal/memory"
)

const defaultMaxTurns = 12

// Runner drives the agentic ReAct loop: it repeatedly asks the Provider for
// the next step, dispatches whatever tool calls come back, and feeds the
// results back in, until the agent calls its finalize tool or MaxTurns is
// reached. There is no fixed sequence of checks — the LLM decides what to
// inspect and when it is done.
type Runner struct {
	Provider llm.Provider
	Config   Config
}

// NewRunner constructs a Runner. Config.MaxTurns defaults to 12 if <= 0.
func NewRunner(provider llm.Provider, cfg Config) *Runner {
	return &Runner{Provider: provider, Config: cfg}
}

// Run executes one full agent run over records in the given Mode.
func (r *Runner) Run(ctx context.Context, records []memory.Record, mode Mode) (*RunResult, error) {
	toolSet := BuildToolSet(mode)
	rc := &RunContext{Records: records, Mode: mode}

	maxTurns := r.Config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: BuildSystemPrompt(mode)},
		{
			Role: llm.RoleUser,
			Content: fmt.Sprintf(
				"Audit this memory store. It contains %d record(s). Investigate using your tools, "+
					"record each issue you confirm, and call %s when you are finished.",
				len(records), TerminalToolName(mode),
			),
		},
	}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := r.Provider.CreateChatCompletion(ctx, llm.ChatRequest{
			Model:       r.Config.Model,
			Messages:    messages,
			Tools:       toolSet.Defs,
			Temperature: r.Config.Temperature,
		})
		if err != nil {
			return nil, fmt.Errorf("agent runner: provider call failed on turn %d: %w", turn, err)
		}

		messages = append(messages, resp.Message)

		if len(resp.Message.ToolCalls) == 0 {
			// Model responded without acting or finishing; nudge it back on protocol.
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: fmt.Sprintf("Continue investigating with your tools, or call %s if you are done.", TerminalToolName(mode)),
			})
			continue
		}

		for _, tc := range resp.Message.ToolCalls {
			result := dispatchTool(toolSet, rc, tc)
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		if rc.Done {
			return &RunResult{
				Findings:      rc.Findings,
				RepairActions: rc.RepairActions,
				Summary:       rc.Summary,
				Truncated:     false,
			}, nil
		}
	}

	return &RunResult{
		Findings:      rc.Findings,
		RepairActions: rc.RepairActions,
		Summary:       rc.Summary,
		Truncated:     true,
	}, nil
}

func dispatchTool(toolSet *ToolSet, rc *RunContext, tc llm.ToolCall) string {
	handler, ok := toolSet.Handlers[tc.Name]
	if !ok {
		return jsonError("unknown tool %q", tc.Name)
	}
	result, err := handler(rc, tc.Arguments)
	if err != nil {
		return jsonError("tool %q failed: %v", tc.Name, err)
	}
	return result
}
