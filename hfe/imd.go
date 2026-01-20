package hfe

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sergev/floppy/mfm"
)

const (
	// IMD comment block terminator
	imdCommentTerminator = 0x1A
)

// IMDTrack represents a single track in IMD format
type IMDTrack struct {
	Mode      byte   // Data rate and encoding (0-5)
	Cylinder  byte   // Physical cylinder number
	Head      byte   // Physical head with flags
	Nsec      byte   // Number of sectors
	Ssize     byte   // Encoded sector size (0-6)
	SectorMap []byte // Sector numbering map
	CylMap    []byte // Optional cylinder map
	HeadMap   []byte // Optional head map
	Sectors   []IMDSector
}

// IMDSector represents a single sector in IMD format
type IMDSector struct {
	Flag       byte   // Sector flag byte
	Data       []byte // Sector data (nil if no data)
	Compressed bool   // True if sector is compressed
	Deleted    bool   // True if deleted address mark
	Bad        bool   // True if bad sector
}

// imdSectorSize calculates the actual sector size from encoded size
func imdSectorSize(ssize byte) int {
	if ssize > 6 {
		return 0
	}
	return 128 << ssize
}

// modeToRateDensity decodes mode byte to data rate and encoding type
func modeToRateDensity(mode byte) (rate int, mfm bool, err error) {
	if mode > 5 {
		return 0, false, fmt.Errorf("invalid mode value: %d (must be 0-5)", mode)
	}
	rateTable := []int{500, 300, 250, 500, 300, 250}
	return rateTable[mode], mode >= 3, nil
}

// rateDensityToMode encodes data rate and encoding type to mode byte
func rateDensityToMode(rate int, mfm bool) (byte, error) {
	var baseMode int
	switch rate {
	case 500:
		baseMode = 0
	case 300:
		baseMode = 1
	case 250:
		baseMode = 2
	default:
		return 0, fmt.Errorf("unsupported data rate: %d kbps", rate)
	}
	if mfm {
		baseMode += 3
	}
	return byte(baseMode), nil
}

// isCompressible checks if all bytes in data are the same value
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

// calculateFlag calculates the sector flag byte from status flags
func calculateFlag(compressed, deleted, bad bool) byte {
	flag := byte(1) // Base: data present
	if compressed {
		flag |= 0x01
	}
	if deleted {
		flag |= 0x02
	}
	if bad {
		flag |= 0x04
	}
	return flag
}

// decodeFlag decodes a sector flag byte into status flags
func decodeFlag(flag byte) (compressed, deleted, bad bool) {
	if flag == 0 {
		return false, false, false // No data
	}
	compressed = (flag & 0x01) != 0
	deleted = (flag & 0x02) != 0
	bad = (flag & 0x04) != 0
	return
}

// ReadIMD reads a file in IMD format and returns a Disk structure.
func ReadIMD(filename string) (*Disk, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read comment block (until 0x1A)
	comment := make([]byte, 0, 1024)
	for {
		var b [1]byte
		n, err := file.Read(b[:])
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("unexpected EOF in comment block")
			}
			return nil, fmt.Errorf("failed to read comment: %w", err)
		}
		if n == 0 {
			return nil, fmt.Errorf("unexpected EOF in comment block")
		}
		if b[0] == imdCommentTerminator {
			break
		}
		comment = append(comment, b[0])
	}

	// Read track records
	var tracks []IMDTrack
	for {
		track, err := readIMDTrack(file)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read track: %w", err)
		}
		tracks = append(tracks, track)
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("no tracks found in IMD file")
	}

	// Validate that we have at least one valid track
	validTracks := 0
	for _, track := range tracks {
		if track.Nsec > 0 {
			validTracks++
		}
	}
	if validTracks == 0 {
		return nil, fmt.Errorf("no tracks with sectors found in IMD file")
	}

	// Convert IMD tracks to Disk structure
	// For now, we'll create a basic Disk structure
	// Full MFM encoding would require more complex conversion
	disk := &Disk{
		Header: Header{
			NumberOfTrack:       uint8(len(tracks)),
			NumberOfSide:        2, // Assume double-sided for now
			TrackEncoding:       ENC_ISOIBM_MFM,
			BitRate:             500,
			FloppyRPM:           300,
			FloppyInterfaceMode: IFM_IBMPC_HD,
			WriteProtected:      0xFF,
			WriteAllowed:        0xFF,
			SingleStep:          0xFF,
		},
		Tracks: make([]TrackData, len(tracks)),
	}

	// TODO: Convert IMD sector data to MFM bitstreams
	// For now, create empty tracks
	for i := range disk.Tracks {
		disk.Tracks[i] = TrackData{
			Side0: make([]byte, 0),
			Side1: make([]byte, 0),
		}
	}

	_ = comment // Comment is read but not used in conversion yet
	_ = tracks  // Tracks are read but conversion to MFM is not yet implemented

	return disk, nil
}

// readIMDTrack reads a single track record from IMD file
func readIMDTrack(file *os.File) (IMDTrack, error) {
	var track IMDTrack

	// Read track header (5 bytes)
	header := make([]byte, 5)
	n, err := io.ReadFull(file, header)
	if err != nil {
		if err == io.EOF && n == 0 {
			return track, io.EOF
		}
		if err == io.ErrUnexpectedEOF {
			return track, fmt.Errorf("incomplete track header: read %d bytes, expected 5", n)
		}
		return track, fmt.Errorf("failed to read track header: %w", err)
	}

	track.Mode = header[0]
	track.Cylinder = header[1]
	track.Head = header[2]
	track.Nsec = header[3]
	track.Ssize = header[4]

	// Validate header fields
	if track.Mode > 5 {
		return track, fmt.Errorf("invalid mode value: %d (must be 0-5)", track.Mode)
	}
	if (track.Head & 0x0F) > 1 {
		return track, fmt.Errorf("invalid head value: %d (must be 0-1)", track.Head&0x0F)
	}
	if track.Ssize > 6 {
		return track, fmt.Errorf("invalid sector size value: %d (must be 0-6)", track.Ssize)
	}

	// Handle null track (no sectors)
	if track.Nsec == 0 {
		return track, nil
	}

	// Read Sector Numbering Map
	if track.Nsec > 0 {
		track.SectorMap = make([]byte, track.Nsec)
		if _, err := io.ReadFull(file, track.SectorMap); err != nil {
			return track, fmt.Errorf("failed to read sector map: %w", err)
		}
	}

	// Read Cylinder Map if present
	if (track.Head & 0x80) != 0 {
		track.CylMap = make([]byte, track.Nsec)
		if _, err := io.ReadFull(file, track.CylMap); err != nil {
			return track, fmt.Errorf("failed to read cylinder map: %w", err)
		}
	}

	// Read Head Map if present
	if (track.Head & 0x40) != 0 {
		track.HeadMap = make([]byte, track.Nsec)
		if _, err := io.ReadFull(file, track.HeadMap); err != nil {
			return track, fmt.Errorf("failed to read head map: %w", err)
		}
	}

	// Read sector data blocks
	secSize := imdSectorSize(track.Ssize)
	if secSize == 0 {
		return track, fmt.Errorf("invalid sector size encoding: %d", track.Ssize)
	}
	track.Sectors = make([]IMDSector, track.Nsec)
	for i := byte(0); i < track.Nsec; i++ {
		sector, err := readIMDSector(file, secSize)
		if err != nil {
			return track, fmt.Errorf("failed to read sector %d (logical sector %d): %w", i, track.SectorMap[i], err)
		}
		track.Sectors[i] = sector
	}

	return track, nil
}

// readIMDSector reads a single sector data block from IMD file
func readIMDSector(file *os.File, secSize int) (IMDSector, error) {
	var sector IMDSector

	// Read flag byte
	var flag [1]byte
	n, err := io.ReadFull(file, flag[:])
	if err != nil {
		if err == io.EOF {
			return sector, fmt.Errorf("unexpected EOF reading sector flag")
		}
		return sector, fmt.Errorf("failed to read sector flag: %w", err)
	}
	if n < 1 {
		return sector, fmt.Errorf("incomplete sector flag: read %d bytes", n)
	}

	sector.Flag = flag[0]

	// No data available
	if sector.Flag == 0 {
		return sector, nil
	}

	// Decode flags
	sector.Compressed, sector.Deleted, sector.Bad = decodeFlag(sector.Flag)

	// Read sector data
	if sector.Compressed {
		// Compressed: single byte value
		var value [1]byte
		if _, err := io.ReadFull(file, value[:]); err != nil {
			if err == io.EOF {
				return sector, fmt.Errorf("unexpected EOF reading compressed sector value")
			}
			return sector, fmt.Errorf("failed to read compressed sector value: %w", err)
		}
		// Expand to full sector size
		sector.Data = make([]byte, secSize)
		for i := range sector.Data {
			sector.Data[i] = value[0]
		}
	} else {
		// Uncompressed: full sector data
		sector.Data = make([]byte, secSize)
		if _, err := io.ReadFull(file, sector.Data); err != nil {
			if err == io.EOF {
				return sector, fmt.Errorf("unexpected EOF reading sector data (expected %d bytes)", secSize)
			}
			return sector, fmt.Errorf("failed to read sector data: %w", err)
		}
	}

	return sector, nil
}

// WriteIMD writes a Disk structure to an IMD format file.
func WriteIMD(filename string, disk *Disk) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write comment block
	now := time.Now()
	comment := fmt.Sprintf("IMD 1.18: %02d/%02d/%04d %02d:%02d:%02d\r\n",
		now.Day(), now.Month(), now.Year(),
		now.Hour(), now.Minute(), now.Second())
	comment += "Created by floppy tool\r\n"

	if _, err := file.WriteString(comment); err != nil {
		return fmt.Errorf("failed to write comment: %w", err)
	}

	// Write comment terminator
	if _, err := file.Write([]byte{imdCommentTerminator}); err != nil {
		return fmt.Errorf("failed to write comment terminator: %w", err)
	}

	// Convert Disk to IMD format and write tracks
	// Process each track (cylinder/head combination)
	numCylinders := int(disk.Header.NumberOfTrack)
	if numCylinders == 0 {
		numCylinders = len(disk.Tracks)
	}
	numSides := int(disk.Header.NumberOfSide)
	if numSides == 0 {
		numSides = 1
	}

	for cyl := 0; cyl < numCylinders; cyl++ {
		for head := 0; head < numSides; head++ {
			// Get track data for this cylinder/head
			var trackData []byte
			if head == 0 && cyl < len(disk.Tracks) {
				trackData = disk.Tracks[cyl].Side0
			} else if head == 1 && cyl < len(disk.Tracks) {
				trackData = disk.Tracks[cyl].Side1
			}

			// Determine mode from disk header
			mode, err := rateDensityToMode(int(disk.Header.BitRate), disk.Header.TrackEncoding == ENC_ISOIBM_MFM)
			if err != nil {
				// Default to MFM 500 kbps
				mode = 3
			}

			// If track has no data, write null track
			if len(trackData) == 0 {
				header := []byte{
					mode,
					byte(cyl),
					byte(head),
					0, // Nsec = 0 (null track)
					2, // Ssize = 2 (512 bytes, default)
				}
				if _, err := file.Write(header); err != nil {
					return fmt.Errorf("failed to write track %d/%d header: %w", cyl, head, err)
				}
				continue
			}

			// Extract sectors from MFM bitstream
			reader := mfm.NewReader(trackData)
			sectors := make(map[int][]byte)
			sectorNumbers := make([]int, 0)

			// Read all sectors from track
			for {
				sectorNum, sectorData, err := reader.ReadSectorIBMPC(cyl, head)
				if err != nil {
					break // End of track or error
				}
				if sectorNum < 0 {
					continue // Invalid sector number
				}
				// Store sector (overwrite if duplicate)
				if _, exists := sectors[sectorNum]; !exists {
					sectorNumbers = append(sectorNumbers, sectorNum)
				}
				sectors[sectorNum] = sectorData
			}

			// If no sectors found, write null track
			if len(sectors) == 0 {
				header := []byte{
					mode,
					byte(cyl),
					byte(head),
					0, // Nsec = 0 (null track)
					2, // Ssize = 2 (512 bytes, default)
				}
				if _, err := file.Write(header); err != nil {
					return fmt.Errorf("failed to write track %d/%d header: %w", cyl, head, err)
				}
				continue
			}

			// Write track with sectors
			if err := writeIMDTrack(file, mode, byte(cyl), byte(head), sectors, sectorNumbers); err != nil {
				return fmt.Errorf("failed to write track %d/%d: %w", cyl, head, err)
			}
		}
	}

	return nil
}

// writeIMDTrack writes a complete track record to IMD file
func writeIMDTrack(file *os.File, mode, cylinder, head byte, sectors map[int][]byte, sectorNumbers []int) error {
	if len(sectors) == 0 {
		return fmt.Errorf("cannot write track with no sectors")
	}
	if len(sectors) > 255 {
		return fmt.Errorf("too many sectors: %d (max 255)", len(sectors))
	}
	nsec := byte(len(sectors))

	// Sector size is 512 bytes (encoded as 2)
	ssize := byte(2)

	// Build sector numbering map
	sectorMap := make([]byte, nsec)
	headFlags := head & 0x0F // Physical head number

	// Check if we need cylinder or head maps
	needCylMap := false
	needHeadMap := false
	cylMap := make([]byte, nsec)
	headMap := make([]byte, nsec)

	// Sort sector numbers and build maps
	// Sort sector numbers to maintain order
	for i := 0; i < len(sectorNumbers)-1; i++ {
		for j := i + 1; j < len(sectorNumbers); j++ {
			if sectorNumbers[i] > sectorNumbers[j] {
				sectorNumbers[i], sectorNumbers[j] = sectorNumbers[j], sectorNumbers[i]
			}
		}
	}

	// Build maps
	for i, sectorNum := range sectorNumbers {
		if i >= int(nsec) {
			break
		}
		// IMD sector numbers are typically 1-based in the map
		// But we'll use the actual sector number from the disk
		sectorMap[i] = byte(sectorNum)
		cylMap[i] = cylinder
		headMap[i] = headFlags
	}

	// Set flags if maps are needed (for now, assume uniform)
	// In practice, these would be set if cylinder/head values vary
	if needCylMap {
		headFlags |= 0x80
	}
	if needHeadMap {
		headFlags |= 0x40
	}

	// Write track header
	header := []byte{
		mode,
		cylinder,
		headFlags,
		nsec,
		ssize,
	}
	if _, err := file.Write(header); err != nil {
		return fmt.Errorf("failed to write track header: %w", err)
	}

	// Write sector numbering map
	if _, err := file.Write(sectorMap); err != nil {
		return fmt.Errorf("failed to write sector map: %w", err)
	}

	// Write cylinder map if needed
	if needCylMap {
		if _, err := file.Write(cylMap); err != nil {
			return fmt.Errorf("failed to write cylinder map: %w", err)
		}
	}

	// Write head map if needed
	if needHeadMap {
		if _, err := file.Write(headMap); err != nil {
			return fmt.Errorf("failed to write head map: %w", err)
		}
	}

	// Write sector data blocks
	secSize := imdSectorSize(ssize)
	if secSize == 0 {
		return fmt.Errorf("invalid sector size: %d", ssize)
	}
	for _, sectorNum := range sectorNumbers {
		sectorData, exists := sectors[sectorNum]
		if !exists {
			return fmt.Errorf("sector %d not found in sectors map", sectorNum)
		}
		if len(sectorData) != secSize && len(sectorData) > 0 {
			// Sector size mismatch - this is a warning but we'll pad/truncate
			if len(sectorData) < secSize {
				// Pad with zeros
				padded := make([]byte, secSize)
				copy(padded, sectorData)
				sectorData = padded
			} else {
				// Truncate
				sectorData = sectorData[:secSize]
			}
		}
		if err := writeIMDSector(file, sectorData, secSize); err != nil {
			return fmt.Errorf("failed to write sector %d: %w", sectorNum, err)
		}
	}

	return nil
}

// writeIMDSector writes a single sector data block to IMD file
func writeIMDSector(file *os.File, data []byte, secSize int) error {
	// Check if sector can be compressed
	compressed := isCompressible(data)
	var flag byte

	if compressed {
		// Compressed sector
		flag = calculateFlag(true, false, false)
		if _, err := file.Write([]byte{flag}); err != nil {
			return fmt.Errorf("failed to write sector flag: %w", err)
		}
		// Write single byte value
		if _, err := file.Write([]byte{data[0]}); err != nil {
			return fmt.Errorf("failed to write compressed sector value: %w", err)
		}
	} else {
		// Uncompressed sector
		flag = calculateFlag(false, false, false)
		if _, err := file.Write([]byte{flag}); err != nil {
			return fmt.Errorf("failed to write sector flag: %w", err)
		}
		// Write full sector data
		if len(data) < secSize {
			// Pad if necessary
			padded := make([]byte, secSize)
			copy(padded, data)
			data = padded
		} else if len(data) > secSize {
			data = data[:secSize]
		}
		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("failed to write sector data: %w", err)
		}
	}

	return nil
}
