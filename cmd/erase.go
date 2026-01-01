package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var eraseCmd = &cobra.Command{
	Use:   "erase",
	Short: "Erase the floppy disk",
	Long:  "Erase the floppy disk connected via USB adapter.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("erase: not yet implemented")
	},
}

func init() {
	rootCmd.AddCommand(eraseCmd)
}

