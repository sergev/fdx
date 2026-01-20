package mfm

import (
	"fmt"
)

const (
	sectorSize = 512 // sector size in bytes
)

// Read bits from an MFM bitstream (MSB-first byte order)
// In MFM encoding: each data bit is encoded as 2 bits.
type Reader struct {
	data   []byte // MFM bitstream data (two bits per each data bit)
	bitPos int    // Current bit position in raw bitstream (0-based)
}

// Create a new MFM bitstream reader
func NewReader(data []byte) *Reader {
	return &Reader{
		data:   data,
		bitPos: 0,
	}
}

// Read "half" bit, which means a raw next bit from MFM stream.
func (r *Reader) readHalfBit() (int, error) {
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
func (r *Reader) readBit() (int, error) {
	// Ignore the first half-bit
	_, err := r.readHalfBit()
	if err != nil {
		return -1, err
	}

	// Return the second half-bit
	bit, err := r.readHalfBit()
	if err != nil {
		return -1, err
	}
	return bit, nil
}

// Read 8 bits and return them as a byte
func (r *Reader) readByte() (byte, error) {
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

// Scan for IBM PC sector markers
// Return the tag byte after the marker, or error
func (r *Reader) scanIBMPC() (int, error) {
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

// Read a sector from IBM PC format
// Return: sector number (0-based), 512-byte data, error
func (r *Reader) ReadSectorIBMPC(cylinder, head int) (int, []byte, error) {
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
			fmt.Printf("Warning: bad checksum in sector %d of track %d.%d\n", sector, cylinder, head)
			continue
		}

		// Return sector number (0-based) and data
		return int(sector) - 1, data, nil
	}
}

// Scan track contents and returns the number of sectors.
// It counts unique sector numbers found in valid sector headers.
// Returns the sector count.
func (r *Reader) CountSectorsIBMPC() int {
	// Track unique sector numbers (0-based)
	sectors := make(map[int]bool)

	// Scan through the track looking for sector headers
	for {
		// Scan for sector header marker (tag 0xFE)
		tag, err := r.scanIBMPC()
		if err != nil {
			// End of track or error, break
			break
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

// Detect floppy format from file size
// Return: cylinders, sides, sectorsPerTrack
func DetectFormatFromSize(fileSize int64) (cylinders, sides, sectorsPerTrack int, err error) {
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
	}{
		// 3½" HD
		{80, 2, 18}, // 1.44M
		{80, 2, 20}, // 1.6M
		// 3½" DD
		{80, 2, 9},  // 720K
		{80, 2, 10}, // 800K
		// 3½" DD single side
		{80, 1, 9}, // 360K
		// 3½" ED
		{80, 2, 36}, // 2.88M
		{80, 2, 39}, // 3.12M
		// 5¼" AT HD
		{80, 2, 15}, // 1.2M
		// 5¼" AT DD
		{40, 2, 9}, // 360K
		// 5¼" XT DD
		{40, 2, 8}, // 320K
		{40, 2, 9}, // 360K
		// 5¼" XT DD single side
		{40, 1, 8}, // 160K
		{40, 1, 9}, // 180K
	}

	for _, format := range commonFormats {
		if totalSectors == format.cylinders*format.sides*format.sectorsPerTrack {
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

// unshuffle reconstructs a 32-bit word from odd and even bit streams.
// The first argument contains the odd bits, the second - the even bits.
func unshuffle(odd, even uint16) uint32 {
	var word uint32
	for i := 0; i < 16; i++ {
		word <<= 2
		word |= uint32((even>>15)&1) | uint32((odd>>14)&2)
		odd <<= 1
		even <<= 1
	}
	return word
}

// scanAmiga scans for Amiga sector markers (00-a1-a1-fx pattern).
// Returns the tag byte after the marker, or error.
func (r *Reader) scanAmiga() (int, error) {
	history := uint32(0)

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

		// Amiga format: waiting for 00-a1-a1-fx
		if (history & 0xfffffff0) == 0x00a1a1f0 {
			// Found marker, read and return its tag
			tag := history & 0xff
			return int(tag), nil
		}
	}
}

// readLong reads a 32-bit word with bit shuffling and calculates the checksum.
// Returns the word and updates the checksum sum.
func (r *Reader) readLong(sum *uint32) (uint32, error) {
	oddHigh, err := r.readByte()
	if err != nil {
		return 0, err
	}
	oddLow, err := r.readByte()
	if err != nil {
		return 0, err
	}
	evenHigh, err := r.readByte()
	if err != nil {
		return 0, err
	}
	evenLow, err := r.readByte()
	if err != nil {
		return 0, err
	}

	odd := uint16(oddHigh)<<8 | uint16(oddLow)
	even := uint16(evenHigh)<<8 | uint16(evenLow)
	*sum ^= uint32(odd) ^ uint32(even)

	return unshuffle(odd, even), nil
}

// readDataAmiga reads a 512-byte block with bit shuffling.
// Returns the checksum.
func (r *Reader) readDataAmiga(data []byte) (uint32, error) {
	if len(data) != sectorSize {
		return 0, fmt.Errorf("data buffer must be %d bytes", sectorSize)
	}

	// Read odd bits (first half)
	odd := make([]uint16, sectorSize/4)
	for i := 0; i < sectorSize/4; i++ {
		high, err := r.readByte()
		if err != nil {
			return 0, err
		}
		low, err := r.readByte()
		if err != nil {
			return 0, err
		}
		odd[i] = uint16(high)<<8 | uint16(low)
	}

	// Read even bits (second half)
	even := make([]uint16, sectorSize/4)
	for i := 0; i < sectorSize/4; i++ {
		high, err := r.readByte()
		if err != nil {
			return 0, err
		}
		low, err := r.readByte()
		if err != nil {
			return 0, err
		}
		even[i] = uint16(high)<<8 | uint16(low)
	}

	// Restore data and calculate checksum
	var sum uint32
	for i := 0; i < sectorSize/4; i++ {
		ldata := unshuffle(odd[i], even[i])
		sum ^= uint32(odd[i]) ^ uint32(even[i])
		data[4*i] = byte(ldata >> 24)
		data[4*i+1] = byte(ldata >> 16)
		data[4*i+2] = byte(ldata >> 8)
		data[4*i+3] = byte(ldata)
	}

	return sum, nil
}

// ReadSectorAmiga reads the next sector from an Amiga format track.
// Returns: sector number (0-based), 512-byte data, error
func (r *Reader) ReadSectorAmiga(track int) (int, []byte, error) {
	data := make([]byte, sectorSize)

	for {
		// Scan for Amiga sector marker (returns tag byte)
		tag, err := r.scanAmiga()
		if err != nil {
			return -1, nil, err
		}

		// Read identifier: tag is high byte of odd, then read 3 more bytes
		oddLow, err := r.readByte()
		if err != nil {
			continue
		}
		evenHigh, err := r.readByte()
		if err != nil {
			continue
		}
		evenLow, err := r.readByte()
		if err != nil {
			continue
		}

		odd := uint16(tag)<<8 | uint16(oddLow)
		even := uint16(evenHigh)<<8 | uint16(evenLow)
		ident := unshuffle(odd, even) & 0xffffff

		readTrack := int(ident >> 16)
		sector := int((ident >> 8) & 0xff)
		myHeaderSum := uint32(odd) ^ uint32(even)

		// Read sector label (4 longs)
		label := make([]uint32, 4)
		for i := 0; i < 4; i++ {
			label[i], err = r.readLong(&myHeaderSum)
			if err != nil {
				continue
			}
		}

		// Read header checksum
		headerSumHigh, err := r.readByte()
		if err != nil {
			continue
		}
		headerSumMid1, err := r.readByte()
		if err != nil {
			continue
		}
		headerSumMid2, err := r.readByte()
		if err != nil {
			continue
		}
		headerSumLow, err := r.readByte()
		if err != nil {
			continue
		}
		headerSum := uint32(headerSumHigh)<<24 | uint32(headerSumMid1)<<16 | uint32(headerSumMid2)<<8 | uint32(headerSumLow)

		if myHeaderSum != headerSum {
			// Header checksum mismatch, continue searching
			continue
		}

		if readTrack != track {
			// Wrong track, continue searching
			continue
		}

		// Read data checksum (before data)
		dataSumHigh, err := r.readByte()
		if err != nil {
			continue
		}
		dataSumMid1, err := r.readByte()
		if err != nil {
			continue
		}
		dataSumMid2, err := r.readByte()
		if err != nil {
			continue
		}
		dataSumLow, err := r.readByte()
		if err != nil {
			continue
		}
		dataSum := uint32(dataSumHigh)<<24 | uint32(dataSumMid1)<<16 | uint32(dataSumMid2)<<8 | uint32(dataSumLow)

		// Read sector data
		myDataSum, err := r.readDataAmiga(data)
		if err != nil {
			continue
		}

		if myDataSum != dataSum {
			// Data checksum mismatch, but use the data anyway
		}

		// Return sector number (0-based) and data
		return sector, data, nil
	}
}

// CountSectorsAmiga counts the number of sectors on an Amiga track.
// Returns the sector count.
func (r *Reader) CountSectorsAmiga(track int) int {
	// Track unique sector numbers (0-based)
	sectors := make(map[int]bool)

	// Scan through the track looking for sectors
	for {
		sector, _, err := r.ReadSectorAmiga(track)
		if err != nil {
			// End of track or error, break
			break
		}
		if sector >= 0 && sector < 11 {
			sectors[sector] = true
		}
	}
	return len(sectors)
}
