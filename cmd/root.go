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
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "gemini-3.8-flash", "Gemini model to use for the agent")
	rootCmd.PersistentFlags().IntVar(&flagMaxTurns, "max-turns", 12, "maximum ReAct loop turns before forced stop")
	rootCmd.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "Gemini API key (defaults to GEMINI_API_KEY env var)")
}

// resolveAPIKey returns the configured Gemini API key, falling back to the
// environment variable, with a clear error if neither is set.
func resolveAPIKey() (string, error) {
	if flagAPIKey != "" {
		return flagAPIKey, nil
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no API key configured: set --api-key or the GEMINI_API_KEY environment variable")
}
