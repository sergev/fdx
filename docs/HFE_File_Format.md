# HFE File Format Specification

This document describes the HFE (HxC Floppy Emulator) file format as implemented in this project. The HFE format is a bitstream container format designed for floppy disk emulation that stores floppy media content at the bit-cell level.

**Reference:** [HxC Floppy Emulator HFE File Format](https://hxc2001.com/floppy_drive_emulator/HFE-file-format.html)

## Overview

The HFE format stores low-level bitstream data from floppy disks, preserving all metadata, sectors, error detection codes (CRC), gaps, and other disk format information. Unlike high-level image formats, HFE maintains the original disk structure, making it suitable for accurate emulation of various floppy formats.

### What is HFE?

**HFE** stands for **H**xC **F**loppy **E**mulator. The format was originally designed for HxC Floppy Emulator devices (see [hxc2001.com](https://hxc2001.com)).

### Why HFE Format?

- **Universal format**: Supports multiple floppy formats without format-specific configuration
- **Low-level accuracy**: Stores data at the bit-cell level, preserving all disk metadata
- **Data integrity**: Maintains original checksums and error detection mechanisms
- **No guessing**: Contains all necessary encoding and layout information

## File Structure

The HFE file consists of three main parts:

1. **Header** (512 bytes at offset 0x0000)
2. **Track List** (Track offset LUT, starting at offset specified in header)
3. **Track Data** (Bitstream buffers for each track)

```
┌─────────────────┐
│  Header (512B)  │  Offset 0x0000
├─────────────────┤
│ Track List LUT  │  Offset from header.track_list_offset
├─────────────────┤
│  Track 0 Data   │
├─────────────────┤
│  Track 1 Data   │
├─────────────────┤
│      ...        │
└─────────────────┘
```

## Header Structure (512 bytes)

The header contains metadata about the disk image:

```go
type Header struct {
    HeaderSignature      [8]byte   // "HXCHFEV3" for HFEv3
    FormatRevision       uint8     // 0 for HFEv1/HFEv3, 1 for HFEv2
    NumberOfTrack        uint8     // Number of tracks
    NumberOfSide         uint8     // Number of sides (not used by emulator)
    TrackEncoding        uint8     // Track encoding mode
    BitRate              uint16    // Bitrate in Kbit/s (max 1000)
    FloppyRPM            uint16    // Rotations per minute (not used)
    FloppyInterfaceMode   uint8     // Floppy interface mode
    WriteProtected       uint8     // 0x00=protected, 0xFF=unprotected (v1.1: was "dnu")
    TrackListOffset      uint16    // Offset of track list in 512-byte blocks
    WriteAllowed         uint8     // 0x00=protected, 0xFF=unprotected
    SingleStep           uint8     // 0xFF=single step, 0x00=double step
    Track0S0AltEncoding  uint8     // 0x00=use alt encoding, 0xFF=use default
    Track0S0Encoding      uint8     // Alternate encoding for track 0 side 0
    Track0S1AltEncoding  uint8     // 0x00=use alt encoding, 0xFF=use default
    Track0S1Encoding      uint8     // Alternate encoding for track 0 side 1
}
```

### Header Field Details

- **HeaderSignature**:
  - `"HXCPICFE"` for HFEv1 and HFEv2
  - `"HXCHFEV3"` for HFEv3 (our implementation)

- **FormatRevision**:
  - `0` for HFEv1 and HFEv3
  - `1` for HFEv2

- **TrackEncoding**: Encoding mode for tracks
  - `0x00`: ISOIBM_MFM_ENCODING
  - `0x01`: AMIGA_MFM_ENCODING
  - `0x02`: ISOIBM_FM_ENCODING
  - `0x03`: EMU_FM_ENCODING
  - `0xFF`: UNKNOWN_ENCODING

- **FloppyInterfaceMode**: Interface configuration
  - `0x00`: IBMPC_DD_FLOPPYMODE
  - `0x01`: IBMPC_HD_FLOPPYMODE
  - `0x02`: ATARIST_DD_FLOPPYMODE
  - `0x03`: ATARIST_HD_FLOPPYMODE
  - `0x04`: AMIGA_DD_FLOPPYMODE
  - `0x05`: AMIGA_HD_FLOPPYMODE
  - `0x06`: CPC_DD_FLOPPYMODE
  - `0x07`: GENERIC_SHUGGART_DD_FLOPPYMODE
  - `0x08`: IBMPC_ED_FLOPPYMODE
  - `0x09`: MSX2_DD_FLOPPYMODE
  - `0x0A`: C64_DD_FLOPPYMODE
  - `0x0B`: EMU_SHUGART_FLOPPYMODE
  - `0x0C`: S950_DD_FLOPPYMODE
  - `0x0D`: S950_HD_FLOPPYMODE
  - `0xFE`: DISABLE_FLOPPYMODE

- **TrackListOffset**: Offset to track list in 512-byte blocks
  - Example: `1` = offset 0x200, `2` = offset 0x400

**Important Notes:**
- All `uint16_t` fields are in **little-endian** format (LSB first)
- Unused header bytes must be set to `0xFF`
- Header structure must be packed (no padding)

## Track List (LUT)

The track list is an array of track descriptors, each 4 bytes:

```go
type TrackHeader struct {
    Offset   uint16  // Track data offset in 512-byte blocks
    TrackLen uint16  // Length of track data in bytes
}
```

- For an 82-track disk, the table contains 82 entries
- Each entry is 4 bytes (2 bytes offset + 2 bytes length)
- All fields are little-endian

## Track Data Format

Track data contains the bitstream for both sides of a track, interleaved in 512-byte blocks.

### Block Structure

Each 512-byte block contains:
- **Bytes 0-255**: Side 0 track data
- **Bytes 256-511**: Side 1 track data

```
Block Structure:
┌─────────────┬─────────────┐
│ Side 0 Data │ Side 1 Data│
│  (256 bytes)│  (256 bytes)│
└─────────────┴─────────────┘
```

### Bit Order

**Critical:** HFE uses **LSB-first** bit order (bit 0 → bit 7), but our internal representation uses **MSB-first**. Conversion is required when reading/writing.

Transmission order to FDC:
```
Bit 0 → Bit 1 → Bit 2 → Bit 3 → Bit 4 → Bit 5 → Bit 6 → Bit 7 → (next byte)
```

### Track Length

- Track length is rounded up to 512-byte boundaries
- Track length represents the total size for both sides
- Actual bitstream length may be shorter (padding with 0xFF or repeated bits)

### Track Rotation

When reading:
- Tracks are rotated so the index pulse is at bit 0
- The `SETINDEX` opcode marks the index position
- Track data is rotated to align index at the start

When writing:
- Tracks are rotated so the gap is at the index position
- This ensures proper alignment when the track is read back

## HFEv3 Opcodes

HFEv3 supports opcodes embedded in the track bitstream to handle variable bitrates, weak bits, and timing compensation.

### Opcode Detection

Opcodes are identified by the pattern: `(byte & 0xF0) == 0xF0`

### Opcode List

| Opcode | Encoding | Description |
|--------|----------|-------------|
| **NOP** | `0xF0` | No operation - used for alignment |
| **SETINDEX** | `0xF1` | Mark index pulse position |
| **SETBITRATE** | `0xF2 0xBB` | Change bitrate to 18000000/0xBB (samples/sec) |
| **SKIPBITS** | `0xF3 0xLL` | Skip `LL` bits (0-7), then copy remaining bits from next byte |
| **RAND** | `0xF4` | Random/weak byte - skip 8 bits |
| **Reserved** | `0xFF` | Reserved for future use |

### Opcode Processing

When reading HFEv3 tracks:

1. **NOP (0xF0)**: Skip 8 bits, advance input pointer
2. **SETINDEX (0xF1)**: Mark current output position as index, skip 8 bits
3. **SETBITRATE (0xF2)**: Read next byte as new bitrate divisor value (18000000/x), skip 16 bits total
4. **SKIPBITS (0xF3)**:
   - Read next byte as skip count (0-7)
   - Skip `skip_count` bits from input
   - Copy remaining `8 - skip_count` bits to output
   - Total input consumed: 16 bits + skip_count
5. **RAND (0xF4)**: Skip 8 bits (represents weak/random bits)

### Bitrate Calculation

- Default bitrate is specified in the header (`bitRate` field in Kbit/s)
- Opcodes allow variable bitrate within a track
- Bitrate changes affect timing but not the bitstream data itself

## Implementation Details

### Bit Reversal

HFE stores data with LSB-first bit order, but we work with MSB-first internally. The `bitReverse()` function converts between formats:

```go
func bitReverse(b byte) byte {
    var result byte
    for i := 0; i < 8; i++ {
        result <<= 1
        result |= b & 1
        b >>= 1
    }
    return result
}
```

### Bit Copying

The `bitCopy()` function copies bits at arbitrary bit offsets:

```go
func bitCopy(dst []byte, dstOffset int, src []byte, srcOffset int, numBits int)
```

This is essential for:
- Processing SKIPBITS opcodes
- Rotating tracks to align index
- Handling bit-level operations

### Track Demuxing

When reading, track data is demuxed from interleaved blocks:

```go
// For each 512-byte block:
side0Data[blockOffset:blockOffset+256] = trackBlock[0:256]
side1Data[blockOffset:blockOffset+256] = trackBlock[256:512]
```

### Track Rotation

**Reading:**
1. Process opcodes to extract bitstream
2. Find index position (from SETINDEX opcode)
3. Rotate track so index is at bit 0:
   - Copy bits from index to end
   - Copy bits from start to index

**Writing:**
1. Rotate track so gap is at index position
2. Write bits in MSB-first order
3. Apply bit reversal before writing
4. Interleave side 0 and side 1 data

### Padding

- Tracks are padded to 512-byte boundaries
- When writing, if track is shorter than expected, last 16 bits may be repeated as gap
- Unused bytes in blocks are typically set to 0xFF

## Reading HFE Files

1. **Read Header**: Read 512 bytes, verify signature "HXCHFEV3"
2. **Read Track List**: Read from `track_list_offset * 512`, parse track headers
3. **For Each Track**:
   - Read track data (padded to 512-byte boundary)
   - Apply bit reversal to entire track buffer
   - Demux side 0 and side 1 data
   - If HFEv3: Process opcodes to extract bitstream
   - Rotate track to align index at bit 0

## Writing HFE Files

1. **Calculate Track Offsets**: Determine positions for all tracks
2. **Write Header**: Write 512-byte header with metadata
3. **Write Track List**: Write track offset table
4. **For Each Track**:
   - Rotate track so gap is at index
   - Write side 0 and side 1 data, interleaved in 512-byte blocks
   - Apply bit reversal before writing
   - Pad to 512-byte boundary

## Constants

```go
const (
    HFEv3Signature = "HXCHFEV3"

    OPCODE_MASK       = 0xF0
    NOP_OPCODE       = 0xF0
    SETINDEX_OPCODE  = 0xF1
    SETBITRATE_OPCODE = 0xF2
    SKIPBITS_OPCODE  = 0xF3
    RAND_OPCODE      = 0xF4

    FLOPPYEMUFREQ = 36000000  // Floppy emulator frequency (36 MHz)
    BlockSize = 512           // Block size in bytes
)
```

## Format Versions

- **HFEv1**: Original format, no opcodes
- **HFEv2**: Added 4-byte opcodes (experimental)
- **HFEv3**: Optimized opcodes (1-2 bytes), current standard

Our implementation supports **HFEv3 only**.

## Notes

- The bitstream content is specific to each targeted system and disk format
- Low-level floppy disk controller track and sector format descriptions are not covered in this document
- For more floppy disk information, see: https://hxc2001.com/download/datasheet/floppy/thirdparty/
- HFEv2 and HFEv3 are experimental formats designed for specific use cases (variable bitrate, weak bits)
- For conventional usage, HFEv1 is recommended

## References

- [HxC Floppy Emulator HFE File Format Specification](https://hxc2001.com/floppy_drive_emulator/HFE-file-format.html)
- HxCFloppyEmulator library source code
- libdisk HFE implementation (hfe.c)
