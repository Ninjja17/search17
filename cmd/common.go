package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/search17/search17/internal/agent"
	"github.com/search17/search17/internal/llm"
	"github.com/search17/search17/internal/memory"
	"github.com/search17/search17/internal/report"
	"github.com/search17/search17/internal/score"
)

// newProvider builds the LLM provider used by commands. Tests override this
// with a mock provider so CLI integration tests never hit the network.
var newProvider = func(apiKey string) llm.Provider {
	return llm.NewOpenAIProvider(apiKey)
}

// runCommand loads a memory file, runs the agent over it, and assembles the
// resulting report Data. Shared by scan/audit/trust-score/repair, which only
// differ in Mode, max-turns, and how they render the result.
func runCommand(name string, filePath string, mode agent.Mode, maxTurns int) (*report.Data, error) {
	apiKey, err := resolveAPIKey()
	if err != nil {
		return nil, err
	}

	parsed, err := memory.LoadFile(filePath)
	if err != nil {
		return nil, err
	}
	for _, verr := range parsed.Errors {
		fmt.Fprintf(os.Stderr, "warning: skipping invalid record: %s\n", verr.Error())
	}
	if len(parsed.Records) == 0 {
		return nil, fmt.Errorf("no valid memory records to audit in %s", filePath)
	}

	provider := newProvider(apiKey)
	runner := agent.NewRunner(provider, agent.Config{Model: flagModel, MaxTurns: maxTurns, Temperature: 0.1})

	result, err := runner.Run(context.Background(), parsed.Records, mode)
	if err != nil {
		return nil, fmt.Errorf("agent run failed: %w", err)
	}

	return &report.Data{
		FilePath:      filePath,
		Command:       name,
		Findings:      result.Findings,
		Summary:       result.Summary,
		Truncated:     result.Truncated,
		Score:         score.Compute(result.Findings),
		RepairActions: result.RepairActions,
	}, nil
}

// printReport renders data as JSON or the boxed text report, per --json.
func printReport(data *report.Data) error {
	if flagJSON {
		out, err := report.RenderJSON(*data)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}
	fmt.Println(report.RenderText(*data))
	return nil
}
