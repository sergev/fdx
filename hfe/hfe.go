package hfe

// HFEVersion represents the HFE file format version
type HFEVersion int

const (
	HFEVersion1 HFEVersion = 1
	HFEVersion3 HFEVersion = 3
)

// Constants for HFE format signatures
const (
	// Signature for HFE v1 format
	HFEv1Signature = "HXCPICFE"

	// Signature for HFE v3 format
	HFEv3Signature = "HXCHFEV3"

	// Opcode constants (used in v3)
	OPCODE_MASK       = 0xF0
	NOP_OPCODE        = 0xF0
	SETINDEX_OPCODE   = 0xF1
	SETBITRATE_OPCODE = 0xF2
	SKIPBITS_OPCODE   = 0xF3
	RAND_OPCODE       = 0xF4

	// Floppy emulator frequency (Hz)
	FLOPPYEMUFREQ = 36000000

	// Block size in bytes
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
	HeaderSignature     [8]byte
	FormatRevision      uint8
	NumberOfTrack       uint8
	NumberOfSide        uint8
	TrackEncoding       uint8
	BitRate             uint16 // in kB/s
	FloppyRPM           uint16
	FloppyInterfaceMode uint8
	WriteProtected      uint8
	TrackListOffset     uint16 // in 512-byte blocks
	WriteAllowed        uint8
	SingleStep          uint8
	Track0S0AltEncoding uint8
	Track0S0Encoding    uint8
	Track0S1AltEncoding uint8
	Track0S1Encoding    uint8
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
