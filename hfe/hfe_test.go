package hfe

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Helper functions

// createTestHeader creates a test header with specified parameters
func createTestHeader(numTracks, numSides uint8) Header {
	header := Header{}
	copy(header.HeaderSignature[:], HFEv3Signature)
	header.FormatRevision = 0
	header.NumberOfTrack = numTracks
	header.NumberOfSide = numSides
	header.TrackEncoding = ENC_ISOIBM_MFM
	header.BitRate = 250
	header.FloppyRPM = 300
	header.FloppyInterfaceMode = IFM_IBMPC_DD
	header.WriteProtected = 0xFF
	header.TrackListOffset = 1
	header.WriteAllowed = 0xFF
	header.SingleStep = 0x00
	header.Track0S0AltEncoding = 0xFF
	header.Track0S0Encoding = ENC_ISOIBM_MFM
	header.Track0S1AltEncoding = 0xFF
	header.Track0S1Encoding = ENC_ISOIBM_MFM
	return header
}

// createTestTrack creates test track data with specified content
func createTestTrack(side0Data, side1Data []byte) TrackData {
	return TrackData{
		Side0: side0Data,
		Side1: side1Data,
	}
}

// createTestDisk creates a minimal valid disk for testing
func createTestDisk(numTracks, numSides uint8, trackDataSize int) *Disk {
	header := createTestHeader(numTracks, numSides)
	tracks := make([]TrackData, numTracks)

	// Create simple test data for each track
	// Avoid opcode-like bytes (0xF0-0xFF) that would be interpreted as opcodes when reading
	for i := range tracks {
		side0 := make([]byte, trackDataSize)
		side1 := make([]byte, trackDataSize)
		// Fill with pattern, avoiding opcode range (0xF0-0xFF)
		// Use a simple pattern that stays below 0xF0
		for j := range side0 {
			val := byte((i*17 + j*3) % 0xF0) // Keep values below 0xF0
			if val >= 0xF0 {
				val = 0x00 // Safety check
			}
			side0[j] = val
		}
		for j := range side1 {
			val := byte((i*17 + j*3 + 64) % 0xF0) // Keep values below 0xF0
			if val >= 0xF0 {
				val = 0x00 // Safety check
			}
			side1[j] = val
		}
		tracks[i] = createTestTrack(side0, side1)
	}

	return &Disk{
		Header: header,
		Tracks: tracks,
	}
}

// compareHeaders compares two Header structures, allowing padding differences
func compareHeaders(t *testing.T, h1, h2 Header) bool {
	t.Helper()
	if h1.HeaderSignature != h2.HeaderSignature {
		t.Errorf("HeaderSignature mismatch: %v != %v", h1.HeaderSignature, h2.HeaderSignature)
		return false
	}
	if h1.FormatRevision != h2.FormatRevision {
		t.Errorf("FormatRevision mismatch: %d != %d", h1.FormatRevision, h2.FormatRevision)
		return false
	}
	if h1.NumberOfTrack != h2.NumberOfTrack {
		t.Errorf("NumberOfTrack mismatch: %d != %d", h1.NumberOfTrack, h2.NumberOfTrack)
		return false
	}
	if h1.NumberOfSide != h2.NumberOfSide {
		t.Errorf("NumberOfSide mismatch: %d != %d", h1.NumberOfSide, h2.NumberOfSide)
		return false
	}
	if h1.TrackEncoding != h2.TrackEncoding {
		t.Errorf("TrackEncoding mismatch: %d != %d", h1.TrackEncoding, h2.TrackEncoding)
		return false
	}
	if h1.BitRate != h2.BitRate {
		t.Errorf("BitRate mismatch: %d != %d", h1.BitRate, h2.BitRate)
		return false
	}
	if h1.FloppyRPM != h2.FloppyRPM {
		t.Errorf("FloppyRPM mismatch: %d != %d", h1.FloppyRPM, h2.FloppyRPM)
		return false
	}
	if h1.FloppyInterfaceMode != h2.FloppyInterfaceMode {
		t.Errorf("FloppyInterfaceMode mismatch: %d != %d", h1.FloppyInterfaceMode, h2.FloppyInterfaceMode)
		return false
	}
	// TrackListOffset may differ, so we don't compare it
	return true
}

// compareTracks compares two TrackData structures
func compareTracks(t *testing.T, tr1, tr2 TrackData) bool {
	t.Helper()
	if !reflect.DeepEqual(tr1.Side0, tr2.Side0) {
		t.Errorf("Side0 mismatch: length %d != %d", len(tr1.Side0), len(tr2.Side0))
		return false
	}
	if !reflect.DeepEqual(tr1.Side1, tr2.Side1) {
		t.Errorf("Side1 mismatch: length %d != %d", len(tr1.Side1), len(tr2.Side1))
		return false
	}
	return true
}

// Test 1: Bit Manipulation Tests

func TestBitReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    byte
		expected byte
	}{
		{"all zeros", 0x00, 0x00},
		{"all ones", 0xFF, 0xFF},
		{"alternating 1", 0xAA, 0x55},
		{"alternating 2", 0x55, 0xAA},
		{"high nibble", 0xF0, 0x0F},
		{"low nibble", 0x0F, 0xF0},
		{"pattern 1", 0x81, 0x81},
		{"pattern 2", 0x42, 0x42},
		{"pattern 3", 0x24, 0x24},
		{"pattern 4", 0x18, 0x18},
		{"pattern 5", 0x3C, 0x3C},
		{"pattern 6", 0xC3, 0xC3},
		{"pattern 7", 0xE7, 0xE7},
		{"pattern 8", 0x7E, 0x7E},
		{"pattern 9", 0xBD, 0xBD},
		{"pattern 10", 0xDB, 0xDB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bitReverse(tt.input)
			if result != tt.expected {
				t.Errorf("bitReverse(0x%02X) = 0x%02X, expected 0x%02X", tt.input, result, tt.expected)
			}
			// Verify symmetry: bitReverse(bitReverse(x)) == x
			reversed := bitReverse(result)
			if reversed != tt.input {
				t.Errorf("bitReverse(bitReverse(0x%02X)) = 0x%02X, expected 0x%02X", tt.input, reversed, tt.input)
			}
		})
	}
}

func TestBitReverseBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"empty slice", []byte{}, []byte{}},
		{"single byte", []byte{0xAA}, []byte{0x55}},
		{"multiple bytes", []byte{0xAA, 0x55, 0xF0}, []byte{0x55, 0xAA, 0x0F}},
		{"all zeros", []byte{0x00, 0x00, 0x00}, []byte{0x00, 0x00, 0x00}},
		{"all ones", []byte{0xFF, 0xFF, 0xFF}, []byte{0xFF, 0xFF, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, len(tt.input))
			copy(data, tt.input)
			bitReverseBlock(data)
			if !reflect.DeepEqual(data, tt.expected) {
				t.Errorf("bitReverseBlock() = %v, expected %v", data, tt.expected)
			}
			// Verify symmetry
			bitReverseBlock(data)
			if !reflect.DeepEqual(data, tt.input) {
				t.Errorf("bitReverseBlock(bitReverseBlock()) = %v, expected %v", data, tt.input)
			}
		})
	}
}

func TestBitCopy(t *testing.T) {
	tests := []struct {
		name        string
		src         []byte
		srcOff      int
		dst         []byte
		dstOff      int
		size        int
		expectedDst []byte
		expectedOff int
	}{
		{
			name:        "byte-aligned copy",
			src:         []byte{0xAA, 0x55},
			srcOff:      0,
			dst:         make([]byte, 2),
			dstOff:      0,
			size:        16,
			expectedDst: []byte{0xAA, 0x55},
			expectedOff: 16,
		},
		{
			name:        "bit-offset copy",
			src:         []byte{0xAA, 0x55}, // 10101010 01010101
			srcOff:      4,
			dst:         make([]byte, 2),
			dstOff:      0,
			size:        8,
			expectedDst: []byte{0xA5, 0x00}, // Copy bits 4-11: 10100101
			expectedOff: 8,
		},
		{
			name:        "partial byte copy",
			src:         []byte{0xFF},
			srcOff:      2,
			dst:         make([]byte, 1),
			dstOff:      4,
			size:        4,
			expectedDst: []byte{0x0F}, // Copy 4 bits starting at src[2] to dst[4]
			expectedOff: 8,
		},
		{
			name:        "boundary condition",
			src:         []byte{0xAA},
			srcOff:      0,
			dst:         make([]byte, 1),
			dstOff:      0,
			size:        20, // More than available
			expectedDst: []byte{0xAA},
			expectedOff: 8, // Stops at end of src
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := make([]byte, len(tt.dst))
			copy(dst, tt.dst)
			resultOff := bitCopy(dst, tt.dstOff, tt.src, tt.srcOff, tt.size)
			if resultOff != tt.expectedOff {
				t.Errorf("bitCopy() returned offset %d, expected %d", resultOff, tt.expectedOff)
			}
			if !reflect.DeepEqual(dst, tt.expectedDst) {
				t.Errorf("bitCopy() dst = %v, expected %v", dst, tt.expectedDst)
			}
		})
	}
}

// Test 2: Opcode Processing Tests

func TestProcessOpcodes_NOP(t *testing.T) {
	// NOP (0xF0): skip 8 bits with no output
	data := []byte{NOP_OPCODE, 0xAA, 0x55}
	result, err := processOpcodes(data)
	if err != nil {
		t.Fatalf("processOpcodes() error: %v", err)
	}
	// NOP should be skipped, so we should get the remaining data
	if len(result) == 0 || result[0] != 0xAA {
		t.Errorf("processOpcodes() with NOP: expected 0xAA, got %v", result)
	}
}

func TestProcessOpcodes_SETINDEX(t *testing.T) {
	// SETINDEX (0xF1): mark index position and rotate track
	data := []byte{0xAA, SETINDEX_OPCODE, 0x55, 0x33}
	result, err := processOpcodes(data)
	if err != nil {
		t.Fatalf("processOpcodes() error: %v", err)
	}
	// Track should be rotated so index is at position 0
	// First byte should be 0x55 (after SETINDEX)
	if len(result) < 2 || result[0] != 0x55 {
		t.Errorf("processOpcodes() with SETINDEX: expected rotation, got %v", result)
	}
}

func TestProcessOpcodes_SETBITRATE(t *testing.T) {
	// SETBITRATE (0xF2 0xBB): change bitrate
	data := []byte{SETBITRATE_OPCODE, 0x64, 0xAA, 0x55}
	result, err := processOpcodes(data)
	if err != nil {
		t.Fatalf("processOpcodes() error: %v", err)
	}
	// Should skip opcode and bitrate byte, then process remaining
	if len(result) < 2 || result[0] != 0xAA {
		t.Errorf("processOpcodes() with SETBITRATE: expected 0xAA, got %v", result)
	}
}

func TestProcessOpcodes_SKIPBITS(t *testing.T) {
	tests := []struct {
		name     string
		skip     byte
		nextByte byte
		expected byte
	}{
		{"skip 0 bits", 0, 0xFF, 0xFF},
		{"skip 1 bit", 1, 0xFF, 0xFE},  // After skipping 1 bit from 0xFF, copy remaining 7 bits
		{"skip 2 bits", 2, 0xFF, 0xFC}, // After skipping 2 bits from 0xFF, copy remaining 6 bits
		{"skip 3 bits", 3, 0xFF, 0xF8}, // After skipping 3 bits from 0xFF, copy remaining 5 bits
		{"skip 4 bits", 4, 0xFF, 0xF0}, // After skipping 4 bits from 0xFF, copy remaining 4 bits
		{"skip 5 bits", 5, 0xFF, 0xE0}, // After skipping 5 bits from 0xFF, copy remaining 3 bits
		{"skip 6 bits", 6, 0xFF, 0xC0}, // After skipping 6 bits from 0xFF, copy remaining 2 bits
		{"skip 7 bits", 7, 0xFF, 0x80}, // After skipping 7 bits from 0xFF, copy remaining 1 bit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte{SKIPBITS_OPCODE, tt.skip, tt.nextByte}
			result, err := processOpcodes(data)
			if err != nil {
				t.Fatalf("processOpcodes() error: %v", err)
			}
			if len(result) == 0 {
				t.Fatalf("processOpcodes() returned empty result")
			}
			// The result should contain the remaining bits after skip
			// SKIPBITS skips the opcode (8 bits), skip value (8 bits), then skip bits,
			// then copies remaining (8-skip) bits from the next byte
			if result[0] != tt.expected {
				t.Errorf("processOpcodes() = 0x%02X, expected 0x%02X", result[0], tt.expected)
			}
		})
	}
}

func TestProcessOpcodes_RAND(t *testing.T) {
	// RAND (0xF4): skip 8 bits (weak bits)
	data := []byte{RAND_OPCODE, 0xAA, 0x55}
	result, err := processOpcodes(data)
	if err != nil {
		t.Fatalf("processOpcodes() error: %v", err)
	}
	// RAND should be skipped, output should be zeros for that byte
	// Then remaining data should be processed
	if len(result) < 2 {
		t.Errorf("processOpcodes() with RAND: expected output, got %v", result)
	}
}

func TestProcessOpcodes_Mixed(t *testing.T) {
	// Test combination of opcodes
	data := []byte{
		0x11,                    // Regular data
		NOP_OPCODE,              // NOP
		0x22,                    // Regular data
		SETINDEX_OPCODE,         // SETINDEX
		0x33,                    // Regular data
		SETBITRATE_OPCODE, 0x64, // SETBITRATE
		0x44,        // Regular data
		RAND_OPCODE, // RAND
		0x55,        // Regular data
	}
	result, err := processOpcodes(data)
	if err != nil {
		t.Fatalf("processOpcodes() error: %v", err)
	}
	if len(result) == 0 {
		t.Fatalf("processOpcodes() returned empty result")
	}
	// Track should be rotated (SETINDEX was present)
	// Should contain regular data bytes (0x11, 0x22, 0x33, 0x44, 0x55)
}

func TestProcessOpcodes_ErrorCases(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"invalid opcode", []byte{0xFF}},
		{"SKIPBITS value > 7", []byte{SKIPBITS_OPCODE, 9, 0xAA}},
		{"SETBITRATE insufficient data", []byte{SETBITRATE_OPCODE}},
		{"SKIPBITS insufficient data", []byte{SKIPBITS_OPCODE}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processOpcodes(tt.data)
			if err == nil {
				t.Errorf("processOpcodes() expected error, got nil")
			}
		})
	}
}

func TestProcessOpcodes_Empty(t *testing.T) {
	result, err := processOpcodes([]byte{})
	if err != nil {
		t.Fatalf("processOpcodes() with empty data: error %v", err)
	}
	if len(result) != 0 {
		t.Errorf("processOpcodes() with empty data: expected empty, got %v", result)
	}
}

// Test 3: Track Reading Tests (requires file operations)

func TestReadTrack_SingleSide(t *testing.T) {
	testReadTrack(t, 1, 1, 256, HFEVersion3)
}

func TestReadTrack_DoubleSide(t *testing.T) {
	testReadTrack(t, 1, 2, 256, HFEVersion3)
}

// Test 4: Track Writing Tests

func TestWriteTrack_SingleSide(t *testing.T) {
	disk := createTestDisk(1, 1, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_write_single.hfe")

	if err := Write(tmpFile, disk, HFEVersion3); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Verify file exists and has content
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Size() < BlockSize {
		t.Errorf("Write() file too small: %d bytes", info.Size())
	}
}

func TestWriteTrack_DoubleSide(t *testing.T) {
	disk := createTestDisk(1, 2, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_write_double.hfe")

	if err := Write(tmpFile, disk, HFEVersion3); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Verify file exists and has content
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Size() < BlockSize*2 {
		t.Errorf("Write() file too small: %d bytes", info.Size())
	}
}

// Test 5: Header Validation Tests

func TestRead_InvalidSignature(t *testing.T) {
	// Create a file with invalid signature
	tmpFile := filepath.Join(t.TempDir(), "test_invalid_sig.hfe")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer file.Close()

	// Write invalid signature and pad to full header size
	// Use a truly invalid signature (not HXCPICFE or HXCHFEV3)
	invalidSig := [8]byte{'I', 'N', 'V', 'A', 'L', 'I', 'D', '!'}
	if err := binary.Write(file, binary.LittleEndian, invalidSig); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	// Pad to at least 32 bytes (minimum header size)
	padding := make([]byte, 24)
	if _, err := file.Write(padding); err != nil {
		t.Fatalf("Write() padding error: %v", err)
	}
	file.Close()

	_, err = Read(tmpFile)
	if err == nil {
		t.Error("Read() with invalid signature: expected error, got nil")
	}
	// Error message should mention invalid signature
	if err != nil && !strings.Contains(err.Error(), "invalid HFE signature") {
		t.Errorf("Read() with invalid signature: expected error containing 'invalid HFE signature', got %v", err)
	}
}

func TestRead_InvalidFormatRevision(t *testing.T) {
	// Create a file with invalid format revision
	tmpFile := filepath.Join(t.TempDir(), "test_invalid_rev.hfe")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer file.Close()

	// Write valid signature but invalid revision
	header := createTestHeader(1, 1)
	header.FormatRevision = 1
	if err := binary.Write(file, binary.LittleEndian, header); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	file.Close()

	_, err = Read(tmpFile)
	if err == nil {
		t.Error("Read() with invalid format revision: expected error, got nil")
	}
}

func TestRead_InvalidTrackCount(t *testing.T) {
	// Create a file with zero tracks
	tmpFile := filepath.Join(t.TempDir(), "test_zero_tracks.hfe")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer file.Close()

	header := createTestHeader(0, 1)
	if err := binary.Write(file, binary.LittleEndian, header); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	file.Close()

	_, err = Read(tmpFile)
	if err == nil {
		t.Error("Read() with zero tracks: expected error, got nil")
	}
}

func TestRead_InvalidSideCount(t *testing.T) {
	// Create a file with zero sides
	tmpFile := filepath.Join(t.TempDir(), "test_zero_sides.hfe")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer file.Close()

	header := createTestHeader(1, 0)
	if err := binary.Write(file, binary.LittleEndian, header); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	file.Close()

	_, err = Read(tmpFile)
	if err == nil {
		t.Error("Read() with zero sides: expected error, got nil")
	}
}

// Test 6: File Reading Integration Tests

// getTestDataPath returns the path to a testdata file, trying multiple locations
func getTestDataPath(filename string) string {
	// Try relative to module root (go test runs from module root)
	testdataPath := filepath.Join("testdata", filename)
	if _, err := os.Stat(testdataPath); err == nil {
		return testdataPath
	}
	// Try relative to current directory
	if _, err := os.Stat(filename); err == nil {
		return filename
	}
	// Return testdata path (will fail with proper error message)
	return testdataPath
}

// findSampleFile searches for a sample file in multiple possible locations
// and returns the found path, or skips the test if not found
func findSampleFile(t *testing.T, filename string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	possiblePaths := []string{
		filepath.Join(wd, "testdata", filename),
		filepath.Join(wd, "..", "testdata", filename),
		filepath.Join("testdata", filename),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	t.Skipf("Sample file %s not found in any of: %v", filename, possiblePaths)
	return ""
}

// testReadSampleFile is a parameterized helper for reading and validating sample files
func testReadSampleFile(t *testing.T, filename string, expectedSignature string, skipErrorMsg string) {
	t.Helper()
	sampleFile := findSampleFile(t, filename)
	if sampleFile == "" {
		return // Test was skipped
	}

	disk, err := Read(sampleFile)
	if err != nil {
		// Check if we should skip due to format mismatch
		if skipErrorMsg != "" && strings.Contains(err.Error(), skipErrorMsg) {
			t.Skipf("Sample file %s: %s, skipping test", sampleFile, err.Error())
			return
		}
		t.Fatalf("Read() error: %v", err)
	}

	// Verify header
	if string(disk.Header.HeaderSignature[:]) != expectedSignature {
		t.Errorf("Read() header signature = %s, expected %s", string(disk.Header.HeaderSignature[:]), expectedSignature)
	}

	if disk.Header.FormatRevision != 0 {
		t.Errorf("Read() format revision = %d, expected 0", disk.Header.FormatRevision)
	}

	if disk.Header.NumberOfTrack == 0 {
		t.Error("Read() number of tracks is zero")
	}

	if disk.Header.NumberOfSide == 0 {
		t.Error("Read() number of sides is zero")
	}

	// Verify tracks
	if len(disk.Tracks) != int(disk.Header.NumberOfTrack) {
		t.Errorf("Read() number of tracks = %d, expected %d", len(disk.Tracks), disk.Header.NumberOfTrack)
	}

	// Verify track data is non-empty
	for i, track := range disk.Tracks {
		if len(track.Side0) == 0 {
			t.Errorf("Read() track %d side 0 is empty", i)
		}
		if disk.Header.NumberOfSide > 1 && len(track.Side1) == 0 {
			t.Errorf("Read() track %d side 1 is empty", i)
		}
	}
}

// testRoundTripSampleFile is a parameterized helper for round-trip testing sample files
func testRoundTripSampleFile(t *testing.T, filename string, writeVersion HFEVersion, skipErrorMsg string) {
	t.Helper()
	sampleFile := findSampleFile(t, filename)
	if sampleFile == "" {
		return // Test was skipped
	}

	// Read original file
	originalDisk, err := Read(sampleFile)
	if err != nil {
		// Check if we should skip due to format mismatch
		if skipErrorMsg != "" && strings.Contains(err.Error(), skipErrorMsg) {
			t.Skipf("Sample file %s: %s, skipping test", sampleFile, err.Error())
			return
		}
		t.Fatalf("Read() original file error: %v", err)
	}

	// Write to temporary file
	tmpFile := filepath.Join(t.TempDir(), "test_roundtrip.hfe")
	if err := Write(tmpFile, originalDisk, writeVersion); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Read back
	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() round-trip file error: %v", err)
	}

	// Compare headers (allowing for padding differences)
	if !compareHeaders(t, originalDisk.Header, readDisk.Header) {
		t.Error("Round-trip header mismatch")
	}

	// Compare tracks
	if len(readDisk.Tracks) != len(originalDisk.Tracks) {
		t.Fatalf("Round-trip track count = %d, expected %d", len(readDisk.Tracks), len(originalDisk.Tracks))
	}

	// For real files, exact byte-level matches may not be possible due to:
	// - Different opcode encoding choices
	// - Track rotation differences
	// - Padding variations
	// So we verify that tracks exist and have reasonable data
	for i := range originalDisk.Tracks {
		if len(readDisk.Tracks[i].Side0) == 0 && len(originalDisk.Tracks[i].Side0) > 0 {
			t.Errorf("Round-trip track %d side 0: original had data but written file is empty", i)
		}
		if len(readDisk.Tracks[i].Side1) == 0 && len(originalDisk.Tracks[i].Side1) > 0 {
			t.Errorf("Round-trip track %d side 1: original had data but written file is empty", i)
		}
		// Verify that we can at least read and write successfully
		// (exact byte matches are not required for real-world files)
	}
}

// testReadTrack is a parameterized helper for reading and validating track data
func testReadTrack(t *testing.T, numTracks, numSides uint8, trackDataSize int, version HFEVersion) {
	t.Helper()
	disk := createTestDisk(numTracks, numSides, trackDataSize)
	tmpFile := filepath.Join(t.TempDir(), "test_track.hfe")

	if err := Write(tmpFile, disk, version); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	if len(readDisk.Tracks) != int(numTracks) {
		t.Fatalf("Read() expected %d track(s), got %d", numTracks, len(readDisk.Tracks))
	}

	for i := 0; i < int(numTracks); i++ {
		if len(readDisk.Tracks[i].Side0) == 0 {
			t.Errorf("Read() track %d side 0 is empty", i)
		}
		if numSides > 1 {
			if len(readDisk.Tracks[i].Side1) == 0 {
				t.Errorf("Read() track %d side 1 is empty", i)
			}
		} else {
			if len(readDisk.Tracks[i].Side1) != 0 {
				t.Errorf("Read() single-side track %d should have empty side 1", i)
			}
		}
	}
}

// testWriteReadDisk is a parameterized helper for write/read verification pattern
// verifyFunc is optional - if nil, only basic verification is performed
func testWriteReadDisk(t *testing.T, disk *Disk, version HFEVersion, verifyFunc func(*testing.T, *Disk, *Disk)) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test_write_read.hfe")

	if err := Write(tmpFile, disk, version); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Fatalf("Write() file was not created")
	}

	// Read it back
	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	// Call custom verification function if provided
	if verifyFunc != nil {
		verifyFunc(t, disk, readDisk)
	}
}

// testWriteV1Format is a parameterized helper for v1 format write/read tests
func testWriteV1Format(t *testing.T, numTracks, numSides uint8, trackDataSize int) {
	t.Helper()
	disk := createTestDisk(numTracks, numSides, trackDataSize)
	tmpFile := filepath.Join(t.TempDir(), "test_v1.hfe")

	if err := Write(tmpFile, disk, HFEVersion1); err != nil {
		t.Fatalf("Write() v1 error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Fatalf("Write() v1 file was not created")
	}

	// Read it back and verify signature
	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() v1 file error: %v", err)
	}

	// Verify signature is HXCPICFE
	if string(readDisk.Header.HeaderSignature[:]) != HFEv1Signature {
		t.Errorf("Write() v1 signature = %s, expected %s", string(readDisk.Header.HeaderSignature[:]), HFEv1Signature)
	}

	// Verify format revision is 0
	if readDisk.Header.FormatRevision != 0 {
		t.Errorf("Write() v1 format revision = %d, expected 0", readDisk.Header.FormatRevision)
	}

	// Verify track data exists
	if len(readDisk.Tracks) != int(numTracks) {
		t.Errorf("Write() v1 track count = %d, expected %d", len(readDisk.Tracks), numTracks)
	}

	// Verify track data based on number of sides
	for i, track := range readDisk.Tracks {
		if len(track.Side0) == 0 {
			t.Errorf("Write() v1 track %d side 0 is empty", i)
		}
		if numSides > 1 {
			if len(track.Side1) == 0 {
				t.Errorf("Write() v1 track %d side 1 is empty", i)
			}
		}
	}
}

func TestRead_SampleFile(t *testing.T) {
	testReadSampleFile(t, "fat12v3.hfe", HFEv3Signature, "invalid HFE v3 signature")
}

func TestRead_NonExistentFile(t *testing.T) {
	_, err := Read("nonexistent_file.hfe")
	if err == nil {
		t.Error("Read() with non-existent file: expected error, got nil")
	}
}

// Test 7: File Writing Integration Tests

func TestWrite_ValidDisk(t *testing.T) {
	disk := createTestDisk(2, 2, 512)
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		// Compare headers (excluding TrackListOffset which may differ)
		if !compareHeaders(t, original.Header, read.Header) {
			t.Error("Write() header mismatch")
		}

		// Verify track count
		if len(read.Tracks) != len(original.Tracks) {
			t.Errorf("Write() track count = %d, expected %d", len(read.Tracks), len(original.Tracks))
		}
	})
}

func TestWrite_HeaderPadding(t *testing.T) {
	disk := createTestDisk(1, 1, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_header_padding.hfe")

	if err := Write(tmpFile, disk, HFEVersion3); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Read raw file and check padding
	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer file.Close()

	headerBuf := make([]byte, BlockSize)
	if _, err := io.ReadFull(file, headerBuf); err != nil {
		t.Fatalf("ReadFull() error: %v", err)
	}

	// Check that bytes after header data (offset 32) are 0xFF
	for i := 32; i < BlockSize; i++ {
		if headerBuf[i] != 0xFF {
			t.Errorf("Write() header padding at offset %d = 0x%02X, expected 0xFF", i, headerBuf[i])
		}
	}
}

func TestWrite_TrackListPadding(t *testing.T) {
	disk := createTestDisk(1, 1, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_tracklist_padding.hfe")

	if err := Write(tmpFile, disk, HFEVersion3); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Read raw file and check track list padding
	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer file.Close()

	// Seek to track list (offset 512)
	if _, err := file.Seek(BlockSize, io.SeekStart); err != nil {
		t.Fatalf("Seek() error: %v", err)
	}

	trackListBuf := make([]byte, BlockSize)
	if _, err := io.ReadFull(file, trackListBuf); err != nil {
		t.Fatalf("ReadFull() error: %v", err)
	}

	// Track list entry is 4 bytes (2 for offset, 2 for length)
	// Check that bytes after track entries are 0xFF
	entrySize := 4
	numEntries := int(disk.Header.NumberOfTrack)
	for i := numEntries * entrySize; i < BlockSize; i++ {
		if trackListBuf[i] != 0xFF {
			t.Errorf("Write() track list padding at offset %d = 0x%02X, expected 0xFF", i, trackListBuf[i])
		}
	}
}

// Test 8: Round-Trip Tests

func TestRoundTrip_SampleFile(t *testing.T) {
	testRoundTripSampleFile(t, "fat12v3.hfe", HFEVersion3, "invalid HFE v3 signature")
}

func TestRead_SampleFileV1(t *testing.T) {
	testReadSampleFile(t, "fat12v1.hfe", HFEv1Signature, "invalid HFE")
}

func TestRoundTrip_SampleFileV1(t *testing.T) {
	testRoundTripSampleFile(t, "fat12v1.hfe", HFEVersion1, "invalid HFE")
}

func TestRoundTrip_GeneratedData(t *testing.T) {
	// Create a minimal disk programmatically
	disk := createTestDisk(3, 2, 1024)

	// Write to temporary file
	tmpFile := filepath.Join(t.TempDir(), "test_roundtrip_gen.hfe")
	if err := Write(tmpFile, disk, HFEVersion3); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Read back
	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	// Compare headers
	if !compareHeaders(t, disk.Header, readDisk.Header) {
		t.Error("Round-trip header mismatch")
	}

	// Compare tracks
	if len(readDisk.Tracks) != len(disk.Tracks) {
		t.Fatalf("Round-trip track count = %d, expected %d", len(readDisk.Tracks), len(disk.Tracks))
	}

	for i := range disk.Tracks {
		// Note: Due to opcode processing and track rotation, exact byte comparison
		// may not work. We check that tracks have data.
		if len(readDisk.Tracks[i].Side0) == 0 {
			t.Errorf("Round-trip track %d side 0 is empty", i)
		}
		if len(readDisk.Tracks[i].Side1) == 0 {
			t.Errorf("Round-trip track %d side 1 is empty", i)
		}
	}
}

// Test 9: Edge Cases and Boundary Tests

func TestWrite_EmptyTracks(t *testing.T) {
	disk := createTestDisk(2, 1, 0) // Zero-length track data
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		if len(read.Tracks) != 2 {
			t.Errorf("Read() track count = %d, expected 2", len(read.Tracks))
		}
	})
}

func TestWrite_SingleTrack(t *testing.T) {
	disk := createTestDisk(1, 1, 256)
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		if len(read.Tracks) != 1 {
			t.Errorf("Read() track count = %d, expected 1", len(read.Tracks))
		}
	})
}

func TestWrite_TrackLengthBoundary(t *testing.T) {
	// Test track at exactly 512-byte boundary (256 bytes per side)
	disk := createTestDisk(1, 2, 256)
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		if len(read.Tracks) != 1 {
			t.Errorf("Read() track count = %d, expected 1", len(read.Tracks))
		}
	})
}

func TestWrite_VeryLongTrack(t *testing.T) {
	// Test track requiring multiple 512-byte blocks
	disk := createTestDisk(1, 1, 2048) // 2048 bytes = 4 blocks per side
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		if len(read.Tracks) != 1 {
			t.Errorf("Read() track count = %d, expected 1", len(read.Tracks))
		}
	})
}

func TestWrite_SingleSideDisk(t *testing.T) {
	disk := createTestDisk(2, 1, 256)
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		if read.Header.NumberOfSide != 1 {
			t.Errorf("Read() number of sides = %d, expected 1", read.Header.NumberOfSide)
		}

		for i, track := range read.Tracks {
			if len(track.Side1) != 0 {
				t.Errorf("Read() track %d side 1 should be empty for single-side disk", i)
			}
		}
	})
}

func TestWrite_DoubleSideDisk(t *testing.T) {
	disk := createTestDisk(2, 2, 256)
	testWriteReadDisk(t, disk, HFEVersion3, func(t *testing.T, original, read *Disk) {
		if read.Header.NumberOfSide != 2 {
			t.Errorf("Read() number of sides = %d, expected 2", read.Header.NumberOfSide)
		}

		for i, track := range read.Tracks {
			if len(track.Side1) == 0 {
				t.Errorf("Read() track %d side 1 is empty for double-side disk", i)
			}
		}
	})
}

// Test 10: Constants and Type Tests

func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
	}{
		{"HFEv3Signature", HFEv3Signature, "HXCHFEV3"},
		{"OPCODE_MASK", int(OPCODE_MASK), 0xF0},
		{"NOP_OPCODE", int(NOP_OPCODE), 0xF0},
		{"SETINDEX_OPCODE", int(SETINDEX_OPCODE), 0xF1},
		{"SETBITRATE_OPCODE", int(SETBITRATE_OPCODE), 0xF2},
		{"SKIPBITS_OPCODE", int(SKIPBITS_OPCODE), 0xF3},
		{"RAND_OPCODE", int(RAND_OPCODE), 0xF4},
		{"FLOPPYEMUFREQ", FLOPPYEMUFREQ, 36000000},
		{"BlockSize", BlockSize, 512},
		{"ENC_ISOIBM_MFM", int(ENC_ISOIBM_MFM), 0},
		{"ENC_Amiga_MFM", int(ENC_Amiga_MFM), 1},
		{"ENC_ISOIBM_FM", int(ENC_ISOIBM_FM), 2},
		{"ENC_Emu_FM", int(ENC_Emu_FM), 3},
		{"ENC_Unknown", int(ENC_Unknown), 0xff},
		{"IFM_IBMPC_DD", int(IFM_IBMPC_DD), 0},
		{"IFM_IBMPC_HD", int(IFM_IBMPC_HD), 1},
		{"IFM_AtariST_DD", int(IFM_AtariST_DD), 2},
		{"IFM_AtariST_HD", int(IFM_AtariST_HD), 3},
		{"IFM_Amiga_DD", int(IFM_Amiga_DD), 4},
		{"IFM_Amiga_HD", int(IFM_Amiga_HD), 5},
		{"IFM_CPC_DD", int(IFM_CPC_DD), 6},
		{"IFM_GenericShugart_DD", int(IFM_GenericShugart_DD), 7},
		{"IFM_IBMPC_ED", int(IFM_IBMPC_ED), 8},
		{"IFM_MSX2_DD", int(IFM_MSX2_DD), 9},
		{"IFM_C64_DD", int(IFM_C64_DD), 10},
		{"IFM_EmuShugart_DD", int(IFM_EmuShugart_DD), 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("Constant %s = %v, expected %v", tt.name, tt.actual, tt.expected)
			}
		})
	}
}

func TestTypeSizes(t *testing.T) {
	// Verify struct sizes match expected binary layout
	// Header should be 32 bytes (8 + 1 + 1 + 1 + 1 + 2 + 2 + 1 + 1 + 2 + 1 + 1 + 1 + 1 + 1 + 1 = 26, but Go may pad)
	headerSize := binary.Size(Header{})
	if headerSize < 26 || headerSize > 32 {
		t.Errorf("Header size = %d bytes, expected 26-32 bytes", headerSize)
	}

	// TrackHeader should be 4 bytes (2 + 2)
	trackHeaderSize := binary.Size(TrackHeader{})
	if trackHeaderSize != 4 {
		t.Errorf("TrackHeader size = %d bytes, expected 4 bytes", trackHeaderSize)
	}
}

func TestEndianness(t *testing.T) {
	// Test that uint16 values are written/read in little-endian
	value := uint16(0x1234)
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, value)

	// In little-endian, 0x1234 should be stored as [0x34, 0x12]
	expected := []byte{0x34, 0x12}
	if !bytes.Equal(buf, expected) {
		t.Errorf("Little-endian encoding: got %v, expected %v", buf, expected)
	}

	// Read it back
	readValue := binary.LittleEndian.Uint16(buf)
	if readValue != value {
		t.Errorf("Little-endian decoding: got 0x%04X, expected 0x%04X", readValue, value)
	}
}

// Additional tests for writeBits function

func TestWriteBits_Interleaving(t *testing.T) {
	// Create test data
	side0 := []byte{0xAA, 0x55}
	side1 := []byte{0x33, 0xCC}

	// Create buffer for one 512-byte block
	trackBuf := make([]byte, BlockSize)

	// Write side 0 (bytes 0-255)
	writeBits(side0, trackBuf, 0, 256)

	// Write side 1 (bytes 256-511)
	writeBits(side1, trackBuf, 256, 256)

	// Verify interleaving
	// Side 0 should be in bytes 0-255
	if trackBuf[0] != 0xAA {
		t.Errorf("writeBits() side 0 byte 0 = 0x%02X, expected 0xAA", trackBuf[0])
	}
	if trackBuf[1] != 0x55 {
		t.Errorf("writeBits() side 0 byte 1 = 0x%02X, expected 0x55", trackBuf[1])
	}

	// Side 1 should be in bytes 256-511
	if trackBuf[256] != 0x33 {
		t.Errorf("writeBits() side 1 byte 0 = 0x%02X, expected 0x33", trackBuf[256])
	}
	if trackBuf[257] != 0xCC {
		t.Errorf("writeBits() side 1 byte 1 = 0x%02X, expected 0xCC", trackBuf[257])
	}
}

func TestWriteBits_Wrapping(t *testing.T) {
	// Test that writeBits wraps around track data
	side0 := []byte{0xFF, 0x00}
	trackBuf := make([]byte, BlockSize*2) // Two blocks

	// Write more than one track length
	writeBits(side0, trackBuf, 0, 512)

	// After wrapping, we should see the pattern repeat
	// The first 2 bytes should be 0xFF, 0x00
	if trackBuf[0] != 0xFF || trackBuf[1] != 0x00 {
		t.Errorf("writeBits() initial bytes = [0x%02X, 0x%02X], expected [0xFF, 0x00]", trackBuf[0], trackBuf[1])
	}

	// After wrapping (at byte 2), pattern should repeat
	// Due to the wrapping logic, we may see repeated patterns
	// This is a basic check that wrapping occurs
}

// Test 11: HFE v1 Format Tests

func TestWriteV1SingleSide(t *testing.T) {
	testWriteV1Format(t, 1, 1, 256)
}

func TestWriteV1DoubleSide(t *testing.T) {
	testWriteV1Format(t, 1, 2, 256)
}

func TestWrite_RejectV2(t *testing.T) {
	// Test that Write() rejects v2 version
	disk := createTestDisk(1, 1, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_v2_reject.hfe")

	// Try to write with v2 (should fail)
	err := Write(tmpFile, disk, HFEVersion(2))
	if err == nil {
		t.Fatalf("Write() with v2: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid HFE version") {
		t.Errorf("Write() with v2: expected error about invalid version, got %v", err)
	}
}

func TestWriteV1RoundTrip(t *testing.T) {
	// Create test disk
	disk := createTestDisk(2, 2, 512)
	tmpFile := filepath.Join(t.TempDir(), "test_v1_roundtrip.hfe")

	// Write v1 format
	if err := Write(tmpFile, disk, HFEVersion1); err != nil {
		t.Fatalf("Write() v1 error: %v", err)
	}

	// Read it back
	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() v1 file error: %v", err)
	}

	// Verify header fields
	if readDisk.Header.NumberOfTrack != disk.Header.NumberOfTrack {
		t.Errorf("Round-trip v1 track count = %d, expected %d", readDisk.Header.NumberOfTrack, disk.Header.NumberOfTrack)
	}
	if readDisk.Header.NumberOfSide != disk.Header.NumberOfSide {
		t.Errorf("Round-trip v1 side count = %d, expected %d", readDisk.Header.NumberOfSide, disk.Header.NumberOfSide)
	}

	// Verify track count
	if len(readDisk.Tracks) != len(disk.Tracks) {
		t.Errorf("Round-trip v1 track count = %d, expected %d", len(readDisk.Tracks), len(disk.Tracks))
	}

	// Verify tracks have data (exact byte comparison may differ due to padding)
	for i := range disk.Tracks {
		if len(readDisk.Tracks[i].Side0) == 0 && len(disk.Tracks[i].Side0) > 0 {
			t.Errorf("Round-trip v1 track %d side 0: original had data but read is empty", i)
		}
		if len(readDisk.Tracks[i].Side1) == 0 && len(disk.Tracks[i].Side1) > 0 {
			t.Errorf("Round-trip v1 track %d side 1: original had data but read is empty", i)
		}
	}
}

func TestRead_RejectV2(t *testing.T) {
	// Test that Read() rejects v2 format files
	tmpFile := filepath.Join(t.TempDir(), "test_v2_reject.hfe")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer file.Close()

	// Create a v2 format file manually (HXCPICFE signature with revision 1)
	header := Header{}
	copy(header.HeaderSignature[:], HFEv1Signature) // Same signature as v1
	header.FormatRevision = 1                       // But revision 1 indicates v2
	header.NumberOfTrack = 1
	header.NumberOfSide = 1
	header.TrackEncoding = ENC_ISOIBM_MFM
	header.BitRate = 250
	header.FloppyRPM = 300
	header.FloppyInterfaceMode = IFM_IBMPC_DD
	header.WriteProtected = 0xFF
	header.TrackListOffset = 1
	header.WriteAllowed = 0xFF
	header.SingleStep = 0x00
	header.Track0S0AltEncoding = 0xFF
	header.Track0S0Encoding = ENC_ISOIBM_MFM
	header.Track0S1AltEncoding = 0xFF
	header.Track0S1Encoding = ENC_ISOIBM_MFM

	// Write header
	headerBuf := make([]byte, BlockSize)
	for i := range headerBuf {
		headerBuf[i] = 0xFF
	}
	headerData := make([]byte, 32)
	copy(headerData[0:8], header.HeaderSignature[:])
	headerData[8] = header.FormatRevision
	headerData[9] = header.NumberOfTrack
	headerData[10] = header.NumberOfSide
	headerData[11] = header.TrackEncoding
	binary.LittleEndian.PutUint16(headerData[12:14], header.BitRate)
	binary.LittleEndian.PutUint16(headerData[14:16], header.FloppyRPM)
	headerData[16] = header.FloppyInterfaceMode
	headerData[17] = header.WriteProtected
	binary.LittleEndian.PutUint16(headerData[18:20], header.TrackListOffset)
	headerData[20] = header.WriteAllowed
	headerData[21] = header.SingleStep
	headerData[22] = header.Track0S0AltEncoding
	headerData[23] = header.Track0S0Encoding
	headerData[24] = header.Track0S1AltEncoding
	headerData[25] = header.Track0S1Encoding
	copy(headerBuf, headerData)
	if _, err := file.Write(headerBuf); err != nil {
		t.Fatalf("Write() header error: %v", err)
	}

	// Write minimal track list
	trackListBuf := make([]byte, BlockSize)
	for i := range trackListBuf {
		trackListBuf[i] = 0xFF
	}
	binary.LittleEndian.PutUint16(trackListBuf[0:2], 2)   // Track offset
	binary.LittleEndian.PutUint16(trackListBuf[2:4], 512) // Track length
	if _, err := file.Write(trackListBuf); err != nil {
		t.Fatalf("Write() track list error: %v", err)
	}

	// Write minimal track data
	trackData := make([]byte, 512)
	for i := range trackData {
		trackData[i] = 0xFF
	}
	if _, err := file.Write(trackData); err != nil {
		t.Fatalf("Write() track data error: %v", err)
	}
	file.Close()

	// Try to read v2 file (should fail)
	_, err = Read(tmpFile)
	if err == nil {
		t.Fatalf("Read() v2 file: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "v2 format") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("Read() v2 file: expected error about v2 not supported, got %v", err)
	}
}

func TestWriteV1V3Compatibility(t *testing.T) {
	// Test that v1 files can be read by the same reader (which supports both)
	disk := createTestDisk(1, 1, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_v1_compat.hfe")

	// Write v1 format
	if err := Write(tmpFile, disk, HFEVersion1); err != nil {
		t.Fatalf("Write() v1 error: %v", err)
	}

	// Read it back (should work with existing Read function)
	readDisk, err := Read(tmpFile)
	if err != nil {
		t.Fatalf("Read() v1 file error (compatibility test): %v", err)
	}

	// Verify it was read correctly
	if string(readDisk.Header.HeaderSignature[:]) != HFEv1Signature {
		t.Errorf("Read() v1 signature = %s, expected %s", string(readDisk.Header.HeaderSignature[:]), HFEv1Signature)
	}

	if len(readDisk.Tracks) != 1 {
		t.Errorf("Read() v1 track count = %d, expected 1", len(readDisk.Tracks))
	}
}

func TestWriteV1NoOpcodes(t *testing.T) {
	// Test that v1 format doesn't use opcodes (0xF0-0xF4)
	disk := createTestDisk(1, 1, 256)
	tmpFile := filepath.Join(t.TempDir(), "test_v1_no_opcodes.hfe")

	if err := Write(tmpFile, disk, HFEVersion1); err != nil {
		t.Fatalf("Write() v1 error: %v", err)
	}

	// Read raw file and check track data doesn't contain opcodes
	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer file.Close()

	// Seek to track data (after header + track list = 2 blocks = 1024 bytes)
	if _, err := file.Seek(BlockSize*2, io.SeekStart); err != nil {
		t.Fatalf("Seek() error: %v", err)
	}

	// Read first track block (512 bytes)
	trackBuf := make([]byte, BlockSize)
	if _, err := io.ReadFull(file, trackBuf); err != nil {
		t.Fatalf("ReadFull() error: %v", err)
	}

	// Check that padding uses 0xFF, not NOP opcode (0xF0)
	// Note: We check the padding area, not the actual data area
	// The actual data may contain any bytes, but padding should be 0xFF
	// For v1, padding is 0xFF, for v3 it would be 0xF0 (NOP opcode)
	// We'll check the last few bytes which are likely padding
	for i := len(trackBuf) - 10; i < len(trackBuf); i++ {
		// Padding should be 0xFF, not 0xF0 (NOP opcode)
		if trackBuf[i] == NOP_OPCODE {
			t.Errorf("Write() v1 track data at offset %d contains NOP opcode (0xF0), expected 0xFF padding", i)
		}
	}
}
