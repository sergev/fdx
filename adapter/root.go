package adapter

import (
	"fmt"
	"strconv"

	"github.com/sergev/floppy/config"
	"github.com/spf13/cobra"
	"go.bug.st/serial/enumerator"
)

var floppyAdapter FloppyAdapter

var rootCmd = &cobra.Command{
	Use:   "floppy",
	Short: "A CLI program which works with floppy disks via USB adapter",
	Long:  "The floppy tool is a CLI program which works with floppy disks via USB adapter.",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
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
