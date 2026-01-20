# ImageDisk (.IMD) File Format Specification

## Overview

ImageDisk is a program for reading and writing floppy disk images, created by Dave Dunfield. The .IMD format stores complete diskette images with detailed formatting information, allowing reconstruction of virtually any soft-sectored diskette format compatible with PC floppy disk controllers.

This document describes the technical details needed to implement reading and writing of .IMD format files, based on analysis of the ImageDisk reference implementation.

## File Structure

An IMD file consists of:

1. **Comment Block** (variable length, terminated by 0x1A)
2. **Track Records** (repeated until end of file)
   - Track Header
   - Sector Numbering Map
   - Optional Cylinder Map
   - Optional Head Map
   - Sector Data Blocks

## Comment Block

The comment block is ASCII text that describes the disk image. It is terminated by a single byte with value `0x1A` (EOF marker, Ctrl+Z).

**Format:**
- ASCII text lines (typically terminated by CRLF: `\r\n`)
- Each line can contain any printable ASCII characters
- The comment block ends with byte `0x1A`
- The comment is optional but typically present

**Example:**
```
IMD 1.18: 01/15/2024 10:30:45
Created by ImageDisk
\r\n
0x1A
```

## Track Record Structure

Each track on the disk is represented by a track record. Track records continue until end of file (EOF).

### Track Header

The track header consists of 5 bytes:

| Offset | Size | Field | Description |
|--------|------|-------|-------------|
| 0 | 1 | Mode | Data rate and encoding method (see Mode Encoding) |
| 1 | 1 | Cylinder | Physical cylinder number (0-based) |
| 2 | 1 | Head | Physical head/side number with flags (see Head Flags) |
| 3 | 1 | Nsec | Number of sectors in this track |
| 4 | 1 | Ssize | Encoded sector size (see Sector Size Encoding) |

#### Mode Encoding

The Mode byte encodes both data rate and encoding method:

| Mode | Data Rate | Encoding | Description |
|------|-----------|----------|-------------|
| 0 | 500 kbps | FM | Single Density, 500 kbps |
| 1 | 300 kbps | FM | Single Density, 300 kbps |
| 2 | 250 kbps | FM | Single Density, 250 kbps |
| 3 | 500 kbps | MFM | Double Density, 500 kbps |
| 4 | 300 kbps | MFM | Double Density, 300 kbps |
| 5 | 250 kbps | MFM | Double Density, 250 kbps |

**Note:** Mode values 0-2 are FM (Frequency Modulation), 3-5 are MFM (Modified Frequency Modulation).

#### Head Flags

The Head byte contains the physical head number in the lower bits and flags in the upper bits:

- **Bits 0-3**: Physical head number (typically 0 or 1)
- **Bit 6 (0x40)**: If set, a Head Map follows the Sector Numbering Map
- **Bit 7 (0x80)**: If set, a Cylinder Map follows the Sector Numbering Map

**Examples:**
- `0x00` = Head 0, no maps
- `0x01` = Head 1, no maps
- `0x40` = Head 0, Head Map present
- `0x80` = Head 0, Cylinder Map present
- `0xC0` = Head 0, both maps present

#### Sector Size Encoding

The Ssize byte encodes the sector size using a power-of-two encoding:

| Ssize | Actual Size | Description |
|-------|-------------|-------------|
| 0 | 128 bytes | 128 bytes per sector |
| 1 | 256 bytes | 256 bytes per sector |
| 2 | 512 bytes | 512 bytes per sector |
| 3 | 1024 bytes | 1 KB per sector |
| 4 | 2048 bytes | 2 KB per sector |
| 5 | 4096 bytes | 4 KB per sector |
| 6 | 8192 bytes | 8 KB per sector |

**Formula:** Actual sector size = `128 << Ssize`

### Sector Numbering Map

Immediately following the track header is the Sector Numbering Map, which contains `Nsec` bytes. Each byte represents the logical sector number as it appears in the sector ID field on the disk.

**Purpose:** This map records the actual sector numbering sequence on the track, which may not be sequential (due to interleaving) and may not start at 0 or 1.

**Example:** For a track with 9 sectors, the map might be:
```
[1, 4, 7, 2, 5, 8, 3, 6, 9]
```
This indicates an interleave factor of 3:1.

### Cylinder Map (Optional)

If bit 7 of the Head byte is set (0x80), a Cylinder Map follows the Sector Numbering Map. It contains `Nsec` bytes, where each byte is the logical cylinder number from the sector ID field.

**Purpose:** Some disk formats encode non-physical cylinder numbers in the sector ID fields. This map records those logical cylinder numbers.

**Default:** If the Cylinder Map is not present, all sectors are assumed to have the same cylinder number as specified in the track header.

### Head Map (Optional)

If bit 6 of the Head byte is set (0x40), a Head Map follows (after the Cylinder Map if present). It contains `Nsec` bytes, where each byte is the logical head number from the sector ID field.

**Purpose:** Some disk formats encode non-physical head numbers in the sector ID fields. This map records those logical head numbers.

**Default:** If the Head Map is not present, all sectors are assumed to have the same head number as specified in the track header.

## Sector Data Blocks

After the maps, there are `Nsec` sector data blocks, one for each sector in the track. The order of these blocks corresponds to the order of entries in the Sector Numbering Map.

### Sector Data Block Format

Each sector data block begins with a single flag byte:

| Flag Value | Description |
|------------|-------------|
| 0x00 | No data available (sector was unreadable or skipped) |
| 0x01-0xFF | Data present (see Flag Byte Encoding below) |

#### Flag Byte Encoding

When data is present (flag != 0), the flag byte encodes compression and status:

- **Bit 0 (0x01)**: Compressed sector - all bytes have the same value
- **Bit 1 (0x02)**: Deleted Data Address Mark (DAM)
- **Bit 2 (0x04)**: Bad sector (CRC error or other read error)

**Flag value calculation:**
- Base value: 1 (data present)
- Add 1 if compressed
- Add 2 if deleted address mark
- Add 4 if bad sector

**Examples:**
- `0x01` = Normal data, not compressed
- `0x02` = Compressed data (all bytes same)
- `0x03` = Normal data with deleted address mark
- `0x05` = Normal data, bad sector
- `0x07` = Compressed data, deleted address mark, bad sector

### Compressed Sector Data

If bit 0 of the flag byte is set (compressed), the sector data consists of a single byte value. This byte is repeated to fill the entire sector size.

**Format:**
```
[Flag Byte] [Single Byte Value]
```

**Example:** For a 512-byte sector with flag `0x02` (compressed) and value `0xE5`:
- The sector contains 512 bytes, all with value `0xE5`

### Uncompressed Sector Data

If bit 0 of the flag byte is clear (not compressed), the sector data consists of the full sector data bytes.

**Format:**
```
[Flag Byte] [Sector Data Bytes...]
```

The number of data bytes equals the sector size (128 << Ssize).

**Example:** For a 512-byte sector with flag `0x01` (normal, not compressed):
- Followed by 512 bytes of actual sector data

## End of File

The file ends when there are no more track records to read (EOF is reached). There is no explicit end marker for track records.

## Null Track Record

A track with no sectors (Nsec = 0) represents a null track. In this case:
- Mode, Cylinder, Head, and Ssize are still present
- No Sector Numbering Map
- No optional maps
- No sector data blocks

**Format:**
```
[Mode] [Cylinder] [Head] [0x00] [Ssize]
```

## Implementation Notes

### Reading IMD Files

1. Read and parse the comment block until `0x1A` is encountered
2. For each track record:
   - Read the 5-byte track header
   - Validate Mode (0-5), Head (bits 0-3 should be 0-1), Ssize (0-6)
   - Read Sector Numbering Map (Nsec bytes)
   - If Head & 0x80, read Cylinder Map (Nsec bytes)
   - If Head & 0x40, read Head Map (Nsec bytes)
   - For each of Nsec sectors:
     - Read flag byte
     - If flag == 0, skip (no data)
     - If flag & 0x01, read 1 byte and expand to sector size
     - Otherwise, read full sector size bytes
3. Continue until EOF

### Writing IMD Files

1. Write comment block (ASCII text) terminated by `0x1A`
2. For each track:
   - Determine Mode from data rate and encoding
   - Write track header (5 bytes)
   - Write Sector Numbering Map
   - If cylinder numbers vary, set Head bit 7 and write Cylinder Map
   - If head numbers vary, set Head bit 6 and write Head Map
   - For each sector:
     - Calculate flag byte from sector status
     - If all bytes are the same, set compression bit and write single byte
     - Otherwise, write full sector data
3. Close file

### Compression Detection

To detect if a sector can be compressed, check if all bytes in the sector have the same value:

```go
func isCompressible(data []byte) bool {
    if len(data) == 0 {
        return false
    }
    first := data[0]
    for i := 1; i < len(data); i++ {
        if data[i] != first {
            return false
        }
    }
    return true
}
```

### Sector Size Calculation

```go
func sectorSize(ssize byte) int {
    return 128 << ssize
}
```

### Mode to Data Rate/Density

```go
func modeToRateDensity(mode byte) (rate int, mfm bool) {
    rateTable := []int{500, 300, 250, 500, 300, 250}
    if mode > 5 {
        return 0, false // Invalid
    }
    return rateTable[mode], mode >= 3
}
```

## Example File Structure

```
[Comment: "IMD 1.18: 01/15/2024 10:30:45\r\nCreated by ImageDisk\r\n"]
0x1A
[Track 0, Head 0: Mode=5, Cyl=0, Head=0, Nsec=18, Ssize=2]
[Sector Map: 18 bytes]
[18 Sector Data Blocks]
[Track 0, Head 1: Mode=5, Cyl=0, Head=1, Nsec=18, Ssize=2]
[Sector Map: 18 bytes]
[18 Sector Data Blocks]
[Track 1, Head 0: ...]
...
[EOF]
```

## References

- ImageDisk source code by Dave Dunfield
- ImageDisk utilities: imd.c, imda.c, imdu.c, imdv.c, td02imd.c
- ImageDisk documentation and specification

## Version History

This specification is based on ImageDisk version 1.18 and later versions. The format has remained stable across versions.
