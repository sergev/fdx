package cmd

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
		fmt.Printf("write: not yet implemented (FILE: %s)\n", args[0])
	},
}

func init() {
	rootCmd.AddCommand(writeCmd)
}

