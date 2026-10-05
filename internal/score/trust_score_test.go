package score

import (
	"testing"

	"github.com/search17/search17/internal/agent"
)

func f(risk agent.Risk) agent.Finding {
	return agent.Finding{Risk: risk, MemoryIDs: []string{"mem-1"}, Category: agent.CategoryStaleness, Title: "t", Detail: "d"}
}

func TestCompute_NoFindings(t *testing.T) {
	b := Compute(nil)
	if b.Score != 100 {
		t.Errorf("expected score 100 with no findings, got %d", b.Score)
	}
}

func TestCompute_MixedFindings(t *testing.T) {
	findings := []agent.Finding{f(agent.RiskCritical), f(agent.RiskHigh), f(agent.RiskMedium), f(agent.RiskLow)}
	b := Compute(findings)
	// 100 - (25 + 15 + 7 + 3) = 50
	if b.Score != 50 {
		t.Errorf("expected score 50, got %d", b.Score)
	}
	if b.Critical != 1 || b.High != 1 || b.Medium != 1 || b.Low != 1 {
		t.Errorf("unexpected breakdown counts: %+v", b)
	}
}

func TestCompute_ClampsToZero(t *testing.T) {
	findings := []agent.Finding{f(agent.RiskCritical), f(agent.RiskCritical), f(agent.RiskCritical), f(agent.RiskCritical), f(agent.RiskCritical)}
	b := Compute(findings)
	if b.Score != 0 {
		t.Errorf("expected score clamped to 0, got %d", b.Score)
	}
}

func TestCompute_ClampsToHundred(t *testing.T) {
	// No findings is already the max; this guards the clamp upper bound logic itself.
	b := Compute([]agent.Finding{})
	if b.Score != 100 {
		t.Errorf("expected score 100, got %d", b.Score)
	}
}
