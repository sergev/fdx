package adapter

import (
	"bufio"
	"fmt"
	"os"

	"github.com/sergev/floppy/config"
	"github.com/sergev/floppy/hfe"
	"github.com/spf13/cobra"
)

var writeCmd = &cobra.Command{
	Use:   "write SRC.EXT",
	Short: "Write image to the floppy disk",
	Long: `Write image from SRC.EXT to the floppy disk.
Format of floppy image is defined by extension.
` + supportedImageFormatsText,
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

		// Match image versus drive.
		if int(disk.Header.BitRate) > config.MaxKBps {
			cobra.CheckErr(fmt.Errorf("Image with bit rate %d kbps is incompatible with drive %s",
				disk.Header.BitRate, config.DriveName))
		}
		if int(disk.Header.NumberOfSide) > config.Heads {
			cobra.CheckErr(fmt.Errorf("Image with %d sides is incompatible with drive %s",
				disk.Header.NumberOfSide, config.DriveName))
		}

		// Get number of tracks to write (but no more than extra 2 tracks)
		numCylinders := int(disk.Header.NumberOfTrack)
		if numCylinders > config.Cyls+2 {
			cobra.CheckErr(fmt.Errorf("Image with %d cylinders is incompatible with drive %s",
				numCylinders, config.DriveName))
		}
		if hfe.DetectImageFormat(filename) != hfe.ImageFormatHFE {
			if numCylinders >= 80 {
				// Ignore extra cylinders
				numCylinders = 80
			} else if numCylinders > 40 {
				numCylinders = 40
			}
		}
		disk.InitVerifyOptions()
		fmt.Printf("Writing %d tracks, %d side(s)\n", numCylinders, disk.Header.NumberOfSide)
		fmt.Printf("Bit Rate: %d kbps\n", disk.Header.BitRate)
		fmt.Printf("Rotation Speed: %d RPM\n", disk.Header.FloppyRPM)
		fmt.Printf("\n")

		// Prompt user to insert diskette
		fmt.Print("Insert TARGET diskette in drive\nand press Enter when ready...")
		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')
		fmt.Printf("\n")

		// Write floppy disk using adapter interface
		err = floppyAdapter.Write(disk, numCylinders)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write floppy disk: %w", err))
		}
		fmt.Printf("\n")
		fmt.Printf("Image from file '%s' written to diskette.\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(writeCmd)
}
