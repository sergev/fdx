package greaseweazle

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/sergev/floppy/hfe"
	"github.com/sergev/floppy/mfm"
)

// readN28 decodes a 28-bit value from Greaseweazle N28 encoding
// Returns the decoded value and the number of bytes consumed
func readN28(data []byte, offset int) (uint32, int, error) {
	if offset+4 > len(data) {
		return 0, 0, fmt.Errorf("insufficient data for N28 encoding at offset %d", offset)
	}

	b0 := data[offset]
	b1 := data[offset+1]
	b2 := data[offset+2]
	b3 := data[offset+3]

	value := ((uint32(b0) & 0xfe) >> 1) |
		((uint32(b1) & 0xfe) << 6) |
		((uint32(b2) & 0xfe) << 13) |
		((uint32(b3) & 0xfe) << 20)

	return value, 4, nil
}

// ReadFlux reads raw flux data from the current track
// ticks: maximum ticks to read (0 = no limit)
// maxIndex: maximum index pulses to read (0 = no limit, typically 2 for 2 revolutions)
func (c *Client) ReadFlux(ticks uint32, maxIndex uint16) ([]byte, error) {
	// Build CMD_READ_FLUX command: [CMD_READ_FLUX, 8, ticks (le32), maxIndex (le16)]
	cmd := make([]byte, 8)
	cmd[0] = CMD_READ_FLUX
	cmd[1] = 8
	binary.LittleEndian.PutUint32(cmd[2:6], ticks)
	binary.LittleEndian.PutUint16(cmd[6:8], maxIndex)

	err := c.doCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to send READ_FLUX command: %w", err)
	}

	// Read flux data until we encounter a 0 byte (end of stream marker)
	var data []byte
	buf := make([]byte, 1)
	for {
		_, err := io.ReadFull(c.port, buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read flux data: %w", err)
		}
		if buf[0] == 0 {
			break
		}
		data = append(data, buf[0])
	}

	return data, nil
}

// Extract index pulse timings from flux data.
// Calculate RPM and bit rate.
// Return the calculated RPM: 300 or 360.
// Return the calculated bit rate: 250, 500 or 1000 bits/msec.
func (c *Client) calculateRPMAndBitRate(fluxData []byte) (uint16, uint16) {
	var indexPulses []uint64 // Index pulse times in nanoseconds

	tickPeriodNs := 1e9 / float64(c.firmwareInfo.SampleFreqHz) // Nanoseconds per tick
	ticksAccumulated := uint64(0)
	countTransitions := uint64(0)

	i := 0
	for i < len(fluxData) {
		b := fluxData[i]

		if b == 0xFF {
			// Special opcode
			if i+1 >= len(fluxData) {
				break
			}

			opcode := fluxData[i+1]
			i += 2

			switch opcode {
			case FLUXOP_INDEX:
				// Index pulse marker
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					break
				}
				i += consumed
				indexTime := ticksAccumulated + uint64(n28)
				indexPulses = append(indexPulses, uint64(float64(indexTime)*tickPeriodNs))

			case FLUXOP_SPACE:
				// Time gap with no transitions
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					break
				}
				i += consumed
				if DebugFlag {
					fmt.Printf(" %d", n28)
				}
				ticksAccumulated += uint64(n28)

			default:
				// Unknown opcode, skip
			}
		} else if b < 250 {
			// Direct interval: 1-249 ticks
			if DebugFlag {
				fmt.Printf(" %d", b)
			}
			ticksAccumulated += uint64(b)
			if len(indexPulses) == 1 {
				// Ignore all before the first index pulse, and
				// after the second index pulse
				countTransitions++
			}
			i++
		} else {
			// Extended interval: 250-254
			if i+1 >= len(fluxData) {
				break
			}
			delta := 250 + uint64(b-250)*255 + uint64(fluxData[i+1]) - 1
			if DebugFlag {
				fmt.Printf(" %d", delta)
			}
			ticksAccumulated += delta
			if len(indexPulses) == 1 {
				// Ignore all before the first index pulse, and
				// after the second index pulse
				countTransitions++
			}
			i += 2
		}
	}

	// Need at least 2 index pulses to calculate rotation period
	if len(indexPulses) < 2 {
		return 300, 250 // Default RPM and bit rate
	}

	//
	// Calculate RPM: 60 seconds per minute / period in seconds
	//
	trackDurationNs := indexPulses[1] - indexPulses[0]
	//fmt.Printf("--- trackDurationNs = %d\n", trackDurationNs)

	rpm := 60e9 / trackDurationNs
	//fmt.Printf("--- rpm = %d\n", rpm)

	// Round to either 300 or 360 RPM (standard floppy drive speeds)
	// Use 330 RPM as the threshold (midpoint between 300 and 360)
	if rpm < 330 {
		rpm = 300
	} else {
		rpm = 360
	}

	//
	// Calculate bit rate
	//
	bitsPerMsec := countTransitions * 1e6 / trackDurationNs
	//fmt.Printf("--- bitsPerMsec = %d\n", bitsPerMsec)

	// Round to standard floppy drive bitrates: 250, 500, or 1000 kbps
	// Use thresholds: < 375 -> 250, < 750 -> 500, >= 750 -> 1000
	if bitsPerMsec < 375 {
		bitsPerMsec = 250
	} else if bitsPerMsec < 750 {
		bitsPerMsec = 500
	} else {
		bitsPerMsec = 1000
	}

	return uint16(rpm), uint16(bitsPerMsec)
}

// decodeFluxToMFM recovers raw MFM bitcells from Greaseweazle flux data using PLL,
// and returns MFM bitcells as bytes (bitcells packed MSB-first, not decoded data bits)
func (c *Client) decodeFluxToMFM(fluxData []byte, bitRateKhz uint16) ([]byte, error) {
	if len(fluxData) == 0 {
		return nil, fmt.Errorf("empty flux data")
	}

	// Step 1: Decode Greaseweazle flux stream to get transition times
	var transitions []uint64 // Times in nanoseconds
	var indexPulses []uint64 // Index pulse times

	tickPeriodNs := 1e9 / float64(c.firmwareInfo.SampleFreqHz) // Nanoseconds per tick = 13.89
	ticksAccumulated := uint64(0)

	i := 0
	for i < len(fluxData) {
		b := fluxData[i]

		if b == 0xFF {
			// Special opcode
			if i+1 >= len(fluxData) {
				return nil, fmt.Errorf("incomplete opcode at offset %d", i)
			}

			opcode := fluxData[i+1]
			i += 2

			switch opcode {
			case FLUXOP_INDEX:
				// Index pulse marker
				_, consumed, err := readN28(fluxData, i)
				if err != nil {
					return nil, fmt.Errorf("failed to read INDEX N28: %w", err)
				}
				i += consumed
				indexTime := ticksAccumulated //+ uint64(n28)
				indexPulses = append(indexPulses, uint64(float64(indexTime)*tickPeriodNs))
				// Index pulse doesn't advance the cursor

			case FLUXOP_SPACE:
				// Time gap with no transitions
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					return nil, fmt.Errorf("failed to read SPACE N28: %w", err)
				}
				i += consumed
				ticksAccumulated += uint64(n28)

			default:
				return nil, fmt.Errorf("unknown opcode 0x%02x at offset %d", opcode, i-1)
			}
		} else if b < 250 {
			// Direct interval: 1-249 ticks
			ticksAccumulated += uint64(b)
			if len(indexPulses) == 1 {
				// Ignore all before the first index pulse, and
				// after the second index pulse
				transitionTime := uint64(float64(ticksAccumulated)*tickPeriodNs) - indexPulses[0]
				transitions = append(transitions, transitionTime)
				//fmt.Printf(" %d", transitionTime)
			}
			i++
		} else {
			// Extended interval: 250-254
			if i+1 >= len(fluxData) {
				return nil, fmt.Errorf("incomplete extended interval at offset %d", i)
			}
			delta := 250 + uint64(b-250)*255 + uint64(fluxData[i+1]) - 1
			ticksAccumulated += delta
			if len(indexPulses) == 1 {
				transitionTime := uint64(float64(ticksAccumulated)*tickPeriodNs) - indexPulses[0]
				transitions = append(transitions, transitionTime)
				//fmt.Printf(" %d", transitionTime)
			}
			i += 2
		}
	}

	if len(transitions) == 0 {
		return nil, fmt.Errorf("no flux transitions found")
	}

	// Step 2: Apply SCP-style PLL to recover clock and generate bitcell boundaries
	// Create and initialize PLL decoder with transitions
	decoder := mfm.NewDecoder(transitions, bitRateKhz)

	// Ignore first half-bit (as done in reference implementation)
	_ = decoder.NextBit()

	// Generate MFM bitcells using PLL algorithm
	var bitcells []bool
	for {
		first := decoder.NextBit()
		second := decoder.NextBit()

		bitcells = append(bitcells, first)
		bitcells = append(bitcells, second)

		if decoder.IsDone() {
			// No more transitions available
			break
		}
	}

	if len(bitcells) == 0 {
		return nil, fmt.Errorf("no bitcells generated")
	}

	// Step 4: Pack bitcells as bytes (MSB-first)
	// Each bitcell becomes one bit in the output
	var mfmBytes []byte
	currentByte := byte(0)
	bitCount := 0

	for _, bit := range bitcells {
		if bit {
			currentByte |= 1 << (7 - bitCount)
		}
		bitCount++

		// When we have 8 bits, save the byte and start a new one
		if bitCount == 8 {
			mfmBytes = append(mfmBytes, currentByte)
			currentByte = 0
			bitCount = 0
		}
	}

	// Add any remaining partial byte
	if bitCount > 0 {
		mfmBytes = append(mfmBytes, currentByte)
	}

	if len(mfmBytes) == 0 {
		return nil, fmt.Errorf("no MFM bytes generated")
	}

	return mfmBytes, nil
}

// Read reads the entire floppy disk and returns it as a disk object
func (c *Client) Read(numberOfTracks int) (*hfe.Disk, error) {
	// Select drive 0 and turn on motor
	err := c.SelectDrive(0)
	if err != nil {
		return nil, fmt.Errorf("failed to select drive: %w", err)
	}
	err = c.SetMotor(0, true)
	if err != nil {
		return nil, fmt.Errorf("failed to turn on motor: %w", err)
	}
	defer c.SetMotor(0, false) // Turn off motor when done

	// Initialize disk structure
	disk := &hfe.Disk{
		Header: hfe.Header{
			NumberOfTrack:       uint8(numberOfTracks),
			NumberOfSide:        2,
			TrackEncoding:       hfe.ENC_ISOIBM_MFM,
			BitRate:             500,              // Will be calculated from flux data
			FloppyRPM:           300,              // Will be calculated from flux data
			FloppyInterfaceMode: hfe.IFM_IBMPC_DD, // Default to double density
			WriteProtected:      0xFF,             // Not write protected
			WriteAllowed:        0xFF,             // Write allowed
			SingleStep:          0xFF,             // Single step mode
			Track0S0AltEncoding: 0xFF,             // Use default encoding
			Track0S0Encoding:    hfe.ENC_ISOIBM_MFM,
			Track0S1AltEncoding: 0xFF, // Use default encoding
			Track0S1Encoding:    hfe.ENC_ISOIBM_MFM,
		},
		Tracks: make([]hfe.TrackData, numberOfTracks),
	}

	// Iterate through cylinders and heads
	for cyl := 0; cyl < numberOfTracks; cyl++ {
		for head := 0; head < 2; head++ {
			// Print progress message
			if cyl != 0 || head != 0 {
				fmt.Printf("\rReading track %d, side %d...", cyl, head)
			}

			// Seek to cylinder
			err = c.Seek(byte(cyl))
			if err != nil {
				return nil, fmt.Errorf("failed to seek to cylinder %d: %w", cyl, err)
			}

			// Set head
			err = c.SetHead(byte(head))
			if err != nil {
				return nil, fmt.Errorf("failed to set head %d: %w", head, err)
			}

			// Read flux data (0 ticks = no limit, 2 index pulses = 2 revolutions)
			fluxData, err := c.ReadFlux(0, 2)
			if err != nil {
				return nil, fmt.Errorf("failed to read flux data from cylinder %d, head %d: %w", cyl, head, err)
			}

			// Calculate RPM and BitRate from first track (cylinder 0, head 0)
			if cyl == 0 && head == 0 {
				calculatedRPM, calculatedBitRate := c.calculateRPMAndBitRate(fluxData)

				// Round to either 300 or 360 RPM (standard floppy drive speeds)
				// Use 330 RPM as the threshold (midpoint between 300 and 360)
				if calculatedRPM < 330 {
					calculatedRPM = 300
				} else {
					calculatedRPM = 360
				}

				// Round to standard floppy drive bitrates: 250, 500, or 1000 kbps
				// Use thresholds: < 375 -> 250, < 750 -> 500, >= 750 -> 1000
				if calculatedBitRate < 375 {
					calculatedBitRate = 250
				} else if calculatedBitRate < 750 {
					calculatedBitRate = 500
				} else {
					calculatedBitRate = 1000
				}
				fmt.Printf("Bit Rate: %d kbps\n", calculatedBitRate)
				fmt.Printf("Rotation Speed: %d RPM\n", calculatedRPM)

				disk.Header.FloppyRPM = calculatedRPM
				disk.Header.BitRate = calculatedBitRate
				if disk.Header.BitRate >= 750 {
					// Extended density
					disk.Header.FloppyInterfaceMode = hfe.IFM_IBMPC_ED
				} else if disk.Header.BitRate >= 375 {
					// High density
					disk.Header.FloppyInterfaceMode = hfe.IFM_IBMPC_HD
				}
			}

			// Decode flux data to MFM bitstream
			mfmBitstream, err := c.decodeFluxToMFM(fluxData, disk.Header.BitRate)
			if err != nil {
				return nil, fmt.Errorf("failed to decode flux data to MFM from cylinder %d, head %d: %w", cyl, head, err)
			}

			// Check flux status
			err = c.GetFluxStatus()
			if err != nil {
				return nil, fmt.Errorf("flux status error after reading cylinder %d, head %d: %w", cyl, head, err)
			}

			// Store MFM bitstream in appropriate side
			if head == 0 {
				disk.Tracks[cyl].Side0 = mfmBitstream
			} else {
				disk.Tracks[cyl].Side1 = mfmBitstream
			}
		}
	}
	fmt.Printf("\nRead complete.\n")

	return disk, nil
}
