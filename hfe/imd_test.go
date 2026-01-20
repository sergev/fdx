package hfe

import (
	"fmt"
	"strings"
	"testing"
)

func TestReadIMDFile(t *testing.T) {
	// Find the test file
	sampleFile := findSampleFile(t, "fat360.imd")
	if sampleFile == "" {
		return // Test was skipped
	}

	// Read the IMD file
	img, err := ReadIMDFile(sampleFile)
	if err != nil {
		t.Fatalf("ReadIMDFile() error: %v", err)
	}

	// Print mode flag - set to false after capturing expected values
	printExpected := false

	if printExpected {
		// Print comment block information
		fmt.Println("=== COMMENT BLOCK ===")
		fmt.Printf("Comment length: %d bytes\n", len(img.Comment))
		fmt.Printf("Comment (first 200 chars): %q\n", string(img.Comment[:min(200, len(img.Comment))]))
		fmt.Printf("Comment starts with: %q\n", string(img.Comment[:min(len(img.Comment), 30)]))

		// Print track count
		fmt.Println("\n=== TRACK COUNT ===")
		fmt.Printf("Number of tracks: %d\n", len(img.Tracks))

		// Print first track header
		if len(img.Tracks) > 0 {
			track := img.Tracks[0]
			fmt.Println("\n=== FIRST TRACK HEADER ===")
			fmt.Printf("Mode: %d\n", track.Mode)
			fmt.Printf("Cylinder: %d\n", track.Cylinder)
			fmt.Printf("Head: %d (head number: %d, cyl map flag: %v, head map flag: %v)\n",
				track.Head, track.Head&0x0F, (track.Head&0x80) != 0, (track.Head&0x40) != 0)
			fmt.Printf("Nsec: %d\n", track.Nsec)
			fmt.Printf("Ssize: %d (sector size: %d bytes)\n", track.Ssize, imdSectorSize(track.Ssize))

			// Print sector map
			if len(track.SectorMap) > 0 {
				fmt.Printf("SectorMap: %v\n", track.SectorMap)
			}

			// Print optional maps
			if len(track.CylMap) > 0 {
				fmt.Printf("CylMap: %v\n", track.CylMap)
			}
			if len(track.HeadMap) > 0 {
				fmt.Printf("HeadMap: %v\n", track.HeadMap)
			}

			// Print first few sector headers
			fmt.Println("\n=== FIRST FEW SECTOR HEADERS ===")
			maxSectorsToPrint := 5
			if len(track.Sectors) < maxSectorsToPrint {
				maxSectorsToPrint = len(track.Sectors)
			}
			for i := 0; i < maxSectorsToPrint; i++ {
				sector := track.Sectors[i]
				fmt.Printf("Sector %d:\n", i)
				fmt.Printf("  Flag: 0x%02X\n", sector.Flag)
				fmt.Printf("  Compressed: %v\n", sector.Compressed)
				fmt.Printf("  Deleted: %v\n", sector.Deleted)
				fmt.Printf("  Bad: %v\n", sector.Bad)
				fmt.Printf("  Data length: %d bytes\n", len(sector.Data))
				if len(sector.Data) > 0 {
					fmt.Printf("  First 16 bytes: %v\n", sector.Data[:min(16, len(sector.Data))])
				}
			}
		}

		// Print track information for all tracks
		fmt.Println("\n=== ALL TRACK HEADERS ===")
		for i, track := range img.Tracks {
			fmt.Printf("Track %d: Mode=%d, Cyl=%d, Head=%d, Nsec=%d, Ssize=%d\n",
				i, track.Mode, track.Cylinder, track.Head, track.Nsec, track.Ssize)
		}

		// Print summary statistics
		fmt.Println("\n=== SUMMARY ===")
		totalSectors := 0
		compressedSectors := 0
		deletedSectors := 0
		badSectors := 0
		for _, track := range img.Tracks {
			totalSectors += len(track.Sectors)
			for _, sector := range track.Sectors {
				if sector.Compressed {
					compressedSectors++
				}
				if sector.Deleted {
					deletedSectors++
				}
				if sector.Bad {
					badSectors++
				}
			}
		}
		fmt.Printf("Total sectors: %d\n", totalSectors)
		fmt.Printf("Compressed sectors: %d\n", compressedSectors)
		fmt.Printf("Deleted sectors: %d\n", deletedSectors)
		fmt.Printf("Bad sectors: %d\n", badSectors)
	}

	// Assertions based on captured values from first run
	// Comment block validation
	if len(img.Comment) != 50 {
		t.Errorf("Comment length = %d, expected 50", len(img.Comment))
	}
	expectedCommentStart := "IMD 1.17: 19/01/2026 20:15:13\r"
	if !strings.HasPrefix(string(img.Comment), expectedCommentStart) {
		t.Errorf("Comment does not start with expected text. Got: %q", string(img.Comment[:min(len(img.Comment), 30)]))
	}

	// Track count validation
	expectedTrackCount := 80
	if len(img.Tracks) != expectedTrackCount {
		t.Errorf("Number of tracks = %d, expected %d", len(img.Tracks), expectedTrackCount)
	}

	// First track header validation
	if len(img.Tracks) > 0 {
		track := img.Tracks[0]
		if track.Mode != 5 {
			t.Errorf("First track Mode = %d, expected 5", track.Mode)
		}
		if track.Cylinder != 0 {
			t.Errorf("First track Cylinder = %d, expected 0", track.Cylinder)
		}
		if track.Head != 0 {
			t.Errorf("First track Head = %d, expected 0", track.Head)
		}
		if track.Nsec != 9 {
			t.Errorf("First track Nsec = %d, expected 9", track.Nsec)
		}
		if track.Ssize != 2 {
			t.Errorf("First track Ssize = %d, expected 2", track.Ssize)
		}
		expectedSectorSize := imdSectorSize(2)
		if expectedSectorSize != 512 {
			t.Errorf("Sector size = %d, expected 512", expectedSectorSize)
		}

		// Sector map validation
		expectedSectorMap := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
		if len(track.SectorMap) != len(expectedSectorMap) {
			t.Errorf("SectorMap length = %d, expected %d", len(track.SectorMap), len(expectedSectorMap))
		} else {
			for i, expected := range expectedSectorMap {
				if track.SectorMap[i] != expected {
					t.Errorf("SectorMap[%d] = %d, expected %d", i, track.SectorMap[i], expected)
				}
			}
		}

		// Optional maps should not be present for first track
		if len(track.CylMap) != 0 {
			t.Errorf("First track CylMap length = %d, expected 0", len(track.CylMap))
		}
		if len(track.HeadMap) != 0 {
			t.Errorf("First track HeadMap length = %d, expected 0", len(track.HeadMap))
		}

		// Sector headers validation
		if len(track.Sectors) != int(track.Nsec) {
			t.Errorf("Number of sectors = %d, expected %d", len(track.Sectors), track.Nsec)
		}

		// First sector validation
		if len(track.Sectors) > 0 {
			sector := track.Sectors[0]
			if sector.Flag != 0x01 {
				t.Errorf("First sector Flag = 0x%02X, expected 0x01", sector.Flag)
			}
			if sector.Compressed {
				t.Errorf("First sector Compressed = %v, expected false", sector.Compressed)
			}
			if sector.Deleted {
				t.Errorf("First sector Deleted = %v, expected false", sector.Deleted)
			}
			if sector.Bad {
				t.Errorf("First sector Bad = %v, expected false", sector.Bad)
			}
			if len(sector.Data) != 512 {
				t.Errorf("First sector Data length = %d, expected 512", len(sector.Data))
			}
			// Check first few bytes match expected
			expectedFirstBytes := []byte{235, 52, 144, 77, 83, 68, 79, 83, 51, 46, 51, 0, 2, 2, 1, 0}
			if len(sector.Data) >= len(expectedFirstBytes) {
				for i, expected := range expectedFirstBytes {
					if sector.Data[i] != expected {
						t.Errorf("First sector Data[%d] = %d, expected %d", i, sector.Data[i], expected)
						break // Only report first mismatch
					}
				}
			}
		}

		// Second sector validation (compressed sector)
		if len(track.Sectors) > 1 {
			sector := track.Sectors[1]
			if sector.Flag != 0x01 {
				t.Errorf("Second sector Flag = 0x%02X, expected 0x01", sector.Flag)
			}
		}

		// Third sector validation (compressed sector with flag 0x02)
		if len(track.Sectors) > 2 {
			sector := track.Sectors[2]
			if sector.Flag != 0x02 {
				t.Errorf("Third sector Flag = 0x%02X, expected 0x02", sector.Flag)
			}
			if !sector.Compressed {
				t.Errorf("Third sector Compressed = %v, expected true", sector.Compressed)
			}
			if sector.Deleted {
				t.Errorf("Third sector Deleted = %v, expected false", sector.Deleted)
			}
			if sector.Bad {
				t.Errorf("Third sector Bad = %v, expected false", sector.Bad)
			}
			if len(sector.Data) != 512 {
				t.Errorf("Third sector Data length = %d, expected 512", len(sector.Data))
			}
			// Check that all bytes are the same (compressed sector)
			if len(sector.Data) > 0 {
				firstByte := sector.Data[0]
				for i, b := range sector.Data {
					if b != firstByte {
						t.Errorf("Third sector Data[%d] = %d, expected all bytes to be %d (compressed sector)", i, b, firstByte)
						break
					}
				}
			}
		}
	}

	// Validate all tracks have consistent structure
	for i, track := range img.Tracks {
		if track.Mode != 5 {
			t.Errorf("Track %d Mode = %d, expected 5", i, track.Mode)
		}
		if track.Ssize != 2 {
			t.Errorf("Track %d Ssize = %d, expected 2", i, track.Ssize)
		}
		if track.Nsec != 9 {
			t.Errorf("Track %d Nsec = %d, expected 9", i, track.Nsec)
		}
		// Validate each sector has correct data length
		for j, sector := range track.Sectors {
			if sector.Flag != 0 && len(sector.Data) != 512 {
				t.Errorf("Track %d Sector %d Data length = %d, expected 512", i, j, len(sector.Data))
			}
		}
	}

	// Summary statistics validation
	totalSectors := 0
	compressedSectors := 0
	deletedSectors := 0
	badSectors := 0
	for _, track := range img.Tracks {
		totalSectors += len(track.Sectors)
		for _, sector := range track.Sectors {
			if sector.Compressed {
				compressedSectors++
			}
			if sector.Deleted {
				deletedSectors++
			}
			if sector.Bad {
				badSectors++
			}
		}
	}
	expectedTotalSectors := 720
	expectedCompressedSectors := 717
	expectedDeletedSectors := 0
	expectedBadSectors := 0
	if totalSectors != expectedTotalSectors {
		t.Errorf("Total sectors = %d, expected %d", totalSectors, expectedTotalSectors)
	}
	if compressedSectors != expectedCompressedSectors {
		t.Errorf("Compressed sectors = %d, expected %d", compressedSectors, expectedCompressedSectors)
	}
	if deletedSectors != expectedDeletedSectors {
		t.Errorf("Deleted sectors = %d, expected %d", deletedSectors, expectedDeletedSectors)
	}
	if badSectors != expectedBadSectors {
		t.Errorf("Bad sectors = %d, expected %d", badSectors, expectedBadSectors)
	}
}

// Helper function to find minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
