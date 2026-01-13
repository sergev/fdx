package hfe

import (
	"fmt"
	"os"
)

// CRC16-CCITT lookup table (CRC-CCITT = x^16 + x^12 + x^5 + 1)
// From ibmpc.c lines 15-38
var crc16PolyTab = [256]uint16{
	0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50a5, 0x60c6, 0x70e7, 0x8108, 0x9129, 0xa14a, 0xb16b,
	0xc18c, 0xd1ad, 0xe1ce, 0xf1ef, 0x1231, 0x0210, 0x3273, 0x2252, 0x52b5, 0x4294, 0x72f7, 0x62d6,
	0x9339, 0x8318, 0xb37b, 0xa35a, 0xd3bd, 0xc39c, 0xf3ff, 0xe3de, 0x2462, 0x3443, 0x0420, 0x1401,
	0x64e6, 0x74c7, 0x44a4, 0x5485, 0xa56a, 0xb54b, 0x8528, 0x9509, 0xe5ee, 0xf5cf, 0xc5ac, 0xd58d,
	0x3653, 0x2672, 0x1611, 0x0630, 0x76d7, 0x66f6, 0x5695, 0x46b4, 0xb75b, 0xa77a, 0x9719, 0x8738,
	0xf7df, 0xe7fe, 0xd79d, 0xc7bc, 0x48c4, 0x58e5, 0x6886, 0x78a7, 0x0840, 0x1861, 0x2802, 0x3823,
	0xc9cc, 0xd9ed, 0xe98e, 0xf9af, 0x8948, 0x9969, 0xa90a, 0xb92b, 0x5af5, 0x4ad4, 0x7ab7, 0x6a96,
	0x1a71, 0x0a50, 0x3a33, 0x2a12, 0xdbfd, 0xcbdc, 0xfbbf, 0xeb9e, 0x9b79, 0x8b58, 0xbb3b, 0xab1a,
	0x6ca6, 0x7c87, 0x4ce4, 0x5cc5, 0x2c22, 0x3c03, 0x0c60, 0x1c41, 0xedae, 0xfd8f, 0xcdec, 0xddcd,
	0xad2a, 0xbd0b, 0x8d68, 0x9d49, 0x7e97, 0x6eb6, 0x5ed5, 0x4ef4, 0x3e13, 0x2e32, 0x1e51, 0x0e70,
	0xff9f, 0xefbe, 0xdfdd, 0xcffc, 0xbf1b, 0xaf3a, 0x9f59, 0x8f78, 0x9188, 0x81a9, 0xb1ca, 0xa1eb,
	0xd10c, 0xc12d, 0xf14e, 0xe16f, 0x1080, 0x00a1, 0x30c2, 0x20e3, 0x5004, 0x4025, 0x7046, 0x6067,
	0x83b9, 0x9398, 0xa3fb, 0xb3da, 0xc33d, 0xd31c, 0xe37f, 0xf35e, 0x02b1, 0x1290, 0x22f3, 0x32d2,
	0x4235, 0x5214, 0x6277, 0x7256, 0xb5ea, 0xa5cb, 0x95a8, 0x8589, 0xf56e, 0xe54f, 0xd52c, 0xc50d,
	0x34e2, 0x24c3, 0x14a0, 0x0481, 0x7466, 0x6447, 0x5424, 0x4405, 0xa7db, 0xb7fa, 0x8799, 0x97b8,
	0xe75f, 0xf77e, 0xc71d, 0xd73c, 0x26d3, 0x36f2, 0x0691, 0x16b0, 0x6657, 0x7676, 0x4615, 0x5634,
	0xd94c, 0xc96d, 0xf90e, 0xe92f, 0x99c8, 0x89e9, 0xb98a, 0xa9ab, 0x5844, 0x4865, 0x7806, 0x6827,
	0x18c0, 0x08e1, 0x3882, 0x28a3, 0xcb7d, 0xdb5c, 0xeb3f, 0xfb1e, 0x8bf9, 0x9bd8, 0xabbb, 0xbb9a,
	0x4a75, 0x5a54, 0x6a37, 0x7a16, 0x0af1, 0x1ad0, 0x2ab3, 0x3a92, 0xfd2e, 0xed0f, 0xdd6c, 0xcd4d,
	0xbdaa, 0xad8b, 0x9de8, 0x8dc9, 0x7c26, 0x6c07, 0x5c64, 0x4c45, 0x3ca2, 0x2c83, 0x1ce0, 0x0cc1,
	0xef1f, 0xff3e, 0xcf5d, 0xdf7c, 0xaf9b, 0xbfba, 0x8fd9, 0x9ff8, 0x6e17, 0x7e36, 0x4e55, 0x5e74,
	0x2e93, 0x3eb2, 0x0ed1, 0x1ef0,
}

// crc16CCITT calculates CRC16-CCITT over data
// From ibmpc.c lines 45-51
func crc16CCITT(sum uint16, data []byte) uint16 {
	for _, b := range data {
		sum = (sum << 8) ^ crc16PolyTab[byte(b)^byte(sum>>8)]
	}
	return sum
}

// crc16CCITTByte calculates CRC16-CCITT for a single byte
// From ibmpc.c lines 53-57
func crc16CCITTByte(sum uint16, b byte) uint16 {
	return (sum << 8) ^ crc16PolyTab[b^byte(sum>>8)]
}

// mfmReader reads bits from an MFM bitstream (MSB-first byte order)
// The HFE format stores the raw MFM-encoded bitstream where clock and data bits are interleaved.
// In MFM encoding: each data bit is encoded as 2 bits (clock bit, data bit).
// The stream is: C0, D0, C1, D1, C2, D2, ... where C=clock, D=data
// We need to extract only the data bits (at odd positions: 1, 3, 5, 7, ...)
type mfmReader struct {
	data   []byte // MFM bitstream data (clock+data interleaved)
	bitPos int    // Current bit position in raw bitstream (0-based)
}

// newMFMReader creates a new MFM bitstream reader
func newMFMReader(data []byte) *mfmReader {
	return &mfmReader{
		data:   data,
		bitPos: 0, // Start at clock bit; readBit will advance two half-bits
	}
}

// Read "half" bit, which means a raw next bit from MFM stream.
func (r *mfmReader) readHalfBit() (int, error) {
	if r.bitPos >= len(r.data)*8 {
		return -1, fmt.Errorf("end of bitstream")
	}
	byteIdx := r.bitPos / 8
	bitIdx := 7 - (r.bitPos & 7) // MSB-first
	bit := (r.data[byteIdx] >> bitIdx) & 1
	r.bitPos++
	return int(bit), nil
}

// Read a single DATA bit from the MFM bitstream.
// Returns 0, 1, or error if end of stream.
func (r *mfmReader) readBit() (int, error) {
	// Ignore the first half-bit (clock)
	_, err := r.readHalfBit()
	if err != nil {
		return -1, err
	}

	// Return the second half-bit (data bit)
	bit, err := r.readHalfBit()
	if err != nil {
		return -1, err
	}
	return bit, nil
}

// readByte reads 8 bits and returns them as a byte
func (r *mfmReader) readByte() (byte, error) {
	var result byte
	for i := 0; i < 8; i++ {
		bit, err := r.readBit()
		if err != nil {
			return 0, err
		}
		result = (result << 1) | byte(bit)
	}
	return result, nil
}

// scanIBMPC scans for IBM PC sector markers
// From ibmpc.c lines 80-118
// Returns the tag byte after the marker, or error
func (r *mfmReader) scanIBMPC() (int, error) {
	history := uint32(0x13713713)

	for {
		bit, err := r.readBit()
		if err != nil {
			return -1, err
		}

		history = (history << 1) | uint32(bit)
		history &= 0xffffffff

		// All ones - synchronize to half-bit
		if history == 0xffffffff {
			if _, err := r.readHalfBit(); err != nil {
				return -1, err
			}
			history = 0
			continue
		}

		// IBM PC format: waiting for 00-a1-a1-a1 or 00-c2-c2-c2
		if history == 0x00a1a1a1 || history == 0x00c2c2c2 {
			// Found marker, read and return its tag
			tag, err := r.readByte()
			if err != nil {
				return -1, err
			}
			return int(tag), nil
		}
	}
}

// readSectorIBMPC reads a sector from IBM PC format
// From ibmpc.c lines 123-202
// Returns: sector number (0-based), 512-byte data, error
func (r *mfmReader) readSectorIBMPC(cylinder, head int) (int, []byte, error) {
	const sectorSize = 512
	data := make([]byte, sectorSize)

	for {
		// Scan for sector header marker (tag 0xFE)
		tag, err := r.scanIBMPC()
		if err != nil {
			return -1, nil, err
		}
		if tag != 0xfe {
			// Not a sector header, continue scanning
			continue
		}

		// Read sector header
		readCylinder, err := r.readByte()
		if err != nil {
			continue
		}
		readHead, err := r.readByte()
		if err != nil {
			continue
		}
		sector, err := r.readByte()
		if err != nil {
			continue
		}
		size, err := r.readByte()
		if err != nil {
			continue
		}
		headerSumHigh, err := r.readByte()
		if err != nil {
			continue
		}
		headerSumLow, err := r.readByte()
		if err != nil {
			continue
		}
		headerSum := uint16(headerSumHigh)<<8 | uint16(headerSumLow)

		// Verify header CRC
		myHeaderSum := crc16CCITTByte(0xb230, readCylinder)
		myHeaderSum = crc16CCITTByte(myHeaderSum, readHead)
		myHeaderSum = crc16CCITTByte(myHeaderSum, sector)
		myHeaderSum = crc16CCITTByte(myHeaderSum, size)
		if myHeaderSum != headerSum {
			// CRC mismatch, but continue searching
			continue
		}

		// Verify cylinder and head match
		readTrack := int(readCylinder)*2 + int(readHead)
		expectedTrack := cylinder*2 + head
		if readTrack != expectedTrack {
			// Wrong track, continue searching
			continue
		}

		// Verify size (should be 2 for 512-byte sectors)
		if size != 2 {
			// Wrong size, continue searching
			continue
		}

		// Scan for data marker (tag 0xFB)
		tag, err = r.scanIBMPC()
		if err != nil {
			return -1, nil, err
		}
		if tag == 0xfe {
			// Found another header marker instead of data marker, restart
			continue
		}
		if tag != 0xfb {
			// Invalid tag, continue searching
			continue
		}

		// Read sector data
		for i := 0; i < sectorSize; i++ {
			b, err := r.readByte()
			if err != nil {
				return -1, nil, err
			}
			data[i] = b
		}

		// Read data CRC
		dataSumHigh, err := r.readByte()
		if err != nil {
			return -1, nil, err
		}
		dataSumLow, err := r.readByte()
		if err != nil {
			return -1, nil, err
		}
		dataSum := uint16(dataSumHigh)<<8 | uint16(dataSumLow)

		// Verify data CRC (log warning but use data anyway)
		myDataSum := crc16CCITTByte(0xcdb4, 0xfb)
		myDataSum = crc16CCITT(myDataSum, data)
		if myDataSum != dataSum {
			// CRC mismatch, but use the data anyway
		}

		// Return sector number (0-based) and data
		return int(sector) - 1, data, nil
	}
}

// Constants for IBM PC floppy format
const (
	sectorSize  = 512
	indexGap    = 42   // bytes before first sector
	dataGap     = 22   // bytes between sector mark and data
	sectorGap9  = 80   // bytes for 9 sectors per track
	sectorGap10 = 46   // bytes for 10+ sectors per track
	gapByte     = 0x4E // standard gap byte
)

// detectFormatFromSize detects floppy format from file size
// Returns: cylinders, sides, sectorsPerTrack
func detectFormatFromSize(fileSize int64) (cylinders, sides, sectorsPerTrack int, err error) {
	// File size must be divisible by sector size
	if fileSize%sectorSize != 0 {
		return 0, 0, 0, fmt.Errorf("file size %d is not divisible by sector size %d", fileSize, sectorSize)
	}

	totalSectors := int(fileSize / sectorSize)

	// Try common floppy format combinations
	commonFormats := []struct {
		cylinders       int
		sides           int
		sectorsPerTrack int
		totalSectors    int
	}{
		{80, 2, 18, 2880}, // 1.44MB
		{80, 2, 9, 1440},  // 720KB
		{40, 2, 9, 720},   // 360KB
		{80, 2, 15, 2400}, // 1.2MB
	}

	for _, format := range commonFormats {
		if totalSectors == format.totalSectors {
			return format.cylinders, format.sides, format.sectorsPerTrack, nil
		}
	}

	// If no match, try to factor total sectors
	// Try common side counts (2 or 1)
	for sides := 2; sides > 0; sides-- {
		if totalSectors%sides != 0 {
			continue
		}
		sectorsPerSide := totalSectors / sides

		// Try common cylinder counts
		for cylinders := 80; cylinders >= 40; cylinders -= 40 {
			if sectorsPerSide%cylinders == 0 {
				sectorsPerTrack := sectorsPerSide / cylinders
				if sectorsPerTrack >= 8 && sectorsPerTrack <= 18 {
					return cylinders, sides, sectorsPerTrack, nil
				}
			}
		}
	}

	return 0, 0, 0, fmt.Errorf("unknown floppy image format %d sectors", totalSectors)
}

// mfmWriter writes MFM-encoded bits to a buffer
type mfmWriter struct {
	buffer      []byte // Output buffer
	bitPos      int    // Current bit position (0-based)
	lastDataBit int    // Last data bit for clock bit calculation
	maxHalfBits int    // Maximum number of half-bits allowed for this track
}

// Create a new MFM writer.
func newMFMWriter(maxHalfBits int) *mfmWriter {
	return &mfmWriter{
		buffer:      make([]byte, 0, 1024),
		bitPos:      0,
		lastDataBit: 0, // Start with 0 for clock bit calculation
		maxHalfBits: maxHalfBits,
	}
}

// Write a "half" bit, which means one MFM bit
func (w *mfmWriter) writeHalfBit(bitValue int) {
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

// Write one data bit, which means two MFM bits: clock + data.
func (w *mfmWriter) writeBit(dataBit int) {
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

// writeByte writes a data byte, encoding it as MFM (16 bits = 2 bytes)
func (w *mfmWriter) writeByte(data byte) {
	// Encode each bit of the data byte
	for i := 7; i >= 0; i-- {
		dataBit := int((data >> i) & 1)
		w.writeBit(dataBit)
	}
}

// writeGap writes n bytes of gap (repeated gapByte)
func (w *mfmWriter) writeGap(n int) {
	for i := 0; i < n; i++ {
		w.writeByte(gapByte)
	}
}

// writeMarker writes the A1 sync marker (12 bytes of 0x00 + 3 bytes of A1 with MFM violation)
// From ibmpc.c write_marker()
// A1 = 0xA1 = 10100001, but with MFM violations in bits 2 and 1 (half-bits)
func (w *mfmWriter) writeMarker() {
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

// writeIndexMarker writes the index marker (C2 sync)
// From ibmpc.c write_index_marker()
// C2 = 0xC2 = 11000010, but with MFM violations in bits 2 and 1 (half-bits)
func (w *mfmWriter) writeIndexMarker() {
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
}

// fillTrack fills remaining track space with gap bytes
func (w *mfmWriter) fillTrack() {
	// Fill to a reasonable track size (approximately 6250 bytes for 1.44MB at 500 kbps)
	// For now, we'll let the caller determine the exact size needed
	// This is a placeholder - actual implementation may need track size calculation
}

// getData returns the MFM-encoded buffer
func (w *mfmWriter) getData() []byte {
	// Trim to actual size used
	actualBytes := (w.bitPos + 7) / 8
	if actualBytes < len(w.buffer) {
		return w.buffer[:actualBytes]
	}
	return w.buffer
}

// encodeTrackIBMPC encodes a track in IBM PC format
// sectors: array of sector data (512 bytes each), indexed by sector number
// cylinder: cylinder number (0-based)
// head: head number (0 or 1)
// sectorsPerTrack: number of sectors per track
func encodeTrackIBMPC(sectors [][]byte, cylinder, head, sectorsPerTrack int, maxHalfBits int) []byte {
	writer := newMFMWriter(maxHalfBits)

	// Determine sector gap based on sectors per track
	sectorGap := sectorGap10
	if sectorsPerTrack == 9 {
		sectorGap = sectorGap9
	}

	// Index gap (before first sector)
	writer.writeGap(indexGap)

	// Write each sector
	for s := 0; s < sectorsPerTrack; s++ {
		if s > 0 {
			writer.writeGap(sectorGap)
		}

		// Sector marker (A1 sync)
		writer.writeMarker()

		// Sector header tag
		writer.writeByte(0xFE)

		// Sector identifier: cylinder, head, sector, size
		writer.writeByte(byte(cylinder))
		writer.writeByte(byte(head))
		writer.writeByte(byte(s + 1)) // Sector number (1-based)
		writer.writeByte(2)           // Size code (2 = 512 bytes)

		// Calculate header CRC
		sum := crc16CCITTByte(0xb230, byte(cylinder))
		sum = crc16CCITTByte(sum, byte(head))
		sum = crc16CCITTByte(sum, byte(s+1))
		sum = crc16CCITTByte(sum, 2)

		// Write header CRC
		writer.writeByte(byte(sum >> 8))
		writer.writeByte(byte(sum))

		// Data gap
		writer.writeGap(dataGap)

		// Data marker (A1 sync)
		writer.writeMarker()

		// Data tag
		writer.writeByte(0xFB)

		// Sector data
		sectorData := sectors[s]
		if sectorData == nil {
			// Missing sector, write zeros
			sectorData = make([]byte, sectorSize)
		}
		for _, b := range sectorData {
			writer.writeByte(b)
		}

		// Calculate data CRC
		sum = crc16CCITTByte(0xcdb4, 0xFB)
		sum = crc16CCITT(sum, sectorData)

		// Write data CRC
		writer.writeByte(byte(sum >> 8))
		writer.writeByte(byte(sum))
	}

	// Fill remaining track (approximate - actual size depends on bit rate)
	// For 1.44MB at 500 kbps, track is approximately 6250 bytes
	// We'll fill to a reasonable size
	trackSize := 6250 // Approximate track size in bytes
	currentSize := len(writer.getData())
	if currentSize < trackSize {
		writer.writeGap(trackSize - currentSize)
	}

	return writer.getData()
}

// Read a file in IMG or IMA format and return a Disk structure.
func ReadIMG(filename string) (*Disk, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}
	fileSize := fileInfo.Size()

	// Detect format from file size
	cylinders, sides, sectorsPerTrack, err := detectFormatFromSize(fileSize)
	if err != nil {
		return nil, fmt.Errorf("failed to detect format: %w", err)
	}

	// Read all sectors
	totalSectors := cylinders * sides * sectorsPerTrack
	sectors := make([][]byte, totalSectors)
	for i := 0; i < totalSectors; i++ {
		sectorData := make([]byte, sectorSize)
		n, err := file.Read(sectorData)
		if err != nil {
			return nil, fmt.Errorf("failed to read sector %d: %w", i, err)
		}
		if n < sectorSize {
			return nil, fmt.Errorf("incomplete sector %d: read %d bytes, expected %d", i, n, sectorSize)
		}
		sectors[i] = sectorData
	}

	// Group sectors by track and encode
	disk := &Disk{
		Header: Header{
			NumberOfTrack:       uint8(cylinders),
			NumberOfSide:        uint8(sides),
			TrackEncoding:       ENC_ISOIBM_MFM,
			BitRate:             500, // 500 kbps for HD floppy
			FloppyRPM:           300, // 300 RPM
			FloppyInterfaceMode: IFM_IBMPC_HD,
			WriteProtected:      0xFF,
			WriteAllowed:        0xFF,
			SingleStep:          0xFF,
			Track0S0AltEncoding: 0xFF,
			Track0S0Encoding:    ENC_ISOIBM_MFM,
			Track0S1AltEncoding: 0xFF,
			Track0S1Encoding:    ENC_ISOIBM_MFM,
		},
		Tracks: make([]TrackData, cylinders),
	}
	if sectorsPerTrack < 12 {
		// Double density
		disk.Header.BitRate = 250
		disk.Header.FloppyInterfaceMode = IFM_IBMPC_DD
	} else if sectorsPerTrack > 18 {
		// Extended density
		disk.Header.BitRate = 1000
		disk.Header.FloppyInterfaceMode = IFM_IBMPC_ED
	}
	if sectorsPerTrack == 15 {
		// 5.25" drive
		disk.Header.FloppyRPM = 360
	}

	// Max track length in MFM bits
	maxHalfBits := int(disk.Header.BitRate) * 1000 * 60 / int(disk.Header.FloppyRPM) * 2

	// Process each cylinder
	for cyl := 0; cyl < cylinders; cyl++ {
		// Process each side
		for head := 0; head < sides; head++ {
			// Collect sectors for this track
			trackSectors := make([][]byte, sectorsPerTrack)
			for s := 0; s < sectorsPerTrack; s++ {
				// Calculate sector index: track * sectorsPerTrack + sector
				track := cyl*sides + head
				sectorIndex := track*sectorsPerTrack + s
				trackSectors[s] = sectors[sectorIndex]
			}

			// Encode track to MFM
			mfmData := encodeTrackIBMPC(trackSectors, cyl, head, sectorsPerTrack, maxHalfBits)

			// Store in appropriate side
			if head == 0 {
				disk.Tracks[cyl].Side0 = mfmData
			} else {
				disk.Tracks[cyl].Side1 = mfmData
			}
		}
	}
	return disk, nil
}

// Scan side 0 of track 0 and returns the number of sectors.
// It counts unique sector numbers found in valid sector headers for cylinder 0, head 0.
// Returns the sector count (valid values: 8-23, 36).
func countSectorsIBMPC(sideData []byte) int {
	if len(sideData) == 0 {
		return 0
	}

	reader := newMFMReader(sideData)
	sectors := make(map[int]bool) // Track unique sector numbers (0-based)

	// Scan through the track looking for sector headers
	for {
		// Scan for sector header marker (tag 0xFE)
		tag, err := reader.scanIBMPC()
		if err != nil {
			// End of track or error, break
			break
		}
		if tag != 0xfe {
			// Not a sector header, continue scanning
			continue
		}

		// Read sector header
		readCylinder, err := reader.readByte()
		if err != nil {
			continue
		}
		readHead, err := reader.readByte()
		if err != nil {
			continue
		}
		sector, err := reader.readByte()
		if err != nil {
			continue
		}
		size, err := reader.readByte()
		if err != nil {
			continue
		}
		headerSumHigh, err := reader.readByte()
		if err != nil {
			continue
		}
		headerSumLow, err := reader.readByte()
		if err != nil {
			continue
		}
		headerSum := uint16(headerSumHigh)<<8 | uint16(headerSumLow)

		// Verify header CRC
		myHeaderSum := crc16CCITTByte(0xb230, readCylinder)
		myHeaderSum = crc16CCITTByte(myHeaderSum, readHead)
		myHeaderSum = crc16CCITTByte(myHeaderSum, sector)
		myHeaderSum = crc16CCITTByte(myHeaderSum, size)
		if myHeaderSum != headerSum {
			// CRC mismatch, continue searching
			continue
		}

		// Verify size (should be 2 for 512-byte sectors)
		if size != 2 {
			// Wrong size, continue searching
			continue
		}

		// Extract sector number (1-based in header, convert to 0-based)
		sectorNum := int(sector) - 1
		if sectorNum >= 0 {
			sectors[sectorNum] = true
		}
	}
	return len(sectors)
}

// Write disk contents to an IMG or IMA format file.
func WriteIMG(filename string, disk *Disk) error {
	// Create output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Figure out disk geometry
	const sectorSize = 512
	numCylinders := int(disk.Header.NumberOfTrack)
	if numCylinders > 80 {
		// Ignore extra cylinders
		numCylinders = 80
	}
	numHeads := int(disk.Header.NumberOfSide)
	numSectorsPerTrack := countSectorsIBMPC(disk.Tracks[0].Side0)
	if numSectorsPerTrack < 8 || (numSectorsPerTrack > 23 && numSectorsPerTrack != 36) {
		return fmt.Errorf("invalid number of sectors per track: %d (valid values: 8-23, 36)", numSectorsPerTrack)
	}

	// Iterate through cylinders and heads
	for cyl := 0; cyl < numCylinders; cyl++ {
		for head := 0; head < numHeads; head++ {

			// Get appropriate side data
			var sideData []byte
			if head == 0 {
				sideData = disk.Tracks[cyl].Side0
			} else {
				sideData = disk.Tracks[cyl].Side1
			}

			if len(sideData) == 0 {
				return fmt.Errorf("empty track %d.%d", cyl, head)
			}

			// Create MFM reader for this track
			reader := newMFMReader(sideData)

			// Extract all sectors from track (may appear in any order)
			sectors := make(map[int][]byte)

			// Read sectors sequentially until we can't find any more
			for len(sectors) < numSectorsPerTrack {
				// Try to read a sector
				sectorNum, sectorData, err := reader.readSectorIBMPC(cyl, head)
				if err != nil {
					// End of track or error, break
					break
				}

				// Validate sector number
				if sectorNum < 0 || sectorNum >= numSectorsPerTrack {
					// Invalid sector number, continue searching
					continue
				}

				// Store sector (overwrite if duplicate)
				sectors[sectorNum] = sectorData
			}

			// Write sectors in sequential order
			for s := 0; s < numSectorsPerTrack; s++ {
				sectorData, found := sectors[s]
				if !found {
					// Missing sector
					return fmt.Errorf("missing sector %d of track %d.%d", s, cyl, head)
				}

				// Write sector data
				if _, err := file.Write(sectorData); err != nil {
					return fmt.Errorf("failed to write sector %d of track %d.%d: %w", s, cyl, head, err)
				}
			}
		}
	}
	return nil
}
