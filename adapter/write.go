package adapter

import (
	"fmt"

	"github.com/sergev/floppy/hfe"
	"github.com/spf13/cobra"
)

var writeCmd = &cobra.Command{
	Use:   "write SRC.EXT",
	Short: "Write image to the floppy disk",
	Long: `Write image from SRC.EXT to the floppy disk.
Format of floppy image is defined by extension.
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

	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Determine input filename
		filename := args[0]

		// Read file
		disk, err := hfe.Read(filename)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read file: %w", err))
		}

		// Get number of tracks to write (but no more than standard 82 tracks)
		numberOfTracks := int(disk.Header.NumberOfTrack)
		if numberOfTracks > 82 {
			numberOfTracks = 82
		}

		fmt.Printf("Writing file to floppy disk\n")
		fmt.Printf("Tracks: %d, Sides: %d, Bit Rate: %d kbps, RPM: %d\n",
			numberOfTracks, disk.Header.NumberOfSide, disk.Header.BitRate, disk.Header.FloppyRPM)

		// Write floppy disk using adapter interface
		err = floppyAdapter.Write(disk, numberOfTracks)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write floppy disk: %w", err))
		}

		fmt.Printf("Successfully imaged floppy disk from %s\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(writeCmd)
}
