package adapter

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the floppy controller",
	Long:  "Check the status of the USB floppy disk controller.",
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Print status information
		floppyAdapter.PrintStatus()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
