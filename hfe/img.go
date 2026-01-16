package hfe

import (
	"fmt"
	"github.com/sergev/floppy/mfm"
	"os"
)

const (
	sectorSize = 512 // sector size in bytes
)

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
	cylinders, sides, sectorsPerTrack, err := mfm.DetectFormatFromSize(fileSize)
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
			writer := mfm.NewWriter(maxHalfBits)
			mfmData := writer.EncodeTrackIBMPC(trackSectors, cyl, head, sectorsPerTrack, disk.Header.BitRate)

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

// Scan track contents and returns the number of sectors.
func countSectors(sideData []byte) int {
	reader := mfm.NewReader(sideData)
	return reader.CountSectorsIBMPC()
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
	numCylinders := int(disk.Header.NumberOfTrack)
	if numCylinders > 80 {
		// Ignore extra cylinders
		numCylinders = 80
	}
	numHeads := int(disk.Header.NumberOfSide)
	numSectorsPerTrack := countSectors(disk.Tracks[0].Side0)
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
			reader := mfm.NewReader(sideData)

			// Extract all sectors from track (may appear in any order)
			sectors := make(map[int][]byte)

			// Read sectors sequentially until we can't find any more
			for len(sectors) < numSectorsPerTrack {
				// Try to read a sector
				sectorNum, sectorData, err := reader.ReadSectorIBMPC(cyl, head)
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
