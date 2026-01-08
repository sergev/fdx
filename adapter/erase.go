package adapter

import (
	"fmt"

	"github.com/spf13/cobra"
)

var eraseCmd = &cobra.Command{
	Use:   "erase",
	Short: "Erase the floppy disk",
	Long:  "Erase the floppy disk connected via USB adapter.",
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Read floppy disk using adapter interface
		err := floppyAdapter.Erase()
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to erase floppy disk: %w", err))
		}

		fmt.Printf("Successfully erased floppy disk\n")
	},
}

func init() {
	rootCmd.AddCommand(eraseCmd)
}
