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
Supported image formats:
    *.adf          - Amiga Disk File
    *.hfe          - HxC Floppy Emulator
    *.img or *.ima - raw binary contents of the entire disk`,
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

	Args: cobra.ExactArgs(2),
	// Override PersistentPreRun to skip USB adapter initialization
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Do nothing - convert command doesn't need USB adapter
	},
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
