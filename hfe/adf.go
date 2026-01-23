package hfe

import (
	"fmt"
	"github.com/sergev/floppy/mfm"
	"os"
)

const (
	adfSectorSize      = 512
	adfCylinders       = 80
	adfHeads           = 2
	adfSectorsPerTrack = 11
	adfTotalSize       = adfCylinders * adfHeads * adfSectorsPerTrack * adfSectorSize // 901,120 bytes
)

// ReadADF reads a file in ADF format and returns a Disk structure.
func ReadADF(filename string) (*Disk, error) {
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

	// Validate file size
	if fileSize != adfTotalSize {
		return nil, fmt.Errorf("invalid ADF file size: %d bytes (expected %d bytes)", fileSize, adfTotalSize)
	}

	// Read all sectors
	totalSectors := adfCylinders * adfHeads * adfSectorsPerTrack
	sectors := make([][]byte, totalSectors)
	for i := 0; i < totalSectors; i++ {
		sectorData := make([]byte, adfSectorSize)
		n, err := file.Read(sectorData)
		if err != nil {
			return nil, fmt.Errorf("failed to read sector %d: %w", i, err)
		}
		if n < adfSectorSize {
			return nil, fmt.Errorf("incomplete sector %d: read %d bytes, expected %d", i, n, adfSectorSize)
		}
		sectors[i] = sectorData
	}

	// Create HFE Disk structure
	disk := &Disk{
		Header: Header{
			NumberOfTrack:       adfCylinders,
			NumberOfSide:        adfHeads,
			TrackEncoding:       ENC_Amiga_MFM,
			BitRate:             250, // Amiga floppies always use 250 kbps
			FloppyRPM:           300,
			FloppyInterfaceMode: IFM_Amiga_DD,
			WriteProtected:      0xFF,
			WriteAllowed:        0xFF,
			SingleStep:          0xFF,
			Track0S0AltEncoding: 0xFF,
			Track0S0Encoding:    ENC_Amiga_MFM,
			Track0S1AltEncoding: 0xFF,
			Track0S1Encoding:    ENC_Amiga_MFM,
		},
		Tracks: make([]TrackData, adfCylinders),
	}

	// Max track length in MFM bits (250 kbps, 300 RPM)
	maxHalfBits := 250 * 1000 * 60 / 300 * 2

	// Process each cylinder
	for cyl := 0; cyl < adfCylinders; cyl++ {
		// Process each side
		for head := 0; head < adfHeads; head++ {
			// Collect sectors for this track
			trackSectors := make([][]byte, adfSectorsPerTrack)
			for s := 0; s < adfSectorsPerTrack; s++ {
				// Calculate sector index: (cylinder * heads + head) * sectorsPerTrack + sector
				trackIndex := cyl*adfHeads + head
				sectorIndex := trackIndex*adfSectorsPerTrack + s
				trackSectors[s] = sectors[sectorIndex]
			}

			// Encode track to Amiga MFM
			// Track number = cylinder*2 + head
			track := cyl*2 + head
			writer := mfm.NewWriter(maxHalfBits)
			mfmData := writer.EncodeTrackAmiga(trackSectors, track)

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

// WriteADF writes a Disk structure to an ADF format file.
func WriteADF(filename string, disk *Disk) error {
	// Validate disk geometry
	numCylinders := int(disk.Header.NumberOfTrack)
	if numCylinders < adfCylinders {
		return fmt.Errorf("invalid number of cylinders: %d (expected %d)", numCylinders, adfCylinders)
	}
	numHeads := int(disk.Header.NumberOfSide)
	if numHeads != adfHeads {
		return fmt.Errorf("invalid number of heads: %d (expected %d)", numHeads, adfHeads)
	}

	// Create output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Iterate through cylinders and heads
	for cyl := 0; cyl < adfCylinders; cyl++ {
		for head := 0; head < adfHeads; head++ {
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

			// Track number = cylinder*2 + head
			track := cyl*2 + head

			// Extract all sectors from track (may appear in any order)
			sectors := make(map[int][]byte)

			// Read sectors sequentially until we can't find any more
			for len(sectors) < adfSectorsPerTrack {
				// Try to read a sector
				sectorNum, sectorData, err := reader.ReadSectorAmiga(track)
				if err != nil {
					// End of track or error, break
					break
				}

				// Validate sector number
				if sectorNum < 0 || sectorNum >= adfSectorsPerTrack {
					// Invalid sector number, continue searching
					continue
				}

				// Store sector (overwrite if duplicate)
				sectors[sectorNum] = sectorData
			}

			// Write sectors in sequential order
			for s := 0; s < adfSectorsPerTrack; s++ {
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
