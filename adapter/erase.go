package adapter

import (
	"bufio"
	"fmt"
	"os"

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

		// Prompt user to insert diskette
		fmt.Print("Insert TARGET diskette in drive\nand press Enter when ready...")
		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')
		fmt.Printf("\n")

		// Read floppy disk using adapter interface
		err := floppyAdapter.Erase(82)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to erase floppy disk: %w", err))
		}
	},
}

func init() {
	rootCmd.AddCommand(eraseCmd)
}
