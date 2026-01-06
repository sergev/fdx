package hfe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Constants for HFE v3 format
const (
	// Signature for HFE v3 format
	HFEv3Signature = "HXCHFEV3"

	// Opcode constants
	OPCODE_MASK       = 0xF0
	NOP_OPCODE       = 0xF0
	SETINDEX_OPCODE  = 0xF1
	SETBITRATE_OPCODE = 0xF2
	SKIPBITS_OPCODE  = 0xF3
	RAND_OPCODE      = 0xF4

	// Floppy emulator frequency (Hz)
	FLOPPYEMUFREQ = 36000000

	// Block size (512 bytes)
	BlockSize = 512
)

// Track encoding types
const (
	ENC_ISOIBM_MFM = iota
	ENC_Amiga_MFM
	ENC_ISOIBM_FM
	ENC_Emu_FM
	ENC_Unknown = 0xff
)

// Interface mode types
const (
	IFM_IBMPC_DD = iota
	IFM_IBMPC_HD
	IFM_AtariST_DD
	IFM_AtariST_HD
	IFM_Amiga_DD
	IFM_Amiga_HD
	IFM_CPC_DD
	IFM_GenericShugart_DD
	IFM_IBMPC_ED
	IFM_MSX2_DD
	IFM_C64_DD
	IFM_EmuShugart_DD
)

// Header represents the HFE v3 file header
type Header struct {
	HeaderSignature      [8]byte
	FormatRevision       uint8
	NumberOfTrack        uint8
	NumberOfSide         uint8
	TrackEncoding        uint8
	BitRate              uint16 // in kB/s
	FloppyRPM            uint16
	FloppyInterfaceMode  uint8
	WriteProtected       uint8
	TrackListOffset      uint16 // in 512-byte blocks
	WriteAllowed         uint8
	SingleStep           uint8
	Track0S0AltEncoding  uint8
	Track0S0Encoding      uint8
	Track0S1AltEncoding  uint8
	Track0S1Encoding      uint8
}

// TrackHeader represents a track offset entry in the track list
type TrackHeader struct {
	Offset   uint16 // in 512-byte blocks
	TrackLen uint16 // in bytes
}

// TrackData represents the MFM bitstream data for a track
type TrackData struct {
	Side0 []byte // MFM bitstream for side 0 (bits, MSB-first)
	Side1 []byte // MFM bitstream for side 1 (bits, MSB-first)
}

// Disk represents a complete HFE v3 disk image
type Disk struct {
	Header Header
	Tracks []TrackData
}

// byteBitsInverter inverts bits in a byte (for PIC EUSART compatibility)
// This is a lookup table that inverts each bit position
var byteBitsInverter [256]byte

func init() {
	// Generate byteBitsInverter lookup table
	// This inverts bits: bit 0 <-> bit 7, bit 1 <-> bit 6, etc.
	for i := 0; i < 256; i++ {
		var inverted byte
		for j := 0; j < 8; j++ {
			if (i & (1 << j)) != 0 {
				inverted |= 1 << (7 - j)
			}
		}
		byteBitsInverter[i] = inverted
	}
}

// bitReverse reverses the bit order in a byte (LSB-first <-> MSB-first)
func bitReverse(b byte) byte {
	var result byte
	for i := 0; i < 8; i++ {
		result <<= 1
		result |= b & 1
		b >>= 1
	}
	return result
}

// bitReverseBlock reverses bits in a block of bytes
func bitReverseBlock(data []byte) {
	for i := range data {
		data[i] = bitReverse(data[i])
	}
}

// bitCopy copies bits from source to destination at arbitrary bit offsets
func bitCopy(dst []byte, dstOff int, src []byte, srcOff int, size int) int {
	for i := 0; i < size; i++ {
		if srcOff >= len(src)*8 || dstOff >= len(dst)*8 {
			return dstOff
		}

		// Get source bit
		srcByte := src[srcOff/8]
		srcBit := (srcByte >> (7 - (srcOff & 7))) & 1

		// Set destination bit
		if srcBit != 0 {
			dst[dstOff/8] |= 1 << (7 - (dstOff & 7))
		} else {
			dst[dstOff/8] &= ^(1 << (7 - (dstOff & 7)))
		}

		srcOff++
		dstOff++
	}
	return dstOff
}

// Read reads an HFE v3 file and returns a Disk structure
func Read(filename string) (*Disk, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	disk := &Disk{}

	// Read header
	if err := binary.Read(file, binary.LittleEndian, &disk.Header); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Validate signature
	if string(disk.Header.HeaderSignature[:]) != HFEv3Signature {
		return nil, errors.New("invalid HFE v3 signature")
	}

	// Validate format revision
	if disk.Header.FormatRevision != 0 {
		return nil, fmt.Errorf("unsupported format revision: %d", disk.Header.FormatRevision)
	}

	// Validate basic fields
	if disk.Header.NumberOfTrack == 0 || disk.Header.NumberOfSide == 0 {
		return nil, errors.New("invalid number of tracks or sides")
	}

	// Read track offset list
	trackListOffset := int64(disk.Header.TrackListOffset) * BlockSize
	if _, err := file.Seek(trackListOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to track list: %w", err)
	}

	trackHeaders := make([]TrackHeader, disk.Header.NumberOfTrack)
	for i := range trackHeaders {
		if err := binary.Read(file, binary.LittleEndian, &trackHeaders[i]); err != nil {
			return nil, fmt.Errorf("failed to read track header %d: %w", i, err)
		}
	}

	// Initialize tracks
	disk.Tracks = make([]TrackData, disk.Header.NumberOfTrack)

	// Read each track
	for i := range trackHeaders {
		trackData, err := readTrack(file, &trackHeaders[i], disk.Header.NumberOfSide)
		if err != nil {
			return nil, fmt.Errorf("failed to read track %d: %w", i, err)
		}
		disk.Tracks[i] = *trackData
	}

	return disk, nil
}

// readTrack reads a single track from the file
func readTrack(file *os.File, th *TrackHeader, numSides uint8) (*TrackData, error) {
	// Calculate track length (rounded up to 512-byte boundary)
	trackLen := int(th.TrackLen)
	if trackLen&0x1FF != 0 {
		trackLen = (trackLen & ^0x1FF) + 0x200
	}

	// Seek to track data
	trackOffset := int64(th.Offset) * BlockSize
	if _, err := file.Seek(trackOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to track data: %w", err)
	}

	// Read track data
	trackBuf := make([]byte, trackLen)
	if _, err := io.ReadFull(file, trackBuf); err != nil {
		return nil, fmt.Errorf("failed to read track data: %w", err)
	}

	// Demux sides: side 0 is bytes 0-255, side 1 is bytes 256-511 of each 512-byte block
	// Apply byteBitsInverter during demuxing (convert from LSB-first to MSB-first)
	side0Data := make([]byte, trackLen/2)
	side1Data := make([]byte, trackLen/2)

	for j := 0; j < trackLen; j += BlockSize {
		for k := 0; k < 256; k++ {
			side0Data[j/2+k] = byteBitsInverter[trackBuf[j+k]]
			if numSides > 1 {
				side1Data[j/2+k] = byteBitsInverter[trackBuf[j+256+k]]
			}
		}
	}

	// Process opcodes for each side
	side0Bits, err := processOpcodes(side0Data)
	if err != nil {
		return nil, fmt.Errorf("failed to process opcodes for side 0: %w", err)
	}

	var side1Bits []byte
	if numSides > 1 {
		side1Bits, err = processOpcodes(side1Data)
		if err != nil {
			return nil, fmt.Errorf("failed to process opcodes for side 1: %w", err)
		}
	}

	return &TrackData{
		Side0: side0Bits,
		Side1: side1Bits,
	}, nil
}

// processOpcodes processes HFEv3 opcodes and extracts the MFM bitstream
func processOpcodes(data []byte) ([]byte, error) {
	// Allocate enough space for output (may be smaller than input due to opcodes)
	newData := make([]byte, len(data))
	// Initialize to zeros
	for i := range newData {
		newData[i] = 0
	}

	bitrate := byte(0)
	bitrates := make([]byte, len(data)+1)

	inBit := 0
	outBit := 0
	indexBit := 0

	for inBit/8 < len(data) {
		if inBit&7 != 0 {
			return nil, errors.New("opcode processing: input not byte-aligned")
		}

		bitrates[outBit/8] = bitrate
		opc := data[inBit/8]

		if (opc & OPCODE_MASK) == OPCODE_MASK {
			switch opc & 0x0F {
			case NOP_OPCODE & 0x0F:
				// NOP: skip 8 bits (no output)
				inBit += 8

			case SETINDEX_OPCODE & 0x0F:
				// SETINDEX: mark index pulse position
				inBit += 8
				indexBit = outBit

			case SETBITRATE_OPCODE & 0x0F:
				// SETBITRATE: change bitrate
				if inBit/8+1 >= len(data) {
					return nil, errors.New("SETBITRATE opcode: insufficient data")
				}
				bitrate = data[inBit/8+1]
				inBit += 16

			case SKIPBITS_OPCODE & 0x0F:
				// SKIPBITS: skip 0-8 bits in next byte, then copy remaining
				if inBit/8+1 >= len(data) {
					return nil, errors.New("SKIPBITS opcode: insufficient data")
				}
				skip := data[inBit/8+1]
				if skip > 8 {
					return nil, fmt.Errorf("SKIPBITS opcode: skip value %d > 8", skip)
				}
				// Skip the opcode byte and skip value byte, then skip bits
				inBit += 16 + int(skip)
				// Copy remaining bits (8 - skip)
				bitCopy(newData, outBit, data, inBit, 8-int(skip))
				inBit += 8 - int(skip)
				outBit += 8 - int(skip)

			case RAND_OPCODE & 0x0F:
				// RAND: random/weak byte - write zeros (or could use random data)
				// For now, write zeros to maintain track length
				inBit += 8
				// Write 8 zero bits
				outBit += 8

			default:
				return nil, fmt.Errorf("unknown opcode: 0x%02X", opc)
			}
		} else {
			// Regular data byte - copy 8 bits
			// Check if this byte was escaped (XORed with 0x90 during encoding)
			// Bytes in 0x60-0x6F range might be escaped opcodes (0xF0-0xFF XOR 0x90)
			dataByte := data[inBit/8]
			// XOR-back if in the escaped range (0x60-0x6F)
			// This recovers bytes that were in 0xF0-0xFF range (except 0xF4)
			if dataByte >= 0x60 && dataByte <= 0x6F {
				dataByte ^= 0x90
			}
			bitCopy(newData, outBit, []byte{dataByte}, 0, 8)
			inBit += 8
			outBit += 8
		}
	}

	bitrates[outBit/8] = bitrate
	lenBits := outBit

	// Rotate track so index pulse is at bit 0
	// If no index was found, indexBit will be 0 (start of track)
	result := make([]byte, (lenBits+7)/8)
	if indexBit < lenBits {
		// Copy from index to end, then from start to index
		bitCopy(result, 0, newData, indexBit, lenBits-indexBit)
		bitCopy(result, lenBits-indexBit, newData, 0, indexBit)
	} else {
		// No index found, just copy data as-is
		copy(result, newData[:lenBits/8])
	}

	return result, nil
}

// Write writes a Disk structure to an HFE v3 file
func Write(filename string, disk *Disk) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Prepare header
	header := disk.Header
	copy(header.HeaderSignature[:], HFEv3Signature)
	header.FormatRevision = 0
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

	// First pass: encode all tracks and calculate exact encoded lengths
	// This matches the legacy implementation which encodes first, then calculates lengths
	type encodedTrack struct {
		side0 []byte
		side1 []byte
	}
	encodedTracks := make([]encodedTrack, len(disk.Tracks))
	bitrateKbps := disk.Header.BitRate
	for i, track := range disk.Tracks {
		encodedTracks[i].side0 = encodeOpcodes(track.Side0, bitrateKbps)
		if disk.Header.NumberOfSide > 1 {
			encodedTracks[i].side1 = encodeOpcodes(track.Side1, bitrateKbps)
		} else {
			encodedTracks[i].side1 = encodedTracks[i].side0
		}
	}

	// Second pass: calculate track offsets using exact encoded lengths
	trackHeaders := make([]TrackHeader, len(disk.Tracks))
	for i := range encodedTracks {
		// Calculate maximum encoded length (max of both sides)
		maxEncodedLen := len(encodedTracks[i].side0)
		if len(encodedTracks[i].side1) > maxEncodedLen {
			maxEncodedLen = len(encodedTracks[i].side1)
		}

		// Track length is for both sides: bytelen = maxEncodedLen * 2
		bytelen := maxEncodedLen * 2

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

	// Write track data using pre-encoded tracks
	for i := range encodedTracks {
		if err := writeEncodedTrack(file, &trackHeaders[i], encodedTracks[i].side0, encodedTracks[i].side1, disk.Header.NumberOfSide); err != nil {
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
		if (b & OPCODE_MASK) == OPCODE_MASK && b != RAND_OPCODE {
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
