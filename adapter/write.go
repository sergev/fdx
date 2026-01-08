package adapter

import (
	"fmt"

	"github.com/spf13/cobra"
)

var writeCmd = &cobra.Command{
	Use:   "write FILE",
	Short: "Write data to the floppy disk",
	Long:  "Write data from FILE to the floppy disk.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Determine input filename
		filename := "image.hfe"
		if len(args) > 0 {
			filename = args[0]
		}

		// Write floppy disk using adapter interface
		err := floppyAdapter.Write(filename)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write floppy disk: %w", err))
		}

		fmt.Printf("Successfully imaged floppy disk from %s\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(writeCmd)
}
