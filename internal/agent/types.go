// Package agent implements Search17's LLM-driven ReAct-style agent: a
// tool-calling loop where the LLM is the reasoning core that decides what to
// inspect and when the audit is complete. The agent classifies; the rest of
// the system (trust scorer, reporter) only renders what it decided.
package agent

// Risk is the severity level assigned to a Finding by the agent.
type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

// Category is one of the four threat categories Search17 looks for.
type Category string

const (
	CategoryManipulation          Category = "manipulation"
	CategoryPersistentInstruction Category = "persistent_instruction"
	CategoryContradiction         Category = "contradiction"
	CategoryStaleness             Category = "staleness"
)

// Mode selects which terminal tool is available to the agent, and therefore
// what kind of run this is (an audit report vs. a repair plan).
type Mode string

const (
	ModeReport Mode = "report"
	ModeRepair Mode = "repair"
)

// Finding is a single issue the agent identified in the memory store.
type Finding struct {
	MemoryIDs []string
	Category  Category
	Title     string
	Risk      Risk
	Detail    string
}

// RepairAction is a single remediation recommendation for one memory record,
// produced in Mode Repair. Repair runs are print-only: they never mutate the
// input file.
type RepairAction struct {
	MemoryID       string
	Recommendation string // "quarantine", "edit", or "revalidate"
	Detail         string
}

// Config controls how the agent runner drives the LLM.
type Config struct {
	Model       string
	MaxTurns    int
	Temperature float64

	// Optional observability hooks for a caller-side progress UI (e.g. a CLI
	// spinner). Both are nil-safe no-ops when unset and never affect what
	// the agent decides \u2014 purely for display.
	OnTurnStart func(turn int)                            // called before each provider call
	OnToolCall  func(turn int, toolName, argsJSON string) // called after each dispatched tool call
}

// RunResult is the outcome of a completed (or forcibly stopped) agent run.
type RunResult struct {
	Findings      []Finding
	RepairActions []RepairAction
	Summary       string
	Truncated     bool // true if MaxTurns was hit before a finalize tool was called
}
