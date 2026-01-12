package adapter

import (
	"fmt"

	"github.com/sergev/floppy/hfe"
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
		filename := "image.hfe"
		if len(args) > 0 {
			filename = args[0]
		}

		// Read floppy disk using adapter interface
		disk, err := floppyAdapter.Read(82)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read floppy disk: %w", err))
		}

		// Write file
		fmt.Printf("Writing file...\n")
		err = hfe.Write(filename, disk, hfe.HFEVersion1)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write file: %w", err))
		}

		fmt.Printf("Successfully read floppy disk to %s\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(readCmd)
}
