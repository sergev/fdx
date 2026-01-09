package supercardpro

import (
	"encoding/binary"
	"fmt"
)

// Generate minimal flux data for one revolution
// Assume 300 RPM (250 kbps) drive speed
// Return flux data as uint16 samples (big-endian) suitable for erase operation
func (c *Client) generateEraseFlux() []byte {
	// For 300 RPM: 1 revolution = 0.2 seconds = 200,000,000 nanoseconds
	// IndexTime in 25ns units = 200,000,000 / 25 = 8,000,000
	const indexTime = uint32(8000000) // 300 RPM in 25ns units

	// Calculate approximate number of samples needed for one revolution
	// Use a reasonable interval size (e.g., 2000 units = 50 microseconds)
	// This gives us enough samples to cover one revolution
	//	intervalSize := uint16(2000) // 2000 * 25ns = 50 microseconds
	intervalSize := uint16(40) // 40 * 25ns = 1 microseconds
	nrSamples := indexTime / uint32(intervalSize)

	// Generate flux data: simple pattern of intervals
	// For erase, we just need enough data - the exact pattern doesn't matter
	fluxData := make([]byte, int(nrSamples)*2)
	for i := uint32(0); i < nrSamples; i++ {
		// Write interval as big-endian uint16
		binary.BigEndian.PutUint16(fluxData[i*2:(i+1)*2], intervalSize)
	}

	return fluxData
}

// Erase erases the floppy disk
func (c *Client) Erase() error {
	// Select drive 0 and turn on motor
	err := c.selectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	defer c.deselectDrive(0)

	// Generate minimal flux data for one revolution (assumes 300 RPM / 250 kbps)
	flux := c.generateEraseFlux()
	nrSamples := uint32(len(flux) / 2)

	// Load flux data into RAM once (same data used for all tracks)
	err = c.loadRAM(flux)
	if err != nil {
		return fmt.Errorf("failed to load flux data: %w", err)
	}

	// Erase all tracks (typically 0-163 for 3.5" floppy: 82 tracks × 2 sides)
	// Standard 3.5" DD floppy has 80 tracks, but we'll go up to 82 to be safe
	maxTrack := uint(82 * 2) // 82 cylinders × 2 sides

	for track := uint(0); track < maxTrack; track++ {
		cyl := track >> 1
		side := track & 1

		// Print progress
		fmt.Printf("\rErasing cylinder %d, side %d...", cyl, side)

		// Seek to track
		err = c.seekTrack(track)
		if err != nil {
			return fmt.Errorf("failed to seek to track %d: %w", track, err)
		}

		// Write with wipe flag to erase the track (1 revolution for faster erase)
		// Note: Flux data is already loaded in RAM from the initial loadRAM call
		err = c.writeFlux(nrSamples, 1)
		if err != nil {
			return fmt.Errorf("failed to erase track %d: %w", track, err)
		}
	}
	fmt.Printf(" Done\n")

	return nil
}
