package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read [FILE]",
	Short: "Read data from the floppy disk",
	Long:  "Read data from the floppy disk. Optionally specify a FILE to read.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Determine output filename
		filename := "floppy_raw.bin"
		if len(args) > 0 {
			filename = args[0]
		}

		// Read floppy disk using adapter interface
		err := floppyAdapter.Read(filename)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read floppy disk: %w", err))
		}

		fmt.Printf("Successfully read floppy disk to %s\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(readCmd)
}

