package hfe

import (
	"bytes"
	"fmt"

	"github.com/sergev/floppy/mfm"
)

// Set flags VerifyIBMPC and VerifyAmiga
func (disk *Disk) InitVerifyOptions() {

	// Count IBMPC sectors on cyl 0 side 0
	reader := mfm.NewReader(disk.Tracks[0].Side0)
	disk.VerifyIBMPC = reader.CountSectorsIBMPC() > 0
	if disk.VerifyIBMPC {
		return
	}

	// Count Amiga sectors on cyl 0 side 0
	reader = mfm.NewReader(disk.Tracks[0].Side0)
	disk.VerifyAmiga = reader.CountSectorsAmiga(0) > 0
}

// Return true when disk must be verified after write.
func (disk *Disk) MustVerify() bool {
	return disk.VerifyIBMPC || disk.VerifyAmiga
}

// Compare data from MFM stream with track on disk
func (disk *Disk) VerifyTrack(cyl int, head int, readBits []byte) error {
	var writeBits []byte
	if head == 0 {
		writeBits = disk.Tracks[cyl].Side0
	} else {
		writeBits = disk.Tracks[cyl].Side1
	}

	if disk.VerifyIBMPC {
		err := disk.VerifyTrackIBMPC(cyl, head, writeBits, readBits)
		if err != nil {
			return err
		}
	}

	if disk.VerifyAmiga {
		err := disk.VerifyTrackAmiga(cyl, head, writeBits, readBits)
		if err != nil {
			return err
		}
	}
	return nil
}

// Decode and compare IBMPC data from MFM streams
func (disk *Disk) VerifyTrackIBMPC(cyl, head int, writeBits, readBits []byte) error {

	// Compare number of sectors
	reader := mfm.NewReader(writeBits)
	numSectors := reader.CountSectorsIBMPC()
	reader = mfm.NewReader(readBits)
	numReadSectors := reader.CountSectorsIBMPC()
	if numSectors != numReadSectors {
		return fmt.Errorf("written %d sectors, read %d sectors", numSectors, numReadSectors)
	}

	// Extract all written sectors
	reader = mfm.NewReader(writeBits)
	sectors := make(map[int][]byte)
	for len(sectors) < numSectors {
		// Try to read a sector
		sectorNum, sectorData, err := reader.ReadSectorIBMPC(cyl, head)
		if err != nil {
			// End of track or error
			break
		}
		if sectorNum < 0 || sectorNum >= numSectors {
			// Invalid sector number, continue searching
			continue
		}

		// Store sector (overwrite if duplicate)
		sectors[sectorNum] = sectorData
	}
	if len(sectors) != numSectors {
		// Cannot happen
		return fmt.Errorf("bad write data")
	}

	// Compare all read sectors
	reader = mfm.NewReader(readBits)
	countSectors := 0
	for countSectors < numSectors {
		// Try to read a sector
		sectorNum, sectorData, err := reader.ReadSectorIBMPC(cyl, head)
		if err != nil {
			// End of track or error
			break
		}
		if sectorNum < 0 || sectorNum >= numSectors {
			// Invalid sector number, continue searching
			continue
		}

		// Compare sector data
		if !bytes.Equal(sectors[sectorNum], sectorData) {
			return fmt.Errorf("bad data in sector %d", sectorNum)
		}
		countSectors++
	}
	if countSectors != numSectors {
		return fmt.Errorf("missing sectors")
	}
	return nil
}

// Decode and compare Amiga data from MFM streams
func (disk *Disk) VerifyTrackAmiga(cyl, head int, writeBits, readBits []byte) error {

	// Compare number of sectors
	track := cyl*2 + head
	reader := mfm.NewReader(writeBits)
	numSectors := reader.CountSectorsAmiga(track)
	reader = mfm.NewReader(readBits)
	numReadSectors := reader.CountSectorsAmiga(track)
	if numSectors != numReadSectors {
		return fmt.Errorf("written %d sectors, read %d sectors", numSectors, numReadSectors)
	}

	// Extract all written sectors
	reader = mfm.NewReader(writeBits)
	sectors := make(map[int][]byte)
	for len(sectors) < numSectors {
		// Try to read a sector
		sectorNum, sectorData, err := reader.ReadSectorAmiga(track)
		if err != nil {
			// End of track or error
			break
		}
		if sectorNum < 0 || sectorNum >= numSectors {
			// Invalid sector number, continue searching
			continue
		}

		// Store sector (overwrite if duplicate)
		sectors[sectorNum] = sectorData
	}
	if len(sectors) != numSectors {
		// Cannot happen
		return fmt.Errorf("bad write data")
	}

	// Compare all read sectors
	reader = mfm.NewReader(readBits)
	countSectors := 0
	for countSectors < numSectors {
		// Try to read a sector
		sectorNum, sectorData, err := reader.ReadSectorAmiga(track)
		if err != nil {
			// End of track or error
			break
		}
		if sectorNum < 0 || sectorNum >= numSectors {
			// Invalid sector number, continue searching
			continue
		}

		// Compare sector data
		if !bytes.Equal(sectors[sectorNum], sectorData) {
			return fmt.Errorf("bad data in sector %d", sectorNum)
		}
		countSectors++
	}
	if countSectors != numSectors {
		return fmt.Errorf("missing sectors")
	}
	return nil
}
