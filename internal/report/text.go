// Package report renders a completed agent RunResult plus trust score as
// either Search17's human-readable boxed report or machine-readable JSON.
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/search17/search17/internal/agent"
	"github.com/search17/search17/internal/score"
)

// Data is everything the reporter needs to render one command's output.
type Data struct {
	FilePath      string
	Command       string // "scan", "audit", "trust-score", or "repair"
	Findings      []agent.Finding
	Summary       string
	Truncated     bool
	Score         score.Breakdown
	RepairActions []agent.RepairAction
}

var riskRank = map[agent.Risk]int{
	agent.RiskCritical: 0,
	agent.RiskHigh:     1,
	agent.RiskMedium:   2,
	agent.RiskLow:      3,
}

func sortedFindings(findings []agent.Finding) []agent.Finding {
	out := make([]agent.Finding, len(findings))
	copy(out, findings)
	sort.SliceStable(out, func(i, j int) bool {
		return riskRank[out[i].Risk] < riskRank[out[j].Risk]
	})
	return out
}

const boxWidth = 60

func boxLine(text string) string {
	if len(text) > boxWidth {
		text = text[:boxWidth]
	}
	return fmt.Sprintf("║ %-*s ║", boxWidth, text)
}

// RenderText renders d as Search17's human-readable boxed report.
func RenderText(d Data) string {
	var b strings.Builder

	title := "Search17 Audit Report"
	fmt.Fprintln(&b, "╔"+strings.Repeat("═", boxWidth+2)+"╗")
	fmt.Fprintln(&b, boxLine(title))
	fmt.Fprintln(&b, "╚"+strings.Repeat("═", boxWidth+2)+"╝")
	fmt.Fprintf(&b, "File:        %s\n", d.FilePath)
	fmt.Fprintf(&b, "Trust Score: %d / 100 (critical=%d high=%d medium=%d low=%d)\n",
		d.Score.Score, d.Score.Critical, d.Score.High, d.Score.Medium, d.Score.Low)

	findings := sortedFindings(d.Findings)
	fmt.Fprintf(&b, "\nFindings (%d)\n", len(findings))
	fmt.Fprintln(&b, strings.Repeat("─", 40))
	if len(findings) == 0 {
		fmt.Fprintln(&b, "No issues found.")
	}
	for _, f := range findings {
		fmt.Fprintf(&b, "[%s] %s — %s\n", strings.ToUpper(string(f.Risk)), f.Category, f.Title)
		fmt.Fprintf(&b, "  Memories: %s\n", strings.Join(f.MemoryIDs, ", "))
		fmt.Fprintf(&b, "  %s\n\n", f.Detail)
	}

	if d.Command == "repair" {
		fmt.Fprintf(&b, "Remediation Plan (%d action(s)) — print-only, no files were modified\n", len(d.RepairActions))
		fmt.Fprintln(&b, strings.Repeat("─", 40))
		for _, a := range d.RepairActions {
			fmt.Fprintf(&b, "[%s] memory=%s\n  %s\n\n", strings.ToUpper(a.Recommendation), a.MemoryID, a.Detail)
		}
	}

	if d.Summary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", d.Summary)
	}
	if d.Truncated {
		fmt.Fprintln(&b, "\n⚠ INCOMPLETE: max-turns limit reached before the agent finished investigating. Results above may be partial.")
	}

	return b.String()
}
