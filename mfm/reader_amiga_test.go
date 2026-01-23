package mfm

import (
	"os"
	"path/filepath"
	"testing"
)

// findSampleFile searches for a sample file in multiple possible locations
// and returns the found path, or skips the test if not found
func findSampleFile(t *testing.T, filename string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	possiblePaths := []string{
		filepath.Join(wd, "images", filename),
		filepath.Join(wd, "..", "images", filename),
		filepath.Join("images", filename),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	t.Skipf("Sample file %s not found in any of: %v", filename, possiblePaths)
	return ""
}

// loadTrackData loads track data from a binary file
// The track should be extracted using: go run cmd/extract_track/main.go images/amiga.hfe images/amiga_track0_side0.bin
func loadTrackData(t *testing.T) []byte {
	t.Helper()

	// Determine path for extracted track
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	possiblePaths := []string{
		filepath.Join(wd, "images", "amiga_track0_side0.bin"),
		filepath.Join(wd, "..", "images", "amiga_track0_side0.bin"),
		filepath.Join("images", "amiga_track0_side0.bin"),
	}

	var trackPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			trackPath = path
			break
		}
	}

	if trackPath == "" {
		t.Skipf("Track data file not found. Extract it using: go run cmd/extract_track/main.go images/amiga.hfe images/amiga_track0_side0.bin")
		return nil
	}

	// Load the track data
	trackData, err := os.ReadFile(trackPath)
	if err != nil {
		t.Fatalf("Failed to read track data from %s: %v", trackPath, err)
	}

	if len(trackData) == 0 {
		t.Fatalf("Track data file is empty: %s", trackPath)
	}

	return trackData
}

// TestReadSectorAmiga_AmigaTrack0 tests ReadSectorAmiga() with the first track from amiga.hfe
func TestReadSectorAmiga_AmigaTrack0(t *testing.T) {
	// Load track data (extract from HFE if needed)
	trackData := loadTrackData(t)
	if trackData == nil {
		return // Test was skipped
	}

	t.Logf("Loaded track data: %d bytes", len(trackData))

	// Create MFM reader with the track data
	reader := NewReader(trackData)

	// Track number for cylinder 0, side 0 is 0
	track := 0

	// Expected number of sectors for Amiga format
	expectedSectors := 11

	// Attempt to read all sectors
	sectors := make(map[int][]byte)

	// Try to read sectors - we'll attempt multiple times to find all sectors
	maxAttempts := 100 // Limit attempts to avoid infinite loop
	attempts := 0

	for len(sectors) < expectedSectors && attempts < maxAttempts {
		sectorNum, sectorData, err := reader.ReadSectorAmiga(track)
		if err != nil {
			// End of track or error
			if attempts == 0 {
				// First attempt failed - this is the bug we're trying to debug
				t.Errorf("Failed to read any sectors from track 0: %v", err)
			}
			break
		}

		// Validate sector number
		if sectorNum < 0 || sectorNum >= expectedSectors {
			// Invalid sector number, continue searching
			attempts++
			continue
		}

		// Store sector (overwrite if duplicate)
		if _, exists := sectors[sectorNum]; exists {
			t.Logf("Found duplicate sector %d", sectorNum)
		}
		sectors[sectorNum] = sectorData
		attempts++

		// Log progress
		if len(sectors)%5 == 0 {
			t.Logf("Found %d/%d sectors", len(sectors), expectedSectors)
		}
	}

	// Report results
	t.Logf("Read %d sectors after %d attempts", len(sectors), attempts)

	// Verify we found all expected sectors
	if len(sectors) < expectedSectors {
		missing := []int{}
		for s := 0; s < expectedSectors; s++ {
			if _, found := sectors[s]; !found {
				missing = append(missing, s)
			}
		}
		t.Errorf("Missing sectors: %v (found %d/%d sectors)", missing, len(sectors), expectedSectors)
	}

	// Verify each sector has correct size
	for sectorNum, sectorData := range sectors {
		if len(sectorData) != sectorSize {
			t.Errorf("Sector %d has incorrect size: got %d bytes, expected %d bytes", sectorNum, len(sectorData), sectorSize)
		}
	}
}
