// Package memory defines the schema for AI agent memory records and
// utilities for loading and validating memory files.
package memory

import "time"

// Record is a single memory entry recorded by an AI agent.
type Record struct {
	ID         string    `json:"id"`
	Memory     string    `json:"memory"`
	Source     string    `json:"source"`
	Timestamp  time.Time `json:"timestamp"`
	Confidence float64   `json:"confidence"`
	MemoryType string    `json:"memory_type"`
}
