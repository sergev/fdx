package mfm

import "fmt"

// Write MFM-encoded bits to a buffer
type Writer struct {
	buffer      []byte // Output buffer
	bitPos      int    // Current bit position (0-based)
	lastDataBit int    // Last data bit for encoding of next zero
	maxHalfBits int    // Maximum number of half-bits allowed for this track
}

// Create a new MFM writer.
func NewWriter(maxHalfBits int) *Writer {
	return &Writer{
		buffer:      make([]byte, 0, 1024),
		bitPos:      0,
		lastDataBit: 0,
		maxHalfBits: maxHalfBits,
	}
}

// Write a "half" bit, which means one MFM bit
func (w *Writer) writeHalfBit(bitValue int) {
	if w.bitPos >= w.maxHalfBits {
		// The track has ended.
		return
	}

	// Ensure we have space for at least one more byte.
	neededBytes := (w.bitPos + 7) / 8
	if neededBytes >= len(w.buffer) {
		w.buffer = append(w.buffer, 0)
	}

	// Write MFM bit
	if bitValue != 0 {
		byteIdx := w.bitPos / 8
		bitIdx := 7 - (w.bitPos % 8)
		w.buffer[byteIdx] |= 1 << bitIdx
	}
	w.bitPos++
}

// Write one data bit, which means two MFM bits.
func (w *Writer) writeBit(dataBit int) {
	if dataBit != 0 {
		// Encoding a one.
		w.writeHalfBit(0)
		w.writeHalfBit(1)
	} else {
		// Encoding a zero.
		w.writeHalfBit(w.lastDataBit ^ 1)
		w.writeHalfBit(0)
	}
	w.lastDataBit = dataBit
}

// Write a data byte, encoding it as MFM (16 bits = 2 bytes)
func (w *Writer) writeByte(data byte) {
	// Encode each bit of the data byte
	for i := 7; i >= 0; i-- {
		dataBit := int((data >> i) & 1)
		w.writeBit(dataBit)
	}
}

// Write n bytes of gap
func (w *Writer) writeGap(n int) {
	for i := 0; i < n; i++ {
		w.writeByte(0x4E) // standard gap byte
	}
}

// Write the A1 sync marker (12 bytes of 0x00 + 3 bytes of A1 with MFM violation)
// A1 = 0xA1 = 10100001, but with MFM violations in bits 2 and 1 (half-bits)
func (w *Writer) writeMarker(tag uint8) {
	// Twelve bytes of zeros (normal MFM encoding)
	for i := 0; i < 12; i++ {
		w.writeByte(0)
	}

	// Three bytes of A1 violating encoding in the sixth bit (bit 2 from MSB)
	// Pattern from C code: 1, 0, 1, 0, 0, [half-bit], [half-bit], 0, 1
	// This encodes A1 (10100001) but with violations
	for i := 0; i < 3; i++ {
		w.writeBit(1)     // data bit 7
		w.writeBit(0)     // data bit 6
		w.writeBit(1)     // data bit 5
		w.writeBit(0)     // data bit 4
		w.writeBit(0)     // data bit 3
		w.writeHalfBit(0) // data bit 2 (half-bit violation)
		w.writeHalfBit(0) // data bit 1 (half-bit violation)
		w.writeBit(0)     // data bit 0
		w.writeBit(1)     // This completes the A1 pattern (10100001)
	}
	w.writeByte(tag)
}

// Write the index marker (C2 sync)
// C2 = 0xC2 = 11000010, but with MFM violations in bits 2 and 1 (half-bits)
func (w *Writer) writeIndexMarker() {
	// Twelve bytes of zeros (normal MFM encoding)
	for i := 0; i < 12; i++ {
		w.writeByte(0)
	}

	// Three bytes of C2 violating encoding in the sixth bit (bit 2 from MSB)
	// Pattern from C code: 1, 1, 0, 0, 0, [half-bit], [half-bit], 1, 0
	// This encodes C2 (11000010) but with violations
	for i := 0; i < 3; i++ {
		w.writeBit(1)     // data bit 7
		w.writeBit(1)     // data bit 6
		w.writeBit(0)     // data bit 5
		w.writeBit(0)     // data bit 4
		w.writeBit(0)     // data bit 3
		w.writeHalfBit(0) // data bit 2 (half-bit violation)
		w.writeHalfBit(0) // data bit 1 (half-bit violation)
		w.writeBit(1)     // data bit 0
		w.writeBit(0)     // This completes the C2 pattern (11000010)
	}
	w.writeByte(0xFC)
}

// Return the MFM-encoded buffer
func (w *Writer) getData() []byte {
	// Trim to actual size used
	actualBytes := (w.bitPos + 7) / 8
	if actualBytes < len(w.buffer) {
		return w.buffer[:actualBytes]
	}
	return w.buffer
}

// Encode a track in IBM PC format
// sectors: array of sector data (512 bytes each), indexed by sector number
// cylinder: cylinder number (0-based)
// head: head number (0 or 1)
// sectorsPerTrack: number of sectors per track
//
// Track layout for IBM PC floppies
// ┌─────┬──────┬────┬···┬──────┬──────┬────┬──────┬────┬────┬···┬─────┐
// │gap4a│Index │gap1│   │Sector│Sector│gap2│Data  │Data│gap3│   │gap4b│
// │(80) │Marker│(50)│   │Marker│Header│(22)│Marker│+CRC│    │   │     │
// └─────┴──────┴────┴···┴──────┴──────┴────┴──────┴────┴────┴···┴─────┘
//                     └───────────────repeat──────────────────┘
func (w *Writer) EncodeTrackIBMPC(sectors [][]byte, cylinder, head, sectorsPerTrack int, bitRate uint16) []byte {

	const startGap = 80 // gap4a: empty bytes before index marker
	const indexGap = 50 // gap1: empty bytes before first sector

	// Compute gap2 and gap3 based on bit rate and sectorsPerTrack.
	headerGap, sectorGap := computeGapsIBMPC(bitRate, sectorsPerTrack)

	// Index (before first sector)
	w.writeGap(startGap)
	w.writeIndexMarker()
	w.writeGap(indexGap)

	// Write each sector
	for s := 0; s < sectorsPerTrack; s++ {

		// Sector marker
		w.writeMarker(0xFE)

		// Sector identifier: cylinder, head, sector, size
		w.writeByte(byte(cylinder))
		w.writeByte(byte(head))
		w.writeByte(byte(s + 1)) // Sector number (1-based)
		w.writeByte(2)           // Size code (2 = 512 bytes)

		// Calculate header CRC
		sum := crc16CCITTByte(0xb230, byte(cylinder))
		sum = crc16CCITTByte(sum, byte(head))
		sum = crc16CCITTByte(sum, byte(s+1))
		sum = crc16CCITTByte(sum, 2)

		// Write header CRC
		w.writeByte(byte(sum >> 8))
		w.writeByte(byte(sum))

		// Gap between sector mark and data
		w.writeGap(headerGap)

		// Data marker
		w.writeMarker(0xFB)

		// Sector data must be present
		sectorData := sectors[s]
		for _, b := range sectorData {
			w.writeByte(b)
		}

		// Calculate data CRC
		sum = crc16CCITTByte(0xcdb4, 0xFB)
		sum = crc16CCITT(sum, sectorData)

		// Write data CRC
		w.writeByte(byte(sum >> 8))
		w.writeByte(byte(sum))

		// Gap between sectors
		w.writeGap(sectorGap)
	}

	// Fill remaining track
	fillGap := w.maxHalfBits/8 - len(w.getData())
	if fillGap > 0 {
		w.writeGap(fillGap)
	}
	return w.getData()
}

// Compute gap2 and gap3 based on bit rate and number of sectors per track.
//
//             Floppy  Media   Sectors
// Bit rate    Drive   Volume  per track  Heads  Tracks  gap2  gap3
// ----------------------------------------------------------------
// 500 kbps    5¼"AT   1.2M    15         2      80      22    84
//             3½"     1.44M   18         2      80      22    108
//             3½"     1.6M    20         2      80      22    44
// ----------------------------------------------------------------
// 250 kbps    5¼"SS   160K    8          1      40      22    80
//             5¼"PC   320K    8          2      40      22    80
//             5¼"SS   180K    9          1      40      22    80
//             5¼"PC   360K    9          2      40      22    80
//             3½"SS   360K    9          1      80      22    80
//             3½"     720K    9          2      80      22    80
//             3½"     800K    10         2      80      22    34
// ----------------------------------------------------------------
// 300 kbps    5¼"AT   360K    9          2      40      22    80
// ----------------------------------------------------------------
// 1000 kbps   3½"     2.88M   36         2      80      41    84
//             3½"     3.12M   39         2      80      41    40
func computeGapsIBMPC(bitRate uint16, sectorsPerTrack int) (int, int) {

	// gap2: empty bytes after sector header before sector data
	headerGap := 22
	if bitRate > 500 {
		// 2.88M floppies need more time for magnetic head to switch
		headerGap = 41
	}

	// gap3: empty bytes between sectors
	sectorGap := 80
	switch bitRate {
	case 500:
		sectorGap = 108
		if sectorsPerTrack < 18 {
			sectorGap = 84
		}
		if sectorsPerTrack > 18 {
			sectorGap = 44
		}
	case 1000:
		sectorGap = 84
		if sectorsPerTrack > 36 {
			sectorGap = 40
		}
	case 250, 300:
		sectorGap = 80
		if sectorsPerTrack > 9 {
			// Recommended gap3 value for 800K format is 46, but
			// it seems unstable: last sector sometimes not found.
			// Greaseweazle uses 30, which looks too low.
			// From my experience, 34 works well.
			sectorGap = 34
		}
	}
	return headerGap, sectorGap
}

// shuffle splits a 32-bit word into odd and even bit streams.
func shuffle(word uint32) (odd, even uint16) {
	for i := 0; i < 16; i++ {
		odd <<= 1
		even <<= 1
		odd |= uint16((word >> 31) & 1)
		even |= uint16((word >> 30) & 1)
		word <<= 2
	}
	return odd, even
}

// writeMarkerAmiga writes the Amiga sector marker (00-a1-a1-fx pattern).
func (w *Writer) writeMarkerAmiga() {
	// Two bytes of zeros
	w.writeByte(0)
	w.writeByte(0)

	// Two bytes of A1 violating encoding in the sixth bit
	// Pattern: 1, 0, 1, 0, 0, [half-bit], [half-bit], 0, 1
	for i := 0; i < 2; i++ {
		w.writeBit(1)     // data bit 7
		w.writeBit(0)     // data bit 6
		w.writeBit(1)     // data bit 5
		w.writeBit(0)     // data bit 4
		w.writeBit(0)     // data bit 3
		w.writeHalfBit(0) // data bit 2 (half-bit violation)
		w.writeHalfBit(0) // data bit 1 (half-bit violation)
		w.writeBit(0)     // data bit 0
		w.writeBit(1)     // This completes the A1 pattern (10100001)
	}
}

// writeIdentAmiga writes the sector identifier and its checksum.
func (w *Writer) writeIdentAmiga(track, sector int) {
	// Compute identifier: 0xff << 24 | track << 16 | sector << 8 | (11 - sector)
	ldata := uint32(0xff)<<24 | uint32(track)<<16 | uint32(sector)<<8 | uint32(11-sector)

	// Shuffle and compute checksum
	odd, even := shuffle(ldata)
	sum := uint32(odd) ^ uint32(even)

	// Write identifier (shuffled)
	w.writeByte(byte(odd >> 8))
	w.writeByte(byte(odd))
	w.writeByte(byte(even >> 8))
	w.writeByte(byte(even))

	// Write label (4 longs, all zeros)
	for i := 0; i < 16; i++ {
		w.writeByte(0)
	}

	// Write checksum
	w.writeByte(byte(sum >> 24))
	w.writeByte(byte(sum >> 16))
	w.writeByte(byte(sum >> 8))
	w.writeByte(byte(sum))
}

// writeSectorAmiga writes a 512-byte block with bit shuffling.
// Before the block, 4 bytes of checksum are written.
func (w *Writer) writeSectorAmiga(data []byte) {
	if len(data) != sectorSize {
		panic(fmt.Sprintf("data buffer must be %d bytes", sectorSize))
	}

	// Shuffle data and compute checksum
	odd := make([]uint16, sectorSize/4)
	even := make([]uint16, sectorSize/4)
	var sum uint32

	for i := 0; i < sectorSize/4; i++ {
		ldata := uint32(data[4*i])<<24 | uint32(data[4*i+1])<<16 | uint32(data[4*i+2])<<8 | uint32(data[4*i+3])
		odd[i], even[i] = shuffle(ldata)
		sum ^= uint32(odd[i]) ^ uint32(even[i])
	}

	// Write checksum
	w.writeByte(byte(sum >> 24))
	w.writeByte(byte(sum >> 16))
	w.writeByte(byte(sum >> 8))
	w.writeByte(byte(sum))

	// Write data, odd bits first
	for i := 0; i < sectorSize/4; i++ {
		w.writeByte(byte(odd[i] >> 8))
		w.writeByte(byte(odd[i]))
	}
	// Then even bits
	for i := 0; i < sectorSize/4; i++ {
		w.writeByte(byte(even[i] >> 8))
		w.writeByte(byte(even[i]))
	}
}

// EncodeTrackAmiga encodes a track in Amiga format.
// sectors: array of sector data (512 bytes each), indexed by sector number (0-10)
// track: track number (0-based, where track = cylinder*2 + head)
func (w *Writer) EncodeTrackAmiga(sectors [][]byte, track int) []byte {
	const gapSize = 150 // Gap before first sector

	// Write gap
	w.writeGap(gapSize)

	// Write each sector
	for s := 0; s < 11; s++ {
		w.writeMarkerAmiga()
		w.writeIdentAmiga(track, s)
		w.writeSectorAmiga(sectors[s])
	}

	// Fill remaining track
	fillGap := w.maxHalfBits/8 - len(w.getData())
	if fillGap > 0 {
		w.writeGap(fillGap)
	}

	return w.getData()
}
