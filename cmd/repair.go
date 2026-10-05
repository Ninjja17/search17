package cmd

import (
	"github.com/spf13/cobra"

	"github.com/search17/search17/internal/agent"
)

var repairCmd = &cobra.Command{
	Use:   "repair [file]",
	Short: "Audit a memory file and print remediation recommendations (never modifies the file)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := runCommand("repair", args[0], agent.ModeRepair, flagMaxTurns)
		if err != nil {
			return err
		}
		return printReport(data)
	},
}

func init() {
	rootCmd.AddCommand(repairCmd)
}
