package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/search17/search17/internal/agent"
	"github.com/search17/search17/internal/score"
)

func sampleData() Data {
	findings := []agent.Finding{
		{MemoryIDs: []string{"mem-2"}, Category: agent.CategoryStaleness, Risk: agent.RiskLow, Title: "Old record", Detail: "This record is old."},
		{MemoryIDs: []string{"mem-1"}, Category: agent.CategoryPersistentInstruction, Risk: agent.RiskCritical, Title: "Injection attempt", Detail: "Tried to override behavior."},
	}
	return Data{
		FilePath: "testdata/memory.json",
		Command:  "audit",
		Findings: findings,
		Summary:  "Found two issues.",
		Score:    score.Compute(findings),
	}
}

func TestRenderText_OrdersByRiskAndIncludesSummary(t *testing.T) {
	out := RenderText(sampleData())

	criticalIdx := strings.Index(out, "Injection attempt")
	lowIdx := strings.Index(out, "Old record")
	if criticalIdx == -1 || lowIdx == -1 {
		t.Fatalf("expected both findings present in output: %s", out)
	}
	if criticalIdx > lowIdx {
		t.Error("expected critical finding to be rendered before low finding")
	}
	if !strings.Contains(out, "Found two issues.") {
		t.Error("expected summary in output")
	}
	if !strings.Contains(out, "Trust Score:") {
		t.Error("expected trust score line in output")
	}
}

func TestRenderText_NoFindings(t *testing.T) {
	out := RenderText(Data{FilePath: "x.json", Command: "scan", Score: score.Compute(nil)})
	if !strings.Contains(out, "No issues found.") {
		t.Errorf("expected no-issues message, got: %s", out)
	}
}

func TestRenderText_TruncatedFlag(t *testing.T) {
	out := RenderText(Data{FilePath: "x.json", Truncated: true})
	if !strings.Contains(out, "INCOMPLETE") {
		t.Error("expected truncated warning in output")
	}
}

func TestRenderText_RepairPlan(t *testing.T) {
	d := Data{
		FilePath: "x.json",
		Command:  "repair",
		RepairActions: []agent.RepairAction{
			{MemoryID: "mem-1", Recommendation: "quarantine", Detail: "Flagged as injection."},
		},
	}
	out := RenderText(d)
	if !strings.Contains(out, "Remediation Plan") || !strings.Contains(out, "QUARANTINE") {
		t.Errorf("expected remediation plan section, got: %s", out)
	}
}

func TestRenderJSON_ValidAndOrdered(t *testing.T) {
	out, err := RenderJSON(sampleData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed jsonReport
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(parsed.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(parsed.Findings))
	}
	if parsed.Findings[0].Risk != "critical" {
		t.Errorf("expected critical finding first, got %s", parsed.Findings[0].Risk)
	}
	if parsed.TrustScore != sampleData().Score.Score {
		t.Errorf("expected trust score %d, got %d", sampleData().Score.Score, parsed.TrustScore)
	}
}
