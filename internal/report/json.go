package report

import "encoding/json"

type jsonFinding struct {
	MemoryIDs []string `json:"memory_ids"`
	Category  string   `json:"category"`
	Risk      string   `json:"risk"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
}

type jsonRepairAction struct {
	MemoryID       string `json:"memory_id"`
	Recommendation string `json:"recommendation"`
	Detail         string `json:"detail"`
}

type jsonReport struct {
	File           string             `json:"file"`
	Command        string             `json:"command"`
	TrustScore     int                `json:"trust_score"`
	ScoreBreakdown map[string]int     `json:"score_breakdown"`
	Findings       []jsonFinding      `json:"findings"`
	RepairActions  []jsonRepairAction `json:"repair_actions,omitempty"`
	Summary        string             `json:"summary"`
	Truncated      bool               `json:"truncated"`
}

// RenderJSON renders d as an indented JSON document.
func RenderJSON(d Data) (string, error) {
	out := jsonReport{
		File:       d.FilePath,
		Command:    d.Command,
		TrustScore: d.Score.Score,
		ScoreBreakdown: map[string]int{
			"critical": d.Score.Critical,
			"high":     d.Score.High,
			"medium":   d.Score.Medium,
			"low":      d.Score.Low,
		},
		Summary:   d.Summary,
		Truncated: d.Truncated,
	}
	for _, f := range sortedFindings(d.Findings) {
		out.Findings = append(out.Findings, jsonFinding{
			MemoryIDs: f.MemoryIDs,
			Category:  string(f.Category),
			Risk:      string(f.Risk),
			Title:     f.Title,
			Detail:    f.Detail,
		})
	}
	for _, a := range d.RepairActions {
		out.RepairActions = append(out.RepairActions, jsonRepairAction{
			MemoryID:       a.MemoryID,
			Recommendation: a.Recommendation,
			Detail:         a.Detail,
		})
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
