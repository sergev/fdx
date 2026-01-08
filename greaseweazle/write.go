package greaseweazle

import (
	"fmt"
	"io"

	"floppy/hfe"
)

const (
	// Enable for debug
	DebugFlag = false
)

// Encode a 28-bit value into N28 format (4 bytes).
// N28 encoding packs 28 bits across 4 bytes, with bit 0 of each byte set to 1.
// According to Greaseweazle protocol: b0 = 1 | (N << 1), b1 = 1 | (N >> 6), etc.
func encodeN28(value uint32) []byte {
	result := make([]byte, 4)
	result[0] = byte(1 | ((value & 0x7F) << 1))
	result[1] = byte(1 | (((value >> 7) & 0x7F) << 1))
	result[2] = byte(1 | (((value >> 14) & 0x7F) << 1))
	result[3] = byte(1 | (((value >> 21) & 0x7F) << 1))
	return result
}

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

// Encode flux transition times into Greaseweazle flux stream format.
// Transitions are relative times in nanoseconds, converted to ticks based on sample frequency.
func encodeFluxStream(transitions []uint64, sampleFreqHz uint32) []byte {
	var result []byte
	tickPeriodNs := 1e9 / float64(sampleFreqHz) // 13.889
	lastTime := uint64(0)

	// Encode each transition as an interval
	for _, transitionTime := range transitions {
		// Calculate interval in nanoseconds
		intervalNs := transitionTime - lastTime

		// Convert to ticks
		intervalTicks := uint32(float64(intervalNs) / tickPeriodNs)

		// Encode interval
		// Minimum interval is 1 tick
		if intervalTicks == 0 {
			intervalTicks = 1
		}
		if DebugFlag {
			fmt.Printf(" %d", intervalTicks)
		}

		if intervalTicks < 250 {
			// Direct encoding: single byte (1-249)
			result = append(result, byte(intervalTicks))
		} else if intervalTicks < 1525 {
			// Extended encoding: base byte + offset byte
			// Formula: value = 250 + (base - 250) * 255 + offset - 1
			// So: value + 1 = 250 + (base - 250) * 255 + offset
			// Ranges:
			//   0xFA (250): 250-504   (offset = value + 1 - 250)
			//   0xFB (251): 505-759   (offset = value + 1 - 505)
			//   0xFC (252): 760-1014  (offset = value + 1 - 760)
			//   0xFD (253): 1015-1269 (offset = value + 1 - 1015)
			//   0xFE (254): 1270-1524 (offset = value + 1 - 1270)
			base := byte(0xFA)
			offset := uint32(intervalTicks + 1 - 250)
			for offset >= 255 {
				base++
				offset -= 255
			}
			// Offset byte of 0 means value = base_range_start - 1, so we need at least 1
			// But actually offset can be 0 for the minimum value in each range
			result = append(result, base, byte(offset))
		} else {
			// Use FLUXOP_SPACE with N28 encoding for very large intervals
			result = append(result, 0xFF, FLUXOP_SPACE)
			n28 := encodeN28(intervalTicks)
			result = append(result, n28...)
		}

		lastTime = transitionTime
	}
	if DebugFlag {
		fmt.Printf("--- %d transitions -> %d fluxes\n", len(transitions), len(result))
	}

	// Terminate stream with null byte
	result = append(result, 0x00)

	return result
}

// Send CMD_WRITE_FLUX command and flux stream data to the device.
func (c *Client) WriteFlux(fluxData []byte) error {
	// Build CMD_WRITE_FLUX command
	// Based on firmware source, the command format is:
	// [CMD_WRITE_FLUX, len, cue_at_index, terminate_at_index, ...hard_sector_ticks (optional)]
	// The len byte includes the command byte and len byte itself.
	// Minimum length is 4 (cmd + len + cue_at_index + terminate_at_index)
	// Maximum length is 8 (includes optional hard_sector_ticks as uint32)
	// Firmware checks: len >= (2 + offsetof(gw_write_flux, hard_sector_ticks))
	// Since hard_sector_ticks is at offset 2 in packed struct, minimum len = 4

	// Always use minimum format with both cue_at_index and terminate_at_index
	// len = 4 means: command(1) + len(1) + cue_at_index(1) + terminate_at_index(1) = 4 bytes
	cmd := []byte{CMD_WRITE_FLUX, 4, 1, 1}

	// Send command
	err := c.doCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to send WRITE_FLUX command: %w", err)
	}

	// Send flux stream data
	_, err = c.port.Write(fluxData)
	if err != nil {
		return fmt.Errorf("failed to write flux data: %w", err)
	}

	// Read synchronization byte (device sends this when write completes)
	syncByte := make([]byte, 1)
	_, err = io.ReadFull(c.port, syncByte)
	if err != nil {
		return fmt.Errorf("failed to read write synchronization byte: %w", err)
	}

	if syncByte[0] != 0 {
		return fmt.Errorf("write operation failed with status byte: 0x%02x", syncByte[0])
	}

	// Check flux status
	err = c.GetFluxStatus()
	if err != nil {
		return fmt.Errorf("write flux status check failed: %w", err)
	}

	return nil
}

// Write an HFE file to the floppy disk track by track.
func (c *Client) Write(filename string) error {
	// Select drive 0 and turn on motor
	err := c.SelectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	err = c.SetMotor(0, true)
	if err != nil {
		return fmt.Errorf("failed to turn on motor: %w", err)
	}
	defer c.SetMotor(0, false) // Turn off motor when done

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

			// Seek to cylinder
			err = c.Seek(byte(cyl))
			if err != nil {
				return fmt.Errorf("failed to seek to cylinder %d: %w", cyl, err)
			}

			// Set head
			err = c.SetHead(byte(head))
			if err != nil {
				return fmt.Errorf("failed to set head %d: %w", head, err)
			}

			// Get MFM bitcells from HFE track data
			var mfmBits []byte
			if head == 0 {
				mfmBits = disk.Tracks[cyl].Side0
			} else {
				mfmBits = disk.Tracks[cyl].Side1
			}

			if len(mfmBits) == 0 {
				// Empty track - skip or write empty flux stream
				continue
			}

			// Convert MFM bitcells to flux transitions
			transitions, err := mfmToFluxTransitions(mfmBits, disk.Header.BitRate)
			if err != nil {
				return fmt.Errorf("failed to convert MFM to flux transitions for cylinder %d, head %d: %w", cyl, head, err)
			}

			// Extend transitions to cover full rotation
			transitions = coverFullRotation(transitions, disk.Header.BitRate, disk.Header.FloppyRPM)

			// Encode flux transitions to flux stream format
			fluxData := encodeFluxStream(transitions, c.firmwareInfo.SampleFreqHz)

			// Write flux stream to floppy
			err = c.WriteFlux(fluxData)
			if err != nil {
				// Check for write protection error
				errMsg := err.Error()
				if errMsg == "Greaseweazle error: write protected" || errMsg == "failed to send WRITE_FLUX command: Greaseweazle error: write protected" {
					return fmt.Errorf("write protected: cannot write to disk")
				}
				return fmt.Errorf("failed to write flux data for cylinder %d, head %d: %w", cyl, head, err)
			}
		}
	}
	fmt.Printf(" Done\n")

	return nil
}
