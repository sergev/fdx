package adapter

import (
	"fmt"

	"github.com/sergev/floppy/config"
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

		fmt.Printf("\nConfiguration script: ~/.floppy\n")
		fmt.Printf("Floppy Drive: %s\n", config.DriveName)
		fmt.Printf("Geometry: %d tracks, %d side(s)\n", config.Cyls, config.Heads)
		fmt.Printf("Speed: %d RPM, max %d kbps\n", config.RPM, config.MaxKBps)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
