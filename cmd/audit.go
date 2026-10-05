package cmd

import (
	"github.com/spf13/cobra"

	"github.com/search17/search17/internal/agent"
)

var auditCmd = &cobra.Command{
	Use:   "audit [file]",
	Short: "Full investigative agent run producing a complete findings report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := runCommand("audit", args[0], agent.ModeReport, flagMaxTurns)
		if err != nil {
			return err
		}
		return printReport(data)
	},
}

func init() {
	rootCmd.AddCommand(auditCmd)
}
