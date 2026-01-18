package supercardpro

import (
	"encoding/binary"
	"fmt"

	"github.com/sergev/floppy/config"
	"github.com/sergev/floppy/hfe"
	"github.com/sergev/floppy/mfm"
)

// Encode flux transition times into SuperCard Pro flux format.
// Transitions are relative times in nanoseconds, converted to intervals in 25ns units.
func encodeFluxToSCP(transitions []uint64) []byte {
	var result []byte

	// Convert transitions to intervals
	lastTime := uint64(0)
	for _, transitionTime := range transitions {
		// Calculate interval in nanoseconds
		intervalNs := transitionTime - lastTime

		// Convert to 25ns units
		interval25ns := uint32(intervalNs / 25)

		// Handle overflow: if interval >= 0x10000, emit 0x0000 and subtract 0x10000
		for interval25ns >= 0x10000 {
			// Emit overflow marker (0x0000)
			result = append(result, 0x00, 0x00)
			interval25ns -= 0x10000
		}

		// Ensure minimum interval of 1 (0 would be interpreted as overflow)
		if interval25ns == 0 {
			interval25ns = 1
		}

		// Emit interval as big-endian uint16
		intervalBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(intervalBytes, uint16(interval25ns))
		result = append(result, intervalBytes...)

		lastTime = transitionTime
	}

	return result
}

// Write writes data from the disk object to the floppy disk
func (c *Client) Write(disk *hfe.Disk, numberOfTracks int) error {
	// Select drive 0 and turn on motor
	err := c.selectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	defer c.deselectDrive(0) // Deselect drive and turn off motor when done

	// Iterate through cylinders and heads
	for cyl := 0; cyl < numberOfTracks; cyl++ {
		for head := 0; head < int(disk.Header.NumberOfSide); head++ {
			// Print progress message
			if cyl != 0 || head != 0 {
				fmt.Printf("\rWriting track %d, side %d...", cyl, head)
			} else {
				fmt.Printf("Writing track %d, side %d...", cyl, head)
			}

			// Calculate track number
			track := uint(cyl*config.Heads + head)

			// Seek to track
			err = c.seekTrack(track)
			if err != nil {
				return fmt.Errorf("failed to seek to track %d: %w", track, err)
			}

			// Get MFM bitcells from track data
			var mfmBits []byte
			if head == 0 {
				mfmBits = disk.Tracks[cyl].Side0
			} else {
				mfmBits = disk.Tracks[cyl].Side1
			}

			// Convert MFM bitcells to flux transitions
			transitions, err := mfm.GenerateFluxTransitions(mfmBits, disk.Header.BitRate)
			if err != nil {
				return fmt.Errorf("failed to convert MFM to flux transitions for cylinder %d, head %d: %w", cyl, head, err)
			}

			// Extend transitions to cover full rotation
			transitions = mfm.CoverFullRotation(transitions, disk.Header.BitRate, disk.Header.FloppyRPM)

			// Encode flux transitions to SuperCard Pro format
			fluxData := encodeFluxToSCP(transitions)
			nrSamples := uint32(len(fluxData) / 2)

			// Load flux data into RAM
			err = c.loadRAM(fluxData)
			if err != nil {
				return fmt.Errorf("failed to load flux data for cylinder %d, head %d: %w", cyl, head, err)
			}

			// Write flux (2-5 revolutions for normal writes, use 2 as default)
			err = c.writeFlux(nrSamples, 2)
			if err != nil {
				return fmt.Errorf("failed to write flux data for cylinder %d, head %d: %w", cyl, head, err)
			}
		}
	}
	fmt.Printf("\nWrite complete.\n")

	return nil
}
