package adapter

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sergev/floppy/config"
	"github.com/sergev/floppy/hfe"
	"github.com/sergev/floppy/images"
	"github.com/spf13/cobra"
)

var formatCmd = &cobra.Command{
	Use:   "format",
	Short: "Format the floppy disk",
	Long:  "Format the floppy disk connected via USB adapter by selecting from pre-defined images.",
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Get list of image names from config
		imageNames := config.Images
		if len(imageNames) == 0 {
			cobra.CheckErr(fmt.Errorf("no images available for current drive"))
		}

		// Display menu with tags
		fmt.Printf("Available formats for floppy drive %s:\n", config.DriveName)
		for i, imgName := range imageNames {
			tag := indexToTag(i)
			fmt.Printf("  %s. %s\n", tag, imgName)
		}
		fmt.Print("\nSelect format (default 1): ")

		// Get user selection
		reader := bufio.NewReader(os.Stdin)
		selection, err := reader.ReadString('\n')
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read selection: %w", err))
		}
		selection = strings.TrimSpace(selection)

		// Default to first option if empty
		selectedIndex := 0
		if selection != "" {
			var err error
			selectedIndex, err = tagToIndex(selection, len(imageNames))
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid selection: %w", err))
			}
		}

		if selectedIndex < 0 || selectedIndex >= len(imageNames) {
			cobra.CheckErr(fmt.Errorf("invalid selection index: %d", selectedIndex))
		}

		selectedImageName := imageNames[selectedIndex]
		fmt.Printf("\nSelected: %s\n", selectedImageName)

		// Get filename from config
		filename, err := config.GetImageFilename(selectedImageName)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to get filename for image %q: %w", selectedImageName, err))
		}

		// Get image data from embedded images
		imageData, err := images.GetImage(filename)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to get embedded image %q: %w", filename, err))
		}

		// Write decompressed data to temporary file
		tmpFile, err := os.CreateTemp("", "floppy-format-*.img")
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to create temporary file: %w", err))
		}
		tmpFilename := tmpFile.Name()
		defer os.Remove(tmpFilename) // Clean up temp file

		// Write image data to temp file, preserving original filename extension for format detection
		// We need to use the original extension for hfe.DetectImageFormat to work correctly
		tmpFile.Close()
		tmpFileWithExt := tmpFilename
		if ext := getExtension(filename); ext != "" {
			tmpFileWithExt = tmpFilename + ext
			err = os.Rename(tmpFilename, tmpFileWithExt)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to rename temp file: %w", err))
			}
			defer os.Remove(tmpFileWithExt)
		}

		err = os.WriteFile(tmpFileWithExt, imageData, 0644)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write temporary file: %w", err))
		}

		// Read file using hfe.Read (same as write command)
		disk, err := hfe.Read(tmpFileWithExt)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to read image file: %w", err))
		}

		// Match image versus drive (same as write command)
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
			numCylinders = config.Cyls + 2
		}
		if hfe.DetectImageFormat(tmpFileWithExt) != hfe.ImageFormatHFE {
			if numCylinders >= 80 {
				// Ignore extra cylinders
				numCylinders = 80
			} else if numCylinders > 40 {
				numCylinders = 40
			}
		}
		fmt.Printf("Writing %d tracks, %d side(s)\n", numCylinders, disk.Header.NumberOfSide)
		fmt.Printf("Bit Rate: %d kbps\n", disk.Header.BitRate)
		fmt.Printf("Rotation Speed: %d RPM\n", disk.Header.FloppyRPM)
		fmt.Printf("\n")

		// Prompt user to insert diskette (same as write command)
		fmt.Print("Insert TARGET diskette in drive\nand press Enter when ready...")
		_, _ = reader.ReadString('\n')
		fmt.Printf("\n")

		// Write floppy disk using adapter interface (same as write command)
		err = floppyAdapter.Write(disk, numCylinders)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to write floppy disk: %w", err))
		}
		fmt.Printf("\n")
		fmt.Printf("Diskette formatted as '%s'.\n", selectedImageName)
	},
}

func init() {
	rootCmd.AddCommand(formatCmd)
}

// indexToTag converts an index (0-based) to a tag string (1-9, a-z)
func indexToTag(index int) string {
	if index < 9 {
		return fmt.Sprintf("%d", index+1)
	}
	return string(rune('a' + index - 9))
}

// tagToIndex converts a tag string (1-9, a-z) to an index (0-based)
func tagToIndex(tag string, maxIndex int) (int, error) {
	if len(tag) == 0 {
		return 0, nil
	}

	tag = strings.ToLower(tag)
	if len(tag) != 1 {
		return -1, fmt.Errorf("tag must be a single character")
	}

	c := tag[0]
	if c >= '1' && c <= '9' {
		index := int(c - '1')
		if index >= maxIndex {
			return -1, fmt.Errorf("tag %s is out of range", tag)
		}
		return index, nil
	}

	if c >= 'a' && c <= 'z' {
		index := 9 + int(c-'a')
		if index >= maxIndex {
			return -1, fmt.Errorf("tag %s is out of range", tag)
		}
		return index, nil
	}

	return -1, fmt.Errorf("invalid tag: %s (must be 1-9 or a-z)", tag)
}

// getExtension extracts the file extension from a filename
func getExtension(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
		if filename[i] == '/' || filename[i] == '\\' {
			break
		}
	}
	return ""
}
