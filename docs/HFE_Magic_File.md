# HFE Magic File Instructions

This document explains how to add HFE (HxC Floppy Emulator) file format recognition to the `file` command using a magic file entry.

## Overview

The magic file (`hfe.magic`) allows the `file` command to recognize HFE files by their header signature and display useful metadata:
- Number of tracks
- Number of sides
- Bit rate (in kB/s)
- Rotation speed (RPM)

## HFE File Format

HFE files have a 512-byte header with the following structure at the beginning:

- **Offset 0-7**: Header signature (8 bytes)
  - `HXCHFEV3` for HFE v3 format
  - `HXCPICFE` for HFE v1/v2 format
- **Offset 9**: Number of tracks (1 byte)
- **Offset 10**: Number of sides (1 byte)
- **Offset 12-13**: Bit rate (2 bytes, little-endian, in kB/s)
- **Offset 14-15**: Floppy RPM (2 bytes, little-endian)

## Installation

### Option 1: System-wide Installation (requires root)

1. Copy the magic file to the system magic directory:
   ```bash
   sudo cp hfe/hfe.magic /usr/share/file/magic/
   ```

2. Rebuild the compiled magic database:
   ```bash
   sudo file -C
   ```

3. Test the installation:
   ```bash
   file testdata/fat12v3.hfe
   ```

   Expected output:
   ```
   testdata/fat12v3.hfe: HFE floppy disk image, 2 tracks, 2 sides, 500 kB/s bit rate, 0 RPM
   ```

### Option 2: User-specific Installation

1. Create a local magic directory (if it doesn't exist):
   ```bash
   mkdir -p ~/.magic
   ```

2. Copy the magic file:
   ```bash
   cp hfe/hfe.magic ~/.magic/
   ```

3. Use the magic file with the `-m` flag:
   ```bash
   file -m ~/.magic/hfe.magic testdata/fat12v3.hfe
   ```

   Or add it to your shell configuration (e.g., `~/.bashrc` or `~/.zshrc`):
   ```bash
   alias file='file -m ~/.magic/hfe.magic'
   ```

### Option 3: Temporary Usage

You can use the magic file directly without installation:

```bash
file -m hfe/hfe.magic testdata/fat12v3.hfe
```

## Usage

Once installed, the `file` command will automatically recognize HFE files:

```bash
file image.hfe
```

Example output:
```
image.hfe: HFE floppy disk image, 80 tracks, 2 sides, 250 kB/s bit rate, 300 RPM
```

## Magic File Format

The magic file uses the following format (see `man 5 magic` for details):

- **Line 1**: Checks for the header signature at offset 0
- **Continuation lines** (starting with `>`): Extract additional values from the header
  - `>9 byte x` - Extracts number of tracks at offset 9
  - `>10 byte x` - Extracts number of sides at offset 10
  - `>12 leshort x` - Extracts bit rate at offset 12 (little-endian 16-bit)
  - `>14 leshort x` - Extracts RPM at offset 14 (little-endian 16-bit)

The `\b` escape sequence removes the preceding space, allowing multiple continuation lines to be concatenated into a single output message.

## Supported Formats

The magic file recognizes both HFE format versions:
- **HFE v1**: Signature `HXCPICFE`
- **HFE v3**: Signature `HXCHFEV3`

### Rebuilding the magic database

If you've modified the magic file and it's installed system-wide, rebuild the database:

```bash
sudo file -C
```

## References

- `man 5 magic` - Magic file format documentation
- `man 1 file` - File command documentation
- [HFE File Format Specification](HFE_File_Format.md) - Detailed HFE format documentation
