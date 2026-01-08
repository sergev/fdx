package adapter

import (
	"fmt"

	"github.com/spf13/cobra"
)

var formatCmd = &cobra.Command{
	Use:   "format",
	Short: "Format the floppy disk",
	Long:  "Format the floppy disk connected via USB adapter.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("format: not yet implemented")
	},
}

func init() {
	rootCmd.AddCommand(formatCmd)
}
