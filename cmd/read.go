package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"floppy/greaseweazle"
)

var readCmd = &cobra.Command{
	Use:   "read [FILE]",
	Short: "Read data from the floppy disk",
	Long:  "Read data from the floppy disk. Optionally specify a FILE to read.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if floppyAdapter == nil {
			cobra.CheckErr(fmt.Errorf("adapter not available"))
		}

		// Type assert to Greaseweazle client for now
		// TODO: Extend FloppyAdapter interface to include Seek, SetHead, ReadFlux, etc.
		gw, ok := floppyAdapter.(*greaseweazle.Client)
		if !ok {
			cobra.CheckErr(fmt.Errorf("read command currently only supports Greaseweazle adapter"))
		}

		// Select drive 0 and turn on motor
		err := gw.SelectDrive(0)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to select drive: %w", err))
		}
		err = gw.SetMotor(0, true)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to turn on motor: %w", err))
		}

		// Determine output filename
		filename := "floppy_raw.bin"
		if len(args) > 0 {
			filename = args[0]
		}

		// Open output file
		file, err := os.Create(filename)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to create output file: %w", err))
		}
		defer file.Close()

		// Iterate through 80 cylinders (0-79) and 2 heads (0-1)
		for cyl := 0; cyl < 80; cyl++ {
			for head := 0; head < 2; head++ {
				// Print progress message
				fmt.Printf("Reading cylinder %d, head %d...\n", cyl, head)

				// Seek to cylinder
				err = gw.Seek(byte(cyl))
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to seek to cylinder %d: %w", cyl, err))
				}

				// Set head
				err = gw.SetHead(byte(head))
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to set head %d: %w", head, err))
				}

				// Read flux data (0 ticks = no limit, 2 index pulses = 2 revolutions)
				data, err := gw.ReadFlux(0, 2)
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to read flux data from cylinder %d, head %d: %w", cyl, head, err))
				}

				// Check flux status
				err = gw.GetFluxStatus()
				if err != nil {
					cobra.CheckErr(fmt.Errorf("flux status error after reading cylinder %d, head %d: %w", cyl, head, err))
				}

				// Write raw flux data to file
				_, err = file.Write(data)
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to write data to file: %w", err))
				}
			}
		}

		fmt.Printf("Successfully read floppy disk to %s\n", filename)
	},
}

func init() {
	rootCmd.AddCommand(readCmd)
}

