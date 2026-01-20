package hfe

import (
	"encoding/binary"
	"fmt"
	"os"
)

// Write a Disk structure to a file, according to it's format.
func Write(filename string, disk *Disk) error {
	format := DetectImageFormat(filename)
	switch format {
	case ImageFormatHFE:
		return WriteHFE(filename, disk, HFEVersion1)
	case ImageFormatADF:
		return WriteADF(filename, disk)
	case ImageFormatBKD:
		return WriteBKD(filename, disk)
	case ImageFormatCP2:
		return WriteCP2(filename, disk)
	case ImageFormatDCF:
		return WriteDCF(filename, disk)
	case ImageFormatEPL:
		return WriteEPL(filename, disk)
	case ImageFormatIMD:
		return WriteIMD(filename, disk)
	case ImageFormatIMG:
		return WriteIMG(filename, disk)
	case ImageFormatMFM:
		return WriteMFM(filename, disk)
	case ImageFormatPDI:
		return WritePDI(filename, disk)
	case ImageFormatPRI:
		return WritePRI(filename, disk)
	case ImageFormatPSI:
		return WritePSI(filename, disk)
	case ImageFormatSCP:
		return WriteSCP(filename, disk)
	case ImageFormatTD0:
		return WriteTD0(filename, disk)
	default:
		return fmt.Errorf("unknown or unsupported image format for file: %s", filename)
	}
}

// Write a Disk structure to an HFE file.
// version specifies the HFE format version (1, 2, or 3)
func WriteHFE(filename string, disk *Disk, version HFEVersion) error {
	// Validate version
	if version != HFEVersion1 && version != HFEVersion3 {
		return fmt.Errorf("invalid HFE version: %d (must be 1 or 3)", version)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Prepare header
	header := disk.Header

	// Set header signature and format revision based on version
	switch version {
	case HFEVersion1:
		copy(header.HeaderSignature[:], HFEv1Signature)
		header.FormatRevision = 0
	case HFEVersion3:
		copy(header.HeaderSignature[:], HFEv3Signature)
		header.FormatRevision = 0
	}
	header.TrackListOffset = 1

	// Write header (512 bytes, padded with 0xFF)
	headerBuf := make([]byte, BlockSize)
	for i := range headerBuf {
		headerBuf[i] = 0xFF
	}

	// Write header data (first 32 bytes)
	headerData := make([]byte, 32)
	copy(headerData[0:8], header.HeaderSignature[:])
	headerData[8] = header.FormatRevision
	headerData[9] = header.NumberOfTrack
	headerData[10] = header.NumberOfSide
	headerData[11] = header.TrackEncoding
	binary.LittleEndian.PutUint16(headerData[12:14], header.BitRate)
	binary.LittleEndian.PutUint16(headerData[14:16], header.FloppyRPM)
	headerData[16] = header.FloppyInterfaceMode
	headerData[17] = header.WriteProtected
	binary.LittleEndian.PutUint16(headerData[18:20], header.TrackListOffset)
	headerData[20] = header.WriteAllowed
	headerData[21] = header.SingleStep
	headerData[22] = header.Track0S0AltEncoding
	headerData[23] = header.Track0S0Encoding
	headerData[24] = header.Track0S1AltEncoding
	headerData[25] = header.Track0S1Encoding

	copy(headerBuf, headerData)

	if _, err := file.Write(headerBuf); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Calculate track offsets and write track list
	trackListBuf := make([]byte, BlockSize)
	for i := range trackListBuf {
		trackListBuf[i] = 0xFF
	}

	trackPos := uint16(header.TrackListOffset + 1) // Start after track list block

	// Prepare track data based on version
	type trackData struct {
		side0 []byte
		side1 []byte
	}
	tracks := make([]trackData, len(disk.Tracks))
	bitrateKbps := disk.Header.BitRate

	if version == HFEVersion3 {
		// For v3: encode tracks with opcodes
		for i, track := range disk.Tracks {
			tracks[i].side0 = encodeOpcodes(track.Side0, bitrateKbps)
			if disk.Header.NumberOfSide > 1 {
				tracks[i].side1 = encodeOpcodes(track.Side1, bitrateKbps)
			} else {
				tracks[i].side1 = tracks[i].side0
			}
		}
	} else {
		// For v1: use raw track data (no opcode encoding)
		for i, track := range disk.Tracks {
			tracks[i].side0 = track.Side0
			if disk.Header.NumberOfSide > 1 {
				tracks[i].side1 = track.Side1
			} else {
				tracks[i].side1 = tracks[i].side0
			}
		}
	}

	// Calculate track offsets using track lengths
	trackHeaders := make([]TrackHeader, len(disk.Tracks))
	for i := range tracks {
		// Calculate maximum length (max of both sides)
		maxLen := len(tracks[i].side0)
		if len(tracks[i].side1) > maxLen {
			maxLen = len(tracks[i].side1)
		}

		// Track length is for both sides: bytelen = maxLen * 2
		bytelen := maxLen * 2

		// Round up to 512-byte boundary
		trackLen := bytelen
		if trackLen%BlockSize != 0 {
			trackLen = ((trackLen / BlockSize) + 1) * BlockSize
		}

		trackHeaders[i].Offset = trackPos
		trackHeaders[i].TrackLen = uint16(trackLen)

		// Calculate next track position (in 512-byte blocks)
		trackPos += uint16(trackLen / BlockSize)
	}

	// Write track list
	for i, th := range trackHeaders {
		offset := i * 4
		if offset+4 > len(trackListBuf) {
			// Need to extend track list buffer
			// For now, assume we fit in one block (up to 128 tracks)
			if i >= 128 {
				return fmt.Errorf("too many tracks for single track list block")
			}
		}
		binary.LittleEndian.PutUint16(trackListBuf[offset:offset+2], th.Offset)
		binary.LittleEndian.PutUint16(trackListBuf[offset+2:offset+4], th.TrackLen)
	}

	if _, err := file.Write(trackListBuf); err != nil {
		return fmt.Errorf("failed to write track list: %w", err)
	}

	// Write track data using appropriate function based on version
	for i := range tracks {
		var err error
		if version == HFEVersion3 {
			// v3: use opcode-encoded track writer
			err = writeEncodedTrack(file, &trackHeaders[i], tracks[i].side0, tracks[i].side1, disk.Header.NumberOfSide)
		} else {
			// v1: use raw track writer (no opcodes)
			err = writeRawTrack(file, &trackHeaders[i], tracks[i].side0, tracks[i].side1, disk.Header.NumberOfSide)
		}
		if err != nil {
			return fmt.Errorf("failed to write track %d: %w", i, err)
		}
	}

	return nil
}

// Encode raw MFM bitstream data with HFEv3 opcodes
func encodeOpcodes(data []byte, bitrateKbps uint16) []byte {
	// Allocate output buffer (worst case: all bytes need escaping)
	result := make([]byte, 0, len(data))

	// Process each data byte
	for _, b := range data {
		// Escape bytes in opcode range (0xF0-0xFF) except RAND_OPCODE (0xF4)
		// by XORing with 0x90 (per adjustrand function in legacy code)
		if (b&OPCODE_MASK) == OPCODE_MASK && b != RAND_OPCODE {
			// Escape by XORing with 0x90
			result = append(result, b^0x90)
		} else {
			// Write byte as-is
			result = append(result, b)
		}
	}

	return result
}

// writeEncodedTrack writes pre-encoded track data to the file
func writeEncodedTrack(file *os.File, th *TrackHeader, encodedSide0, encodedSide1 []byte, numSides uint8) error {
	trackLen := int(th.TrackLen)

	// Allocate buffers for each side (padded to trackLen/2)
	side0Buf := make([]byte, trackLen/2)
	side1Buf := make([]byte, trackLen/2)

	// Copy encoded data and pad with NOP opcodes
	copy(side0Buf, encodedSide0)
	for i := len(encodedSide0); i < len(side0Buf); i++ {
		side0Buf[i] = NOP_OPCODE
	}

	if numSides > 1 {
		copy(side1Buf, encodedSide1)
		for i := len(encodedSide1); i < len(side1Buf); i++ {
			side1Buf[i] = NOP_OPCODE
		}
	} else {
		copy(side1Buf, side0Buf)
	}

	// Interleave side0 and side1 data into track buffer
	// Side 0: bytes 0-255 of each 512-byte block
	// Side 1: bytes 256-511 of each 512-byte block
	trackBuf := make([]byte, trackLen)
	for k := 0; k < trackLen/BlockSize; k++ {
		for j := 0; j < 256; j++ {
			// Head 0
			trackBuf[k*BlockSize+j] = byteBitsInverter[side0Buf[k*256+j]]
			// Head 1
			trackBuf[k*BlockSize+j+256] = byteBitsInverter[side1Buf[k*256+j]]
		}
	}

	// Write to file
	if _, err := file.Write(trackBuf); err != nil {
		return fmt.Errorf("failed to write track data: %w", err)
	}

	return nil
}

// writeRawTrack writes raw track data to the file (for v1 format, no opcodes)
func writeRawTrack(file *os.File, th *TrackHeader, side0, side1 []byte, numSides uint8) error {
	trackLen := int(th.TrackLen)

	// Allocate buffers for each side (padded to trackLen/2)
	side0Buf := make([]byte, trackLen/2)
	side1Buf := make([]byte, trackLen/2)

	// Copy raw data and pad with 0xFF (not NOP opcodes)
	copy(side0Buf, side0)
	for i := len(side0); i < len(side0Buf); i++ {
		side0Buf[i] = 0xFF
	}

	if numSides > 1 {
		copy(side1Buf, side1)
		for i := len(side1); i < len(side1Buf); i++ {
			side1Buf[i] = 0xFF
		}
	} else {
		copy(side1Buf, side0Buf)
	}

	// Interleave side0 and side1 data into track buffer
	// Side 0: bytes 0-255 of each 512-byte block
	// Side 1: bytes 256-511 of each 512-byte block
	trackBuf := make([]byte, trackLen)
	for k := 0; k < trackLen/BlockSize; k++ {
		for j := 0; j < 256; j++ {
			// Head 0
			trackBuf[k*BlockSize+j] = byteBitsInverter[side0Buf[k*256+j]]
			// Head 1
			trackBuf[k*BlockSize+j+256] = byteBitsInverter[side1Buf[k*256+j]]
		}
	}

	// Write to file
	if _, err := file.Write(trackBuf); err != nil {
		return fmt.Errorf("failed to write track data: %w", err)
	}

	return nil
}

// writeBits writes bits from a bitstream to a buffer at a specific offset
// The bits are written in MSB-first order (will be reversed later)
// This follows the pattern from hfe.c write_bits function
// dstOffset is the starting byte offset in the destination buffer
// lenBytes is the number of bytes to write for this side
func writeBits(bits []byte, dst []byte, dstOffset int, lenBytes int) {
	bitPos := 0 // Position in source bitstream
	var x byte  // Accumulator for current byte

	for i := 0; i < lenBytes*8; i++ {
		// Consume a bit from source (MSB-first)
		if bitPos < len(bits)*8 {
			srcByte := bits[bitPos/8]
			bit := (srcByte >> (7 - (bitPos & 7))) & 1
			x = (x << 1) | bit
		} else {
			// Pad with zeros after track data
			x = x << 1
		}

		// Write byte when we have 8 bits
		if (i+1)%8 == 0 {
			// Calculate destination byte index
			// Every 256 bytes written, we skip 256 bytes in destination (for interleaving)
			bytesWritten := i / 8
			blockNum := bytesWritten / 256
			byteInBlock := bytesWritten % 256
			dstIdx := dstOffset + blockNum*512 + byteInBlock

			if dstIdx < len(dst) {
				dst[dstIdx] = x
			}
			x = 0
		}

		// Wrap around track
		bitPos++
		if bitPos >= len(bits)*8 {
			bitPos = 0
			// Add padding: repeat last 16 bits as extra gap when wrapping
			// This happens every 16 bits after we've written all track data
			bitsWritten := i + 1 - len(bits)*8
			if bitsWritten > 0 && bitsWritten%16 == 0 {
				// Go back 16 bits in source
				newBitPos := len(bits)*8 - 16
				if newBitPos < 0 {
					newBitPos = 0
				}
				bitPos = newBitPos
			}
		}
	}
}
