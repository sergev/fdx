package adapter

import (
	"bufio"
	"fmt"
	"os"

	"github.com/sergev/floppy/hfe"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read [DEST.EXT]",
	Short: "Read image of the floppy disk",
	Long: `Read the floppy disk and save image to file DEST.EXT.
Format of floppy image is defined by extension.
By default the floppy image is saved in HDE format as 'image.hde'.
Supported image formats:
	adf        - Amiga Disk File
    hde        - HxC Floppy Emulator
    img or ima - raw binary contents of the entire disk`,
	// TODO: bkd        - BK-0010/0011M Disk image
	// TODO: cp2        - Central Point Software's Copy-II-PC
	// TODO: dcf        - Disk Copy Fast utility
	// TODO: epl        - EPLCopy utility
	// TODO: imd        - Dave Dunfield's ImageDisk utility
	// TODO: mfm        - low-level MFM encoded bit stream
	// TODO: pdi        - Upland's PlanetPress
	// TODO: pri        - PCE Raw Image
	// TODO: psi        - PCE Sector Image
	// TODO: scp        - SuperCard Pro low-level raw magnetic flux transitions
	// TODO: td0        - Teledisk

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
		if hfe.DetectImageFormat(filename) == hfe.ImageFormatUnknown {
			cobra.CheckErr(fmt.Errorf("unknown image format: %s", filename))
		}

		// Prompt user to insert diskette
		fmt.Print("Insert SOURCE diskette in drive\nand press Enter when ready...")
		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')
		fmt.Printf("\n")

		// Read floppy disk using adapter interface
		disk, err := floppyAdapter.Read(82)
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
