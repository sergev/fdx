package adapter

import (
	"fmt"

	"github.com/sergev/floppy/hfe"
	"github.com/spf13/cobra"
)

var convertCmd = &cobra.Command{
	Use:   "convert SRC.EXT DEST.EXT",
	Short: "Convert between image formats",
	Long: `Convert between image formats.
Reads contents of the SRC.EXT file and writes it to DEST.EXT file.
Format of floppy image is defined by extension.
USB adapter is not used.
` + supportedImageFormatsText,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		srcFilename := args[0]
		destFilename := args[1]

		// Read source file
		disk, err := hfe.Read(srcFilename)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read file %s: %w", srcFilename, err))
		}

		// Write destination file
		err = hfe.Write(destFilename, disk)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write file %s: %w", destFilename, err))
		}

		fmt.Printf("Successfully converted %s to %s\n", srcFilename, destFilename)
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)
}
