package kryoflux

import (
	"encoding/binary"
	"fmt"
	"time"

	"floppy/hfe"
	"floppy/pll"
)

// Find EOF marker in the KryoFlux stream data according to the format specification
// Returns false if validation should continue, true if stream is complete or invalid
func (c *Client) findEndOfStream(data []byte) bool {

	// Process the data
	offset := 0
	for {
		if offset >= len(data) {
			// No EOF found - stream is incomplete
			return false
		}
		val := data[offset]

		switch {
		case val <= 0x07:
			// Value: 2-byte sequence
			offset += 2
		case val == 0x08:
			// Nop1: 1 byte
			offset += 1
		case val == 0x09:
			// Nop2: 2 bytes
			offset += 2
		case val == 0x0a:
			// Nop3: 3 bytes
			offset += 3
		case val == 0x0b:
			// Overflow16: 1-byte
			offset++
		case val == 0x0c:
			// Value16: 3-byte sequence
			offset += 3
		case val == 0x0d:
			// OOB marker: 4-byte header + data
			if offset+4 > len(data) {
				fmt.Printf("Lost OOB header!\n")
				return true
			}

			oobType := data[offset+1]
			if oobType == 0x0d {
				// End of stream marker
				return true
			}

			oobSize := int(data[offset+2]) | (int(data[offset+3]) << 8)
			if offset+4+oobSize > len(data) {
				fmt.Printf("Lost OOB data!\n")
				return true
			}

			// OOB markers are metadata - skip over them
			offset += oobSize + 4
		case val >= 0xe:
			// Sample: 1-byte
			offset++
		}
	}
}

// Capture a stream from the device and returns the raw stream data
func (c *Client) captureStream() ([]byte, error) {

	var streamData []byte

	// Start stream
	err := c.streamOn()
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}
	streamStarted := true
	defer func() {
		// Stop stream if we started it
		if streamStarted {
			c.controlIn(RequestStream, 0, true)
		}
	}()

	// Read buffer
	buf := make([]byte, ReadBufferSize)
	maxTotalTime := 30 * time.Second // Absolute maximum time for stream capture
	noDataTimeout := 5 * time.Second // Timeout if no data received for this duration
	startTime := time.Now()
	lastDataTime := time.Now()
	dataReceived := false

	// Process incoming data synchronously
	for {
		// Check for overall timeout
		if time.Since(startTime) > maxTotalTime {
			// If we have some data, return it anyway - might be a partial stream
			if len(streamData) > 0 {
				return streamData, nil
			}
			return nil, fmt.Errorf("stream read timeout: maximum time %v exceeded", maxTotalTime)
		}

		// Check for no data timeout
		if time.Since(lastDataTime) > noDataTimeout {
			// If we have some data, return it anyway - might be a partial stream
			if len(streamData) > 0 {
				return streamData, nil
			}
			// No data received at all
			if !dataReceived {
				return nil, fmt.Errorf("stream read timeout: no data received within %v", noDataTimeout)
			}
			// We received data before but now timed out - return what we have
			return streamData, nil
		}

		// Read data synchronously
		length, err := c.bulkIn.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read stream data: %w", err)
		}

		if length == 0 {
			// No data, but continue
			continue
		}

		// Update timing
		dataReceived = true
		lastDataTime = time.Now()

		// Copy data
		data := make([]byte, length)
		copy(data, buf[:length])

		// Append the data
		streamData = append(streamData, data...)

		// Stop processing if EOF found
		if c.findEndOfStream(data) {
			break
		}
	}

	return streamData, nil
}

// Decode OOB Index blocks from the byte stream
// Returns array of IndexTiming records
// Typical sequence of OOB blocks is:
//
//	KFInfo: infoData='name=KryoFlux DiskSystem, version=3.00s, date=Mar 27 2018, time=18:25:55,
//	                  hwid=1, hwrv=1, hs=1, sck=24027428.5714285, ick=3003428.5714285625'
//	Index: streamPosition=21154, sampleCounter=66, indexCounter=109798707
//	Index: streamPosition=96737, sampleCounter=66, indexCounter=110398148
//	Index: streamPosition=172321, sampleCounter=66, indexCounter=110997615
//	Index: streamPosition=247904, sampleCounter=66, indexCounter=111597074
//	Index: streamPosition=323485, sampleCounter=60, indexCounter=112196534
//	Index: streamPosition=399070, sampleCounter=66, indexCounter=112795973
//	StreamEnd: streamPosition=399071, resultCode=0
//	StreamInfo: streamPosition=399071, transferTime=0
func (c *Client) decodePulses(data []byte) []IndexTiming {

	var indexPulses []IndexTiming

	// Process the data
	offset := 0
	for {
		if offset >= len(data) {
			// No EOF found - stream is incomplete
			return indexPulses
		}
		val := data[offset]

		switch {
		case val <= 0x07:
			// Value: 2-byte sequence
			offset += 2
		case val == 0x08:
			// Nop1: 1 byte
			offset += 1
		case val == 0x09:
			// Nop2: 2 bytes
			offset += 2
		case val == 0x0a:
			// Nop3: 3 bytes
			offset += 3
		case val == 0x0b:
			// Overflow16: 1-byte
			offset++
		case val == 0x0c:
			// Value16: 3-byte sequence
			offset += 3
		case val == 0x0d:
			// OOB marker: 4-byte header + data
			if offset+4 > len(data) {
				// Lost OOB header
				return indexPulses
			}

			oobType := data[offset+1]
			if oobType == 0x0d {
				// End of stream marker
				return indexPulses
			}

			oobSize := int(data[offset+2]) | (int(data[offset+3]) << 8)
			if offset+4+oobSize > len(data) {
				// Lost OOB data
				return indexPulses
			}

			// Handle Index block (type 0x02)
			if oobType == 0x02 && oobSize >= 12 {
				//
				// Index block: Stream Position (4 bytes), Sample Counter (4 bytes),
				//              Index Counter (4 bytes)
				// Example:
				//      Index: streamPosition=21154, sampleCounter=66, indexCounter=109798707
				//      Index: streamPosition=96737, sampleCounter=66, indexCounter=110398148
				//
				streamPosition := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
				sampleCounter := binary.LittleEndian.Uint32(data[offset+8 : offset+12])
				indexCounter := binary.LittleEndian.Uint32(data[offset+12 : offset+16])
				if DebugFlag {
					fmt.Printf("--- Index: streamPosition=%d, sampleCounter=%d, indexCounter=%d\n",
						streamPosition, sampleCounter, indexCounter)
				}
				indexPulses = append(indexPulses, IndexTiming{
					streamPosition: streamPosition,
					sampleCounter:  sampleCounter,
					indexCounter:   indexCounter,
				})
			}

			// Handle StreamEnd block (type 0x03) - indicates stream has ended
			if oobType == 0x03 && oobSize >= 8 {
				// StreamEnd block: Stream Position (4 bytes), Result Code (4 bytes)
				streamPosition := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
				resultCode := binary.LittleEndian.Uint32(data[offset+8 : offset+12])
				if DebugFlag {
					fmt.Printf("--- StreamEnd: streamPosition=%d, resultCode=%d\n",
						streamPosition, resultCode)
				}
			}

			// Handle StreamInfo block (type 0x01) - provides information on the progress
			if oobType == 0x03 && oobSize >= 8 {
				// StreamEnd block: Stream Position (4 bytes), Transfer Time (4 bytes)
				streamPosition := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
				transferTime := binary.LittleEndian.Uint32(data[offset+8 : offset+12])
				if DebugFlag {
					fmt.Printf("--- StreamInfo: streamPosition=%d, transferTime=%d\n",
						streamPosition, transferTime)
				}
			}

			// Handle KFInfo block (type 0x04) to extract sample clock
			if oobType == 0x04 && oobSize > 0 {
				infoData := string(data[offset+4 : offset+4+int(oobSize)])
				if DebugFlag {
					fmt.Printf("--- KFInfo: infoData='%s'\n", infoData)
				}
			}

			offset += oobSize + 4
		case val >= 0xe:
			// Sample: 1-byte
			offset++
		}
	}
}

// Extract flux transitions.
func (c *Client) decodeFlux(data []byte, streamStart uint32, streamEnd uint32) ([]uint64, error) {

	ticksAccumulated := uint64(0)
	tickPeriodNs := 1e9 / DefaultSampleClock // Nanoseconds per tick

	// Collect all flux transitions with their absolute times in ticks
	// Filter transitions to only include those between first and second index
	var fluxTransitions []uint64

	if DebugFlag {
		fmt.Printf("--- decodeFlux() streamStart=%d, streamEnd=%d\n", streamStart, streamEnd)
		fmt.Printf("--- len(data) = %d\n", len(data))
	}
	i := streamStart
	for i < streamEnd {
		val := data[i]
		switch {
		case val <= 7:
			// Flux2 block: 2-byte sequence
			if i+1 >= streamEnd {
				return nil, fmt.Errorf("incomplete Flux2 block at offset %d", i)
			}
			fluxValue := (uint32(val) << 8) | uint32(data[i+1])
			ticksAccumulated += uint64(fluxValue)
			fluxNs := uint64(float64(ticksAccumulated) * tickPeriodNs)
			fluxTransitions = append(fluxTransitions, fluxNs)
			i += 2
		case val == 0x08:
			// NOP block: 1 byte
			i++
		case val == 0x09:
			// NOP block: 2 bytes
			i += 2
		case val == 0x0a:
			// NOP block: 3 bytes
			i += 3
		case val == 0x0b:
			// Ovl16 block: add 0x10000 to next flux value
			ticksAccumulated += 0x10000
			i++
		case val == 0x0c:
			// Flux3 block: 3-byte sequence
			if i+2 >= streamEnd {
				return nil, fmt.Errorf("incomplete Flux3 block at offset %d", i)
			}
			fluxValue := (uint32(data[i+1]) << 8) | uint32(data[i+2])
			ticksAccumulated += uint64(fluxValue)
			fluxNs := uint64(float64(ticksAccumulated) * tickPeriodNs)
			fluxTransitions = append(fluxTransitions, fluxNs)
			i += 3
		case val == 0x0d:
			// OOB block: 4-byte header + optional data
			if i+3 >= streamEnd {
				return nil, fmt.Errorf("incomplete OOB header at offset %d", i)
			}
			oobType := data[i+1]
			if oobType == 0x0d {
				// EOF marker - stop processing
				return fluxTransitions, nil
			}
			oobSize := uint32(data[i+2]) | (uint32(data[i+3]) << 8)
			if i+4+uint32(oobSize) > streamEnd {
				return nil, fmt.Errorf("incomplete OOB data at offset %d", i)
			}
			i += 4 + uint32(oobSize)
		default: // val >= 0x0e
			// Flux1 block: 1-byte (0x0E-0xFF)
			fluxValue := uint32(val)
			ticksAccumulated += uint64(fluxValue)
			fluxNs := uint64(float64(ticksAccumulated) * tickPeriodNs)
			fluxTransitions = append(fluxTransitions, fluxNs)
			i++
		}
	}
	if DebugFlag {
		fmt.Printf("--- len(fluxTransitions) = %d\n", len(fluxTransitions))
	}
	return fluxTransitions, nil
}

// Decode KryoFlux stream data to extract flux transitions and index pulses.
func (c *Client) decodeKryoFluxStream(data []byte) (*DecodedStreamData, error) {

	// Decode index pulses
	indexPulses := c.decodePulses(data)
	if len(indexPulses) < 2 {
		return nil, fmt.Errorf("no index pulses detected")
	}

	// Decode transitions between two indices
	fluxTransitions, err := c.decodeFlux(data, indexPulses[0].streamPosition,
		indexPulses[1].streamPosition)
	if err != nil {
		return nil, err
	}
	result := &DecodedStreamData{
		FluxTransitions: fluxTransitions,
		IndexPulses:     indexPulses,
	}
	return result, nil
}

// calculateRPMAndBitRate calculates RPM and bit rate from decoded stream data
func (c *Client) calculateRPMAndBitRate(decoded *DecodedStreamData) (uint16, uint16) {
	if len(decoded.IndexPulses) < 2 {
		return 300, 250 // Default RPM and bit rate
	}
	if DebugFlag {
		fmt.Printf("--- len(decoded.IndexPulses) = %d\n", len(decoded.IndexPulses))
	}

	// Calculate RPM from index pulse intervals
	// IndexPulses contains absolute times, so subtract to get interval
	trackIndexTicks := float64(decoded.IndexPulses[1].indexCounter - decoded.IndexPulses[0].indexCounter)
	trackDurationNs := uint64(trackIndexTicks / DefaultIndexClock * 1e9)
	if DebugFlag {
		fmt.Printf("--- track duration = %d nsec\n", trackDurationNs)
	}

	rpm := 60e9 / float64(trackDurationNs)
	if DebugFlag {
		fmt.Printf("--- rpm = %.2f\n", rpm)
	}

	// Round to either 300 or 360 RPM
	var roundedRPM uint16
	if rpm < 330 {
		roundedRPM = 300
	} else {
		roundedRPM = 360
	}

	// Calculate bit rate from transition count and track duration
	transitionCount := uint64(len(decoded.FluxTransitions))
	bitsPerMsec := transitionCount * 1e6 / trackDurationNs

	// Round to standard floppy drive bitrates: 250, 500, or 1000 kbps
	var roundedBitRate uint16
	if bitsPerMsec < 375 {
		roundedBitRate = 250
	} else if bitsPerMsec < 750 {
		roundedBitRate = 500
	} else {
		roundedBitRate = 1000
	}

	return roundedRPM, roundedBitRate
}

// kfFluxIterator provides flux intervals from KryoFlux decoded stream data
// It implements pll.FluxSource interface
type kfFluxIterator struct {
	transitions []uint64 // Absolute transition times in nanoseconds
	index       int      // Current index into transitions
	lastTime    uint64   // Last transition time (for calculating intervals)
}

// NextFlux returns the next flux interval in nanoseconds (time until next transition)
// Returns 0 if no more transitions available
// Implements pll.FluxSource interface
func (fi *kfFluxIterator) NextFlux() uint64 {
	if fi.index >= len(fi.transitions) {
		return 0 // No more transitions
	}

	nextTime := fi.transitions[fi.index]
	interval := nextTime - fi.lastTime
	fi.lastTime = nextTime
	fi.index++
	return interval
}

// Recover raw MFM bitcells from KryoFlux decoded stream data using PLL,
// and returns MFM bitcells as bytes (bitcells packed MSB-first, not decoded data bits)
func (c *Client) decodeFluxToMFM(decoded *DecodedStreamData, bitRateKhz uint16) ([]byte, error) {
	if len(decoded.FluxTransitions) == 0 {
		return nil, fmt.Errorf("no flux transitions found")
	}

	// Create flux iterator from transition times
	fi := &kfFluxIterator{
		transitions: decoded.FluxTransitions,
		index:       0,
		lastTime:    0, // Start from time 0
	}

	// Initialize PLL
	pllState := &pll.State{}
	pll.Init(pllState, bitRateKhz)

	// Ignore first half-bit (as done in reference implementation)
	_ = pll.NextBit(pllState, fi)

	// Generate MFM bitcells using PLL algorithm
	var bitcells []bool
	for {
		// Check if transitions are exhausted or nearly exhausted BEFORE generating more bits
		if fi.index >= len(decoded.FluxTransitions) {
			// Transitions exhausted - stop immediately
			break
		}

		first := pll.NextBit(pllState, fi)
		second := pll.NextBit(pllState, fi)

		bitcells = append(bitcells, first)
		bitcells = append(bitcells, second)
	}
	if DebugFlag {
		fmt.Printf("--- len(bitcells) = %d\n", len(bitcells))
	}

	if len(bitcells) == 0 {
		return nil, fmt.Errorf("no bitcells generated")
	}

	// Pack bitcells as bytes (MSB-first)
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
	if DebugFlag {
		fmt.Printf("--- len(mfmBytes) = %d\n", len(mfmBytes))
	}

	if len(mfmBytes) == 0 {
		return nil, fmt.Errorf("no MFM bytes generated")
	}

	return mfmBytes, nil
}

// Read reads the entire floppy disk and writes it to the specified filename as HFE format
func (c *Client) Read(filename string) error {
	NumberOfTracks := 82

	// Configure device with default values (device=0, density=0, minTrack=0, maxTrack=83)
	err := c.configure(0, 0, 0, NumberOfTracks-1)
	if err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Initialize HFE disk structure
	disk := &hfe.Disk{
		Header: hfe.Header{
			NumberOfTrack:       uint8(NumberOfTracks),
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
		Tracks: make([]hfe.TrackData, NumberOfTracks),
	}

	// Assume uknown bitrate
	disk.Header.BitRate = 0

	// Iterate through cylinders and sides
	for cyl := 0; cyl < NumberOfTracks; cyl++ {
		for side := 0; side < 2; side++ {
			// Print progress message
			if cyl != 0 || side != 0 {
				fmt.Printf("\rReading track %d, side %d...", cyl, side)
			}

			// Turn on motor and position head
			err = c.motorOn(side, cyl)
			if err != nil {
				fmt.Printf(" ERROR\n")
				c.motorOff()
				return fmt.Errorf("failed to position head at track %d, side %d: %v", cyl, side, err)
			}

			// Capture stream data to memory
			streamData, err := c.captureStream()
			if err != nil {
				fmt.Printf(" ERROR\n")
				c.motorOff()
				return fmt.Errorf("failed to capture stream from track %d, side %d: %v", cyl, side, err)
			}

			// Decode stream data to extract flux transitions
			decoded, err := c.decodeKryoFluxStream(streamData)
			if err != nil {
				fmt.Printf(" ERROR\n")
				c.motorOff()
				return fmt.Errorf("failed to decode stream from track %d, side %d: %v", cyl, side, err)
			}

			// Calculate RPM and BitRate from first track
			if disk.Header.BitRate == 0 {
				calculatedRPM, calculatedBitRate := c.calculateRPMAndBitRate(decoded)
				fmt.Printf("Rotation Speed: %d RPM\n", calculatedRPM)
				fmt.Printf("Bit Rate: %d kbps\n", calculatedBitRate)

				disk.Header.FloppyRPM = calculatedRPM
				disk.Header.BitRate = calculatedBitRate
			}

			// Decode flux data to MFM bitstream
			mfmBitstream, err := c.decodeFluxToMFM(decoded, disk.Header.BitRate)
			if err != nil {
				fmt.Printf(" ERROR\n")
				c.motorOff()
				return fmt.Errorf("failed to decode flux data to MFM from track %d, side %d: %v", cyl, side, err)
			}

			// Store MFM bitstream in appropriate side
			if side == 0 {
				disk.Tracks[cyl].Side0 = mfmBitstream
			} else {
				disk.Tracks[cyl].Side1 = mfmBitstream
			}
		}
	}
	fmt.Printf(" Done\n")

	// Turn off motor
	err = c.motorOff()
	if err != nil {
		return fmt.Errorf("failed to turn off motor: %w", err)
	}

	// Write HFE file
	fmt.Printf("Writing HFE file...\n")
	err = hfe.Write(filename, disk)
	if err != nil {
		return fmt.Errorf("failed to write HFE file: %w", err)
	}

	return nil
}
