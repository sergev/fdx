package adapter

import (
	"fmt"
	"strconv"

	"github.com/sergev/floppy/config"
	"github.com/spf13/cobra"
	"go.bug.st/serial/enumerator"
)

var floppyAdapter FloppyAdapter

const supportedImageFormatsText = `Supported image formats:
  *.adf          - Amiga Disk File
  *.bkd          - BK-0010/0011M Disk image
  *.hfe          - HxC Floppy Emulator
  *.img or *.ima - raw binary contents of the entire disk`
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

var rootCmd = &cobra.Command{
	Use:   "floppy",
	Short: "Tool for reading and writing diskettes via USB floppy adapters",
	Long: `Command-line tool for reading, writing and formatting diskettes via USB floppy adapters.
` + supportedImageFormatsText,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		switch cmd.Name() {
		case "status", "read", "write", "format", "erase":
			// These commands require the floppy hardware
			break
		default:
			// Other commands don't need the floppy device
			return
		}

		var err error
		floppyAdapter, err = findAdapter()
		if err != nil {
			cobra.CheckErr(fmt.Errorf("%w", err))
		}

		// Initialize configuration
		err = config.Initialize()
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to initialize config: %w", err))
		}
	},
}

// findAdapter attempts to find and initialize a registered adapter
// Returns the initialized adapter or an error if none is found
func findAdapter() (FloppyAdapter, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("failed to list serial ports: %w", err)
	}

	// Try registered serial port adapters
	for _, port := range ports {
		portVID, err := strconv.ParseUint(port.VID, 16, 16)
		if err != nil {
			continue
		}
		portPID, err := strconv.ParseUint(port.PID, 16, 16)
		if err != nil {
			continue
		}

		// Check each registered adapter
		for _, info := range registeredAdapters {
			if info.VendorID == 0 && info.ProductID == 0 {
				continue // Skip USB-only adapters here
			}
			if uint16(portVID) == info.VendorID && uint16(portPID) == info.ProductID {
				adapter, err := info.Factory(port)
				if err != nil {
					continue // Try next port
				}
				return adapter, nil
			}
		}
	}

	// Try registered USB-only adapters (like KryoFlux)
	for _, info := range registeredAdapters {
		if info.VendorID == 0 && info.ProductID == 0 {
			adapter, err := info.Factory(nil)
			if err == nil && adapter != nil {
				return adapter, nil
			}
		}
	}

	return nil, fmt.Errorf("no supported USB floppy adapter found")
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}
