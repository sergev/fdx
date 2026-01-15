package mfm

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
func (w *Writer) writeMarker() {
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
func (w *Writer) EncodeTrackIBMPC(sectors [][]byte, cylinder, head, sectorsPerTrack int) []byte {

	//TODO: compute gaps based on bit rate and sectorsPerTrack.
	indexGap := 50   // empty bytes before first sector
	headerGap := 22  // empty bytes after sector header before sector data
	sectorGap := 108 // empty bytes between sectors

	// Index (before first sector)
	w.writeGap(80)
	w.writeIndexMarker()
	w.writeGap(indexGap)

	// Write each sector
	for s := 0; s < sectorsPerTrack; s++ {
		// Sector marker (A1 sync)
		w.writeMarker()

		// Sector header tag
		w.writeByte(0xFE)

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

		// Data marker (A1 sync)
		w.writeMarker()

		// Data tag
		w.writeByte(0xFB)

		// Sector data should be present
		sectorData := sectors[s]
		if sectorData != nil {
			for _, b := range sectorData {
				w.writeByte(b)
			}
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
