package cmd

import (
	"fmt"
	"strconv"

	"floppy/adapter"
	"floppy/greaseweazle"
	"floppy/kryoflux"
	"floppy/supercardpro"

	"github.com/spf13/cobra"
	"go.bug.st/serial/enumerator"
)

var floppyAdapter adapter.FloppyAdapter

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
			cobra.CheckErr(fmt.Errorf("failed to find USB adapter: %w", err))
		}
	},
}

// findAdapter attempts to find and initialize either a Greaseweazle, SuperCard Pro, or KryoFlux adapter
// It tries Greaseweazle first, then SuperCard Pro, then KryoFlux
// Returns the initialized adapter or an error if none is found
func findAdapter() (adapter.FloppyAdapter, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("failed to list serial ports: %w", err)
	}

	// Try Greaseweazle first
	for _, port := range ports {
		portVID, err := strconv.ParseUint(port.VID, 16, 16)
		if err != nil {
			continue
		}
		portPID, err := strconv.ParseUint(port.PID, 16, 16)
		if err != nil {
			continue
		}

		if uint16(portVID) == greaseweazle.VendorID && uint16(portPID) == greaseweazle.ProductID {
			adapter, err := greaseweazle.NewClient(port)
			if err != nil {
				continue // Try next port
			}
			return adapter, nil
		}
	}

	// Try SuperCard Pro
	for _, port := range ports {
		portVID, err := strconv.ParseUint(port.VID, 16, 16)
		if err != nil {
			continue
		}
		portPID, err := strconv.ParseUint(port.PID, 16, 16)
		if err != nil {
			continue
		}

		if uint16(portVID) == supercardpro.VendorID && uint16(portPID) == supercardpro.ProductID {
			adapter, err := supercardpro.NewClient(port)
			if err != nil {
				continue // Try next port
			}
			return adapter, nil
		}
	}

	// Try KryoFlux
	for _, port := range ports {
		portVID, err := strconv.ParseUint(port.VID, 16, 16)
		if err != nil {
			continue
		}
		portPID, err := strconv.ParseUint(port.PID, 16, 16)
		if err != nil {
			continue
		}

		if uint16(portVID) == kryoflux.VendorID && uint16(portPID) == kryoflux.ProductID {
			adapter, err := kryoflux.NewClient(port)
			if err != nil {
				continue // Try next port
			}
			return adapter, nil
		}
	}

	return nil, fmt.Errorf("no supported USB adapter found (Greaseweazle: VID=0x%04X PID=0x%04X, SuperCard Pro: VID=0x%04X PID=0x%04X, KryoFlux: VID=0x%04X PID=0x%04X)",
		greaseweazle.VendorID, greaseweazle.ProductID, supercardpro.VendorID, supercardpro.ProductID, kryoflux.VendorID, kryoflux.ProductID)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}
