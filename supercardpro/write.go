package supercardpro

import (
	"encoding/binary"
	"fmt"

	"floppy/hfe"
)

// Convert MFM bitcells to flux transition times.
// MFM bitcells are bits where transitions occur when bit values change.
// Return transition times in nanoseconds relative to track start.
func mfmToFluxTransitions(mfmBits []byte, bitRateKhz uint16) ([]uint64, error) {
	if len(mfmBits) == 0 {
		return nil, fmt.Errorf("empty MFM data")
	}

	// Calculate bitcell period in nanoseconds
	// bitRateKhz is in kbps, so bitRate_bps = bitRateKhz * 1000
	bitRateBps := float64(bitRateKhz) * 1000.0 * 2
	bitcellPeriodNs := uint64(1e9 / bitRateBps)

	var transitions []uint64
	currentTime := uint64(0)

	// Process each bit in the MFM bitcell stream
	bitCount := len(mfmBits) * 8
	for i := 0; i < bitCount; i++ {
		// Extract bit at position i (MSB-first)
		byteIdx := i / 8
		bitIdx := 7 - (i % 8) // MSB-first
		currentBit := (mfmBits[byteIdx] & (1 << bitIdx)) != 0

		// Advance time by one bitcell period before checking for transition
		currentTime += bitcellPeriodNs

		// Add transition time when bit changes
		if currentBit {
			transitions = append(transitions, currentTime)
		}
	}
	return transitions, nil
}

// Extend transitions array to cover a full rotation period.
// Appends transitions at 2-bitcell intervals until the rotation duration is reached.
func coverFullRotation(transitions []uint64, bitRateKhz uint16, floppyRPM uint16) []uint64 {
	// Calculate rotation duration in nanoseconds
	// Rotation duration = 60 seconds / RPM = 60e9 nanoseconds / RPM
	rotationDurationNs := uint32(60e9 / float64(floppyRPM))

	// Calculate bitcell period in nanoseconds
	// bitRateKhz is in kbps, so bitRate_bps = bitRateKhz * 1000
	bitRateBps := float64(bitRateKhz) * 1000.0 * 2
	bitcellPeriodNs := uint64(1e9 / bitRateBps)

	// Calculate 2-bitcell period
	twoBitcellPeriodNs := 2 * bitcellPeriodNs

	// Get last transition time (or 0 if empty)
	lastTime := uint64(0)
	if len(transitions) > 0 {
		lastTime = transitions[len(transitions)-1]
	}

	// Append transitions at 2-bitcell intervals until we reach the rotation duration
	currentTime := lastTime
	for currentTime+twoBitcellPeriodNs <= uint64(rotationDurationNs) {
		currentTime += twoBitcellPeriodNs
		transitions = append(transitions, currentTime)
	}

	return transitions
}

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

// Write writes data from the specified filename to the floppy disk
func (c *Client) Write(filename string) error {
	// Select drive 0 and turn on motor
	err := c.selectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	defer c.deselectDrive(0) // Deselect drive and turn off motor when done

	// Read HFE file
	disk, err := hfe.Read(filename)
	if err != nil {
		return fmt.Errorf("failed to read HFE file: %w", err)
	}

	// Validate HFE file
	if disk.Header.NumberOfTrack == 0 || disk.Header.NumberOfSide == 0 {
		return fmt.Errorf("invalid HFE file: zero tracks or sides")
	}

	if disk.Header.FloppyRPM == 0 {
		return fmt.Errorf("invalid HFE file: bad floppy rotation speed")
	}

	if disk.Header.TrackEncoding != hfe.ENC_ISOIBM_MFM {
		return fmt.Errorf("unsupported track encoding: %d (only ISOIBM_MFM is supported)", disk.Header.TrackEncoding)
	}

	// Get number of tracks to write (use minimum of HFE tracks and standard 82)
	numberOfTracks := int(disk.Header.NumberOfTrack)
	if numberOfTracks > 82 {
		numberOfTracks = 82
	}

	fmt.Printf("Writing HFE file to floppy disk\n")
	fmt.Printf("Tracks: %d, Sides: %d, Bit Rate: %d kbps, RPM: %d\n",
		numberOfTracks, disk.Header.NumberOfSide, disk.Header.BitRate, disk.Header.FloppyRPM)

	// Iterate through cylinders and heads
	for cyl := 0; cyl < numberOfTracks; cyl++ {
		for head := 0; head < int(disk.Header.NumberOfSide); head++ {
			// Print progress message
			if cyl != 0 || head != 0 {
				fmt.Printf("\rWriting track %d, side %d...", cyl, head)
			} else {
				fmt.Printf("Writing track %d, side %d...", cyl, head)
			}

			// Calculate track number (track = cyl * 2 + head)
			track := uint(cyl*2 + head)

			// Seek to track
			err = c.seekTrack(track)
			if err != nil {
				return fmt.Errorf("failed to seek to track %d: %w", track, err)
			}

			// Get MFM bitcells from HFE track data
			var mfmBits []byte
			if head == 0 {
				mfmBits = disk.Tracks[cyl].Side0
			} else {
				mfmBits = disk.Tracks[cyl].Side1
			}

			// Convert MFM bitcells to flux transitions
			transitions, err := mfmToFluxTransitions(mfmBits, disk.Header.BitRate)
			if err != nil {
				return fmt.Errorf("failed to convert MFM to flux transitions for cylinder %d, head %d: %w", cyl, head, err)
			}

			// Extend transitions to cover full rotation
			transitions = coverFullRotation(transitions, disk.Header.BitRate, disk.Header.FloppyRPM)

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
	fmt.Printf(" Done\n")

	return nil
}
