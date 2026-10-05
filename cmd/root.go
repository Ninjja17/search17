package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global flags shared by all subcommands.
var (
	flagJSON     bool
	flagModel    string
	flagMaxTurns int
	flagAPIKey   string
)

var rootCmd = &cobra.Command{
	Use:   "search17",
	Short: "Search17 — a Trust & Safety agent for AI agent memory files",
	Long: `Search17 is an LLM-driven agent that audits AI agent memory files for
manipulation, persistent-instruction attacks, contradictions, and staleness.`,
	SilenceUsage: true,
}

// Execute runs the root command and returns any error encountered.
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion sets the version string reported by `search17 --version`.
func SetVersion(v string) {
	rootCmd.Version = v
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output machine-readable JSON instead of the boxed report")
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "gpt-4o-mini", "LLM model to use for the agent")
	rootCmd.PersistentFlags().IntVar(&flagMaxTurns, "max-turns", 12, "maximum ReAct loop turns before forced stop")
	rootCmd.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "OpenAI API key (defaults to OPENAI_API_KEY env var)")
}

// resolveAPIKey returns the configured API key, falling back to the
// OPENAI_API_KEY environment variable, with a clear error if neither is set.
func resolveAPIKey() (string, error) {
	if flagAPIKey != "" {
		return flagAPIKey, nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no API key configured: set --api-key or the OPENAI_API_KEY environment variable")
}
