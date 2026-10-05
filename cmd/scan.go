package cmd

import (
	"github.com/spf13/cobra"

	"github.com/search17/search17/internal/agent"
)

// scanDefaultMaxTurns keeps scan fast/cheap unless the user explicitly
// overrides --max-turns.
const scanDefaultMaxTurns = 4

var scanCmd = &cobra.Command{
	Use:   "scan [file]",
	Short: "Fast, low-turn triage pass over a memory file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		maxTurns := flagMaxTurns
		if !cmd.Flags().Changed("max-turns") {
			maxTurns = scanDefaultMaxTurns
		}
		data, err := runCommand("scan", args[0], agent.ModeReport, maxTurns)
		if err != nil {
			return err
		}
		return printReport(data)
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
