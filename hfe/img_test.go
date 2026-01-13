package hfe

import (
	"testing"
)

// TestMfmWriterReaderRoundTrip tests the round-trip encoding/decoding:
// 1. Write bytes with mfmWriter
// 2. Get resulting byte array
// 3. Initialize mfmReader with it
// 4. Read bytes back
// 5. Verify they match, taking bit phase into account
// 6. Verify total number of bits in MFM output is 2x larger than data bytes written
func TestMfmWriterReaderRoundTrip(t *testing.T) {
	testCases := []struct {
		name        string
		inputBytes  []byte
		description string
	}{
		{
			name:        "SingleByte",
			inputBytes:  []byte{0x42},
			description: "Single byte test",
		},
		{
			name:        "SimplePattern",
			inputBytes:  []byte{0x00, 0xFF, 0xAA, 0x55},
			description: "Simple alternating patterns",
		},
		{
			name:        "MixedPattern",
			inputBytes:  []byte{0x12, 0x34, 0x56},
			description: "Mixed byte pattern",
		},
		{
			name:        "AllZeros",
			inputBytes:  []byte{0x00, 0x00, 0x00},
			description: "All zeros",
		},
		{
			name:        "AllOnes",
			inputBytes:  []byte{0xFF, 0xFF},
			description: "All ones",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Write bytes with mfmWriter
			writer := newMFMWriter()
			for _, b := range tc.inputBytes {
				writer.writeByte(b)
			}

			// Step 2: Get resulting byte array
			mfmOutput := writer.getData()

			// Step 3: Verify buffer size - each data byte becomes 16 bits (2 bytes) in MFM
			expectedMFMBytes := len(tc.inputBytes) * 2
			if len(mfmOutput) < expectedMFMBytes {
				t.Errorf("MFM output too small: got %d bytes, expected at least %d bytes", len(mfmOutput), expectedMFMBytes)
			}

			// Step 4: Verify total number of bits in MFM output is 2x larger than data bytes written
			// Each data byte has 8 bits, so N bytes = 8N bits
			// In MFM encoding, each data bit becomes 2 bits (clock + data), so 8N bits become 16N bits
			// 16N bits = 2N bytes
			expectedBits := len(tc.inputBytes) * 8 * 2
			actualBits := len(mfmOutput) * 8
			if actualBits < expectedBits {
				t.Errorf("MFM output has insufficient bits: got %d bits, expected at least %d bits", actualBits, expectedBits)
			}

			// Also verify we have exactly 2x bytes (accounting for potential partial byte at end)
			// The writer should produce exactly 2N bytes for N input bytes
			if len(mfmOutput) != expectedMFMBytes {
				// Allow for potential rounding, but should be very close
				if len(mfmOutput) < expectedMFMBytes || len(mfmOutput) > expectedMFMBytes+1 {
					t.Errorf("MFM output size mismatch: got %d bytes, expected %d bytes (each data byte = 2 MFM bytes)",
						len(mfmOutput), expectedMFMBytes)
				}
			}

			// Step 5: Initialize mfmReader with the output
			reader := newMFMReader(mfmOutput)

			// Step 6: Read bytes back
			readBytes := make([]byte, 0, len(tc.inputBytes))
			for i := 0; i < len(tc.inputBytes); i++ {
				b, err := reader.readByte()
				if err != nil {
					t.Fatalf("Failed to read byte %d: %v", i, err)
				}
				readBytes = append(readBytes, b)
			}

			// Step 7: Verify they match, taking bit phase into account
			// If they don't match immediately, try with a phase offset
			matched := false
			if len(readBytes) == len(tc.inputBytes) {
				match := true
				for i := 0; i < len(tc.inputBytes); i++ {
					if readBytes[i] != tc.inputBytes[i] {
						match = false
						break
					}
				}
				if match {
					matched = true
				}
			}

			// If not matched, try reading with a phase offset (skip one bit)
			if !matched {
				// Try reading from offset position 0 (instead of 1)
				// Create a reader that starts at bit position 0
				reader2 := &mfmReader{
					data:   mfmOutput,
					bitPos: 0, // Start at 0 instead of 1
				}
				readBytes2 := make([]byte, 0, len(tc.inputBytes))
				for i := 0; i < len(tc.inputBytes); i++ {
					b, err := reader2.readByte()
					if err != nil {
						break // Can't read more
					}
					readBytes2 = append(readBytes2, b)
				}

				if len(readBytes2) == len(tc.inputBytes) {
					match := true
					for i := 0; i < len(tc.inputBytes); i++ {
						if readBytes2[i] != tc.inputBytes[i] {
							match = false
							break
						}
					}
					if match {
						matched = true
						readBytes = readBytes2
					}
				}
			}

			if !matched {
				t.Errorf("Read bytes do not match written bytes")
				t.Errorf("Input bytes (%d): %v", len(tc.inputBytes), tc.inputBytes)
				t.Errorf("Read bytes (%d): %v", len(readBytes), readBytes)
				t.Errorf("MFM output (%d bytes): %v", len(mfmOutput), mfmOutput)
			} else {
				t.Logf("Successfully round-tripped %d bytes", len(tc.inputBytes))
			}

			// Additional verification: Check that the bit count is exactly 2x
			// Each input byte = 8 data bits
			// Each data bit in MFM = 2 bits (clock + data)
			// So N input bytes = 8N data bits = 16N MFM bits = 2N MFM bytes
			// The writer.bitPos should reflect this
			expectedBitPos := len(tc.inputBytes) * 8 * 2 // 8 bits per byte * 2 (clock+data) per bit
			if writer.bitPos != expectedBitPos {
				t.Errorf("Writer bitPos incorrect: got %d, expected %d (input bytes: %d, bits per byte: 8, MFM factor: 2)",
					writer.bitPos, expectedBitPos, len(tc.inputBytes))
			}
		})
	}
}

func TestCountSectorsIBMPC(t *testing.T) {
	// Find the test file
	sampleFile := findSampleFile(t, "fat12v1.hfe")
	if sampleFile == "" {
		return // Test was skipped
	}

	// Load the HFE file
	disk, err := ReadHFE(sampleFile)
	if err != nil {
		t.Fatalf("ReadHFE() error: %v", err)
	}

	// Verify we have at least one track
	if len(disk.Tracks) == 0 {
		t.Fatalf("ReadHFE() returned disk with no tracks")
	}

	// Extract side #0 data from track #0
	side0Data := disk.Tracks[0].Side0
	if len(side0Data) == 0 {
		t.Fatalf("Track 0 side 0 data is empty")
	}

	// Call countSectorsIBMPC() with the side 0 data from HFE file
	sectorCount := countSectorsIBMPC(side0Data)

	// Assert the result equals 18
	if sectorCount != 18 {
		t.Errorf("countSectorsIBMPC() = %d, expected 18", sectorCount)
	}
}

func TestEncodeTrackIBMPC_CountSectors(t *testing.T) {
	testCases := []struct {
		name            string
		sectorsPerTrack int
	}{
		{"9 sectors", 9},
		{"15 sectors", 15},
		{"18 sectors", 18},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create sectors filled with 0x0f (512 bytes each)
			sectors := make([][]byte, tc.sectorsPerTrack)
			for i := 0; i < tc.sectorsPerTrack; i++ {
				sectorData := make([]byte, 512)
				for j := range sectorData {
					sectorData[j] = 0x0f
				}
				sectors[i] = sectorData
			}

			// Encode track using encodeTrackIBMPC (cylinder 0, head 0)
			encodedTrack := encodeTrackIBMPC(sectors, 0, 0, tc.sectorsPerTrack)

			// Verify encoded track is not empty
			if len(encodedTrack) == 0 {
				t.Fatalf("encodeTrackIBMPC() returned empty track data")
			}

			// Count sectors using countSectorsIBMPC
			sectorCount := countSectorsIBMPC(encodedTrack)

			// Assert that the count matches the expected number
			if sectorCount != tc.sectorsPerTrack {
				t.Errorf("countSectorsIBMPC() = %d, expected %d", sectorCount, tc.sectorsPerTrack)
			}
		})
	}
}
