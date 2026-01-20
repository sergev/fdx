package adapter

import (
	"bufio"
	"fmt"
	"os"

	"github.com/sergev/floppy/config"
	"github.com/sergev/floppy/hfe"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read [DEST.EXT]",
	Short: "Read image of the floppy disk",
	Long: `Read the floppy disk and save image to file DEST.EXT.
Format of floppy image is defined by extension.
By default the floppy image is saved in HDE format as 'image.hde'.
` + supportedImageFormatsText,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Determine output filename
		filename := "image.hfe"
		if len(args) > 0 {
			filename = args[0]
		}

		// Compute number of cylinders to read
		cylinders := config.Cyls
		switch hfe.DetectImageFormat(filename) {
		case hfe.ImageFormatUnknown:
			cobra.CheckErr(fmt.Errorf("unknown image format: %s", filename))
		case hfe.ImageFormatHFE:
			// For HFE, read two extra cylinders
			cylinders += 2
		}
		fmt.Printf("Reading %d tracks, %d side(s)\n", cylinders, config.Heads)
		fmt.Printf("\n")

		// Prompt user to insert diskette
		fmt.Print("Insert SOURCE diskette in drive\nand press Enter when ready...")
		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')
		fmt.Printf("\n")

		// Read floppy disk using adapter interface
		disk, err := floppyAdapter.Read(cylinders)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read floppy disk: %w", err))
		}

		// Write file
		err = hfe.Write(filename, disk)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write file: %w", err))
		}
		fmt.Printf("\n")
		fmt.Printf("Image from diskette saved to file '%s'.\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(readCmd)
}
