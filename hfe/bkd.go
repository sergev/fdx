package hfe

import (
	"fmt"
	"github.com/sergev/floppy/mfm"
	"os"
)

const (
	bkdCylinders       = 80
	bkdHeads           = 2
	bkdSectorsPerTrack = 10
	bkdExpectedSize    = bkdCylinders * bkdHeads * bkdSectorsPerTrack * sectorSize // 819,200 bytes
)

// Read a file in BKD format and return a Disk structure.
// BKD format has fixed geometry: 80 cylinders, 2 heads, 10 sectors per track.
// Tracks are encoded using IBMPC encoding but without index marks.
func ReadBKD(filename string) (*Disk, error) {
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

	// Validate file size - must be exactly 819,200 bytes
	if fileSize != bkdExpectedSize {
		return nil, fmt.Errorf("invalid BKD file size: %d bytes (expected %d)", fileSize, bkdExpectedSize)
	}

	// Read all sectors
	totalSectors := bkdCylinders * bkdHeads * bkdSectorsPerTrack
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
			NumberOfTrack:       bkdCylinders,
			NumberOfSide:        bkdHeads,
			TrackEncoding:       ENC_ISOIBM_MFM,
			BitRate:             250, // 250 kbps for double density
			FloppyRPM:           300, // 300 RPM
			FloppyInterfaceMode: IFM_IBMPC_DD,
			WriteProtected:      0xFF,
			WriteAllowed:        0xFF,
			SingleStep:          0xFF,
			Track0S0AltEncoding: 0xFF,
			Track0S0Encoding:    ENC_ISOIBM_MFM,
			Track0S1AltEncoding: 0xFF,
			Track0S1Encoding:    ENC_ISOIBM_MFM,
		},
		Tracks: make([]TrackData, bkdCylinders),
	}

	// Max track length in MFM bits
	maxHalfBits := int(disk.Header.BitRate) * 1000 * 60 / int(disk.Header.FloppyRPM) * 2

	// Process each cylinder
	for cyl := 0; cyl < bkdCylinders; cyl++ {
		// Process each side
		for head := 0; head < bkdHeads; head++ {
			// Collect sectors for this track
			trackSectors := make([][]byte, bkdSectorsPerTrack)
			for s := 0; s < bkdSectorsPerTrack; s++ {
				// Calculate sector index: track * sectorsPerTrack + sector
				track := cyl*bkdHeads + head
				sectorIndex := track*bkdSectorsPerTrack + s
				trackSectors[s] = sectors[sectorIndex]
			}

			// Encode track to MFM without index mark (skipIndexMark=true)
			writer := mfm.NewWriter(maxHalfBits)
			mfmData := writer.EncodeTrackBK(trackSectors, cyl, head, bkdSectorsPerTrack, disk.Header.BitRate)

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

// Write a Disk structure to a BKD format file.
// BKD format has fixed geometry: 80 cylinders, 2 heads, 10 sectors per track.
func WriteBKD(filename string, disk *Disk) error {
	// Validate disk geometry matches BKD format
	numCylinders := int(disk.Header.NumberOfTrack)
	numHeads := int(disk.Header.NumberOfSide)
	numSectorsPerTrack := countSectors(disk.Tracks[0].Side0)

	if numCylinders != bkdCylinders {
		return fmt.Errorf("invalid number of cylinders: %d (BKD format requires %d)", numCylinders, bkdCylinders)
	}
	if numHeads != bkdHeads {
		return fmt.Errorf("invalid number of heads: %d (BKD format requires %d)", numHeads, bkdHeads)
	}
	if numSectorsPerTrack != bkdSectorsPerTrack {
		return fmt.Errorf("invalid number of sectors per track: %d (BKD format requires %d)", numSectorsPerTrack, bkdSectorsPerTrack)
	}

	// Create output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

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
