package hfe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Read a disk image file and return a Disk structure.
// The format is automatically detected from the file extension.
func Read(filename string) (*Disk, error) {
	format := DetectImageFormat(filename)
	switch format {
	case ImageFormatHFE:
		return ReadHFE(filename)
	case ImageFormatADF:
		return ReadADF(filename)
	case ImageFormatCP2:
		return ReadCP2(filename)
	case ImageFormatDCF:
		return ReadDCF(filename)
	case ImageFormatEPL:
		return ReadEPL(filename)
	case ImageFormatIMD:
		return ReadIMD(filename)
	case ImageFormatIMG:
		return ReadIMG(filename)
	case ImageFormatMFM:
		return ReadMFM(filename)
	case ImageFormatPDI:
		return ReadPDI(filename)
	case ImageFormatPRI:
		return ReadPRI(filename)
	case ImageFormatPSI:
		return ReadPSI(filename)
	case ImageFormatSCP:
		return ReadSCP(filename)
	case ImageFormatTD0:
		return ReadTD0(filename)
	default:
		return nil, fmt.Errorf("unknown or unsupported image format for file: %s", filename)
	}
}

// ReadHFE reads an HFE file (v1 or v3) and return a Disk structure
// Supports HFE format versions:
//   - v1: signature "HXCPICFE", format revision 0
//   - v3: signature "HXCHFEV3", format revision 0
//
// v2 format is not supported and will return an error
func ReadHFE(filename string) (*Disk, error) {
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

	// Validate signature - support v1 (HXCPICFE) and v3 (HXCHFEV3)
	sig := string(disk.Header.HeaderSignature[:])
	isV1 := sig == HFEv1Signature
	isV3 := sig == HFEv3Signature

	if !isV1 && !isV3 {
		return nil, fmt.Errorf("invalid HFE signature: %s (expected %s or %s)", sig, HFEv1Signature, HFEv3Signature)
	}

	// Validate format revision based on signature
	if isV3 {
		// v3: format revision must be 0
		if disk.Header.FormatRevision != 0 {
			return nil, fmt.Errorf("invalid HFE v3 format revision: %d (expected 0)", disk.Header.FormatRevision)
		}
	} else if isV1 {
		// v1: format revision must be 0
		// v2 (revision 1) is not supported
		if disk.Header.FormatRevision == 1 {
			return nil, fmt.Errorf("HFE v2 format (revision 1) is not supported, only v1 and v3 are supported")
		}
		if disk.Header.FormatRevision != 0 {
			return nil, fmt.Errorf("invalid HFE v1 format revision: %d (expected 0)", disk.Header.FormatRevision)
		}
	}

	// Validate basic fields
	if disk.Header.BitRate == 0 {
		return nil, errors.New("invalid bit rate")
	}
	if disk.Header.NumberOfTrack == 0 {
		return nil, errors.New("invalid number of tracks")
	}
	if disk.Header.NumberOfSide == 0 {
		return nil, errors.New("invalid number of sides")
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

	// Determine if we need to process opcodes (only for v3)
	shouldProcessOpcodes := isV3

	// Read each track
	for i := range trackHeaders {
		trackData, err := readTrack(file, &trackHeaders[i], disk.Header.NumberOfSide, shouldProcessOpcodes)
		if err != nil {
			return nil, fmt.Errorf("failed to read track %d: %w", i, err)
		}
		disk.Tracks[i] = *trackData
	}

	// Compute FloppyRPM from track #0 length if not set
	if disk.Header.FloppyRPM == 0 {
		trackBits := len(disk.Tracks[0].Side0) * 8
		if trackBits == 0 {
			return nil, errors.New("unknown RPM")
		}
		rpm := (60 * uint32(disk.Header.BitRate) * 2000) / uint32(trackBits)
		if rpm > 400 || rpm < 250 {
			return nil, errors.New("bad RPM")
		}

		// Round to either 300 or 360 RPM (standard floppy drive speeds)
		// Use 330 RPM as the threshold (midpoint between 300 and 360)
		if rpm < 330 {
			disk.Header.FloppyRPM = 300
		} else {
			disk.Header.FloppyRPM = 360
		}
	}

	return disk, nil
}

// readTrack reads a single track from the file
// shouldProcessOpcodes indicates whether to process HFEv3 opcodes (true for v3, false for v1)
func readTrack(file *os.File, th *TrackHeader, numSides uint8, shouldProcessOpcodes bool) (*TrackData, error) {
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

	// Process opcodes for each side (only for v3 format)
	var side0Bits, side1Bits []byte
	var err error

	if shouldProcessOpcodes {
		// v3 format: process opcodes
		side0Bits, err = processOpcodes(side0Data)
		if err != nil {
			return nil, fmt.Errorf("failed to process opcodes for side 0: %w", err)
		}

		if numSides > 1 {
			side1Bits, err = processOpcodes(side1Data)
			if err != nil {
				return nil, fmt.Errorf("failed to process opcodes for side 1: %w", err)
			}
		}
	} else {
		// v1 format: use raw data directly (no opcode processing)
		side0Bits = side0Data
		if numSides > 1 {
			side1Bits = side1Data
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
