// Package score computes Search17's deterministic trust score from the
// agent's recorded findings. The agent classifies; this package only does
// arithmetic, so the score stays auditable and reproducible across runs.
package score

import "github.com/search17/search17/internal/agent"

// Breakdown is the per-risk finding counts plus the resulting score.
type Breakdown struct {
	Critical int
	High     int
	Medium   int
	Low      int
	Score    int
}

// Compute derives a Breakdown from a set of findings using the formula:
// score = clamp(100 - (critical*25 + high*15 + medium*7 + low*3), 0, 100)
func Compute(findings []agent.Finding) Breakdown {
	var b Breakdown
	for _, f := range findings {
		switch f.Risk {
		case agent.RiskCritical:
			b.Critical++
		case agent.RiskHigh:
			b.High++
		case agent.RiskMedium:
			b.Medium++
		case agent.RiskLow:
			b.Low++
		}
	}
	raw := 100 - (b.Critical*25 + b.High*15 + b.Medium*7 + b.Low*3)
	b.Score = clamp(raw, 0, 100)
	return b
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
