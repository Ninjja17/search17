package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/search17/search17/internal/agent"
	"github.com/search17/search17/internal/report"
)

var flagVerboseScore bool

var trustScoreCmd = &cobra.Command{
	Use:   "trust-score [file]",
	Short: "Run a full audit and print only the resulting trust score",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := runCommand("trust-score", args[0], agent.ModeReport, flagMaxTurns)
		if err != nil {
			return err
		}

		if flagJSON {
			out, err := report.RenderJSON(*data)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		}

		if flagVerboseScore {
			fmt.Printf("Trust Score: %d / 100 (critical=%d high=%d medium=%d low=%d)\n",
				data.Score.Score, data.Score.Critical, data.Score.High, data.Score.Medium, data.Score.Low)
		} else {
			fmt.Println(data.Score.Score)
		}
		return nil
	},
}

func init() {
	trustScoreCmd.Flags().BoolVar(&flagVerboseScore, "verbose", false, "include the risk breakdown alongside the score")
	rootCmd.AddCommand(trustScoreCmd)
}
