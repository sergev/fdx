package mfm

import (
	"fmt"
	"math/rand"
	"testing"
)

// Helper function: decodeAllBits decodes a fixed number of bits from the decoder.
// This corresponds to the number of bitcells in the original MFM pattern.
func decodeAllBits(decoder *Decoder, numBits int) []bool {
	bits := make([]bool, 0, numBits)
	for i := 0; i < numBits; i++ {
		bits = append(bits, decoder.NextBit())
	}
	return bits
}

// Helper function: randomizeFluxTransitions adds random variation to flux transitions
// to simulate real-world flux timing variations. Each transition can vary by up to
// 20% of bitcellPeriodNs. Uses a fixed seed for test reproducibility.
func randomizeFluxTransitions(transitions []uint64, bitRateKhz uint16) []uint64 {
	// Calculate bitcell period in nanoseconds (matching encoder.go logic)
	bitRateBps := float64(bitRateKhz) * 1000.0 * 2
	bitcellPeriodNs := uint64(1e9 / bitRateBps)

	// Initialize RNG with fixed seed for test reproducibility
	rng := rand.New(rand.NewSource(42))

	// Post-process transitions: add random variation up to 20% of bitcellPeriodNs
	// Variation can be positive or negative, but transitions must remain in order
	maxVariation := float64(bitcellPeriodNs) * 0.20 // 20% of bitcell period

	// Create a copy of transitions to avoid modifying the original slice
	randomized := make([]uint64, len(transitions))
	copy(randomized, transitions)

	previousTime := uint64(0)
	for i := range randomized {
		// Generate random variation between -20% and +20% of bitcellPeriodNs
		variation := (rng.Float64()*2.0 - 1.0) * maxVariation // Range: [-maxVariation, +maxVariation]

		// Apply variation to transition time
		newTime := float64(randomized[i]) + variation

		// Ensure transition time is non-negative and maintains order
		if newTime < float64(previousTime) {
			newTime = float64(previousTime) + 1 // Minimum 1ns after previous
		}

		randomized[i] = uint64(newTime)
		previousTime = randomized[i]
	}

	return randomized
}

// Helper function: createTestDecoder creates a decoder from MFM bytes using GenerateFluxTransitions.
// It post-processes transitions to add random variation to simulate real-world flux timing variations.
func createTestDecoder(mfmBits []byte, bitRateKhz uint16) (*Decoder, error) {
	transitions, err := GenerateFluxTransitions(mfmBits, bitRateKhz)
	if err != nil {
		return nil, err
	}

	// Randomize transitions to simulate real-world flux variations
	transitions = randomizeFluxTransitions(transitions, bitRateKhz)

	return NewDecoder(transitions, bitRateKhz), nil
}

// Helper function: verifyDecodedBits verifies that decoded bits match the expected MFM bit pattern.
// expectedBits should be a slice of bools representing the MFM bitstream (true = 1, false = 0).
func verifyDecodedBits(t *testing.T, decodedBits []bool, expectedBits []bool) {
	t.Helper()

	// Compare up to the length of expected bits
	minLen := len(decodedBits)
	if len(expectedBits) < minLen {
		minLen = len(expectedBits)
	}

	// Check matching bits
	for i := 0; i < minLen; i++ {
		if decodedBits[i] != expectedBits[i] {
			t.Errorf("Bit mismatch at position %d: got %v, expected %v", i, decodedBits[i], expectedBits[i])
		}
	}

	// Check length mismatch
	if len(decodedBits) < len(expectedBits) {
		t.Errorf("Decoded bits too short: got %d bits, expected %d bits", len(decodedBits), len(expectedBits))
	}
}

// Helper function: bitsToBytes converts a slice of bools (bits) to bytes (MSB-first).
func bitsToBytes(bits []bool) []byte {
	if len(bits) == 0 {
		return []byte{}
	}

	bytes := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		if bit {
			byteIdx := i / 8
			bitIdx := 7 - (i % 8) // MSB-first
			bytes[byteIdx] |= 1 << bitIdx
		}
	}
	return bytes
}

// Helper function: bytesToBits converts bytes to a slice of bools (MSB-first).
func bytesToBits(data []byte) []bool {
	bits := make([]bool, len(data)*8)
	for i := 0; i < len(data)*8; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8) // MSB-first
		bits[i] = (data[byteIdx] & (1 << bitIdx)) != 0
	}
	return bits
}

// Helper function: generateRealisticMFMPattern generates a realistic MFM bitstream following MFM encoding rules:
// - Each "1" bit is immediately followed by 0
// - No more than three "0" bits in a row
// - Between two "1"s there are always one, two or three "0"s
// Examples: 101, 1001, 10001
// Returns MFM bits as bytes (bitstream format, MSB-first).
func generateRealisticMFMPattern(length int) []byte {
	if length <= 0 {
		return []byte{}
	}

	var bits []bool

	// Generate pattern: 1, then 1-3 zeros, then 1, then 1-3 zeros, etc.
	// This creates valid MFM patterns: 101, 1001, 10001
	patternIndex := 0
	for len(bits) < length {
		// Add a "1" bit
		bits = append(bits, true)
		if len(bits) >= length {
			break
		}

		// Determine how many zeros to add (1, 2, or 3)
		// Cycle through: 1, 2, 3, 1, 2, 3, ...
		zeroCount := (patternIndex % 3) + 1
		patternIndex++

		// Add the zeros
		for i := 0; i < zeroCount && len(bits) < length; i++ {
			bits = append(bits, false)
		}
	}

	// Trim to exact length
	if len(bits) > length {
		bits = bits[:length]
	}

	return bitsToBytes(bits)
}

// TestDecoder_RealWorldMFMPatterns tests the decoder with realistic MFM flux patterns.
func TestDecoder_RealWorldMFMPatterns(t *testing.T) {
	bitRates := []uint16{250, 500, 1000}

	testCases := []struct {
		name     string
		mfmBits  []byte
		expected []bool
		desc     string
	}{
		{
			name:     "KnownPattern_0x44_0xa9",
			mfmBits:  []byte{0x44, 0xa9},
			expected: bytesToBits([]byte{0x44, 0xa9}),
			desc:     "Known pattern from encoder_test.go",
		},
		{
			name:     "ShortPattern_8bits",
			mfmBits:  generateRealisticMFMPattern(8),
			expected: bytesToBits(generateRealisticMFMPattern(8)),
			desc:     "Short realistic MFM pattern (8 bits)",
		},
		{
			name:     "ShortPattern_16bits",
			mfmBits:  generateRealisticMFMPattern(16),
			expected: bytesToBits(generateRealisticMFMPattern(16)),
			desc:     "Short realistic MFM pattern (16 bits)",
		},
		{
			name:     "MediumPattern_32bits",
			mfmBits:  generateRealisticMFMPattern(32),
			expected: bytesToBits(generateRealisticMFMPattern(32)),
			desc:     "Medium realistic MFM pattern (32 bits)",
		},
		{
			name:     "MediumPattern_64bits",
			mfmBits:  generateRealisticMFMPattern(64),
			expected: bytesToBits(generateRealisticMFMPattern(64)),
			desc:     "Medium realistic MFM pattern (64 bits)",
		},
		{
			name:     "LongPattern_128bits",
			mfmBits:  generateRealisticMFMPattern(128),
			expected: bytesToBits(generateRealisticMFMPattern(128)),
			desc:     "Long realistic MFM pattern (128 bits)",
		},
		{
			name:     "LongPattern_256bits",
			mfmBits:  generateRealisticMFMPattern(256),
			expected: bytesToBits(generateRealisticMFMPattern(256)),
			desc:     "Long realistic MFM pattern (256 bits)",
		},
	}

	for _, bitRate := range bitRates {
		t.Run(bitRateToName(bitRate), func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Create decoder from MFM bits
					decoder, err := createTestDecoder(tc.mfmBits, bitRate)
					if err != nil {
						t.Fatalf("createTestDecoder failed: %v", err)
					}

					// Decode exactly the number of bits we encoded
					numBits := len(tc.expected)
					decodedBits := decodeAllBits(decoder, numBits)

					// Verify decoded bits match expected pattern
					verifyDecodedBits(t, decodedBits, tc.expected)
				})
			}
		})
	}
}

// Helper function: bitRateToName converts bit rate to a test name.
func bitRateToName(bitRate uint16) string {
	return fmt.Sprintf("%dkHz", bitRate)
}

// TestDecoder_EndOfStream tests behavior when transitions run out mid-decoding.
func TestDecoder_EndOfStream(t *testing.T) {
	bitRates := []uint16{250, 500, 1000}

	for _, bitRate := range bitRates {
		t.Run(bitRateToName(bitRate), func(t *testing.T) {
			// Test with empty transitions
			t.Run("EmptyTransitions", func(t *testing.T) {
				decoder := NewDecoder([]uint64{}, bitRate)

				// Should return clocked zeros immediately
				if !decoder.IsDone() {
					t.Error("IsDone() should return true for empty transitions")
				}

				// Call NextBit multiple times - should return false (clocked zero) and increment counter
				initialClockedZeros := decoder.ClockedZeros
				for i := 0; i < 5; i++ {
					bit := decoder.NextBit()
					if bit {
						t.Errorf("NextBit() should return false (clocked zero) for empty transitions, got true at call %d", i+1)
					}
					if decoder.ClockedZeros != initialClockedZeros+i+1 {
						t.Errorf("ClockedZeros should increment, got %d, expected %d", decoder.ClockedZeros, initialClockedZeros+i+1)
					}
				}
			})

			// Test with transitions that end abruptly (partial pattern)
			t.Run("PartialTransitions", func(t *testing.T) {
				// Create a short realistic MFM pattern
				mfmBits := generateRealisticMFMPattern(16)
				transitions, err := GenerateFluxTransitions(mfmBits, bitRate)
				if err != nil {
					t.Fatalf("GenerateFluxTransitions failed: %v", err)
				}

				// Create decoder with these transitions
				decoder := NewDecoder(transitions, bitRate)

				// Decode all 16 bits (the full pattern)
				bitsDecoded := 0
				for bitsDecoded < 16 {
					decoder.NextBit()
					bitsDecoded++
				}

				// Now transitions should be exhausted (IsDone returns true)
				// But we need to check - IsDone() might not be true yet if there's remaining flux
				// Let's decode a few more bits to ensure we've consumed all transitions
				for !decoder.IsDone() && bitsDecoded < 32 {
					decoder.NextBit()
					bitsDecoded++
				}

				// Verify IsDone() returns true after transitions are exhausted
				if !decoder.IsDone() {
					t.Error("IsDone() should return true after transitions are exhausted")
				}

				// Call NextBit() multiple times after exhaustion
				// Should return false (clocked zero) and increment ClockedZeros
				clockedZerosBefore := decoder.ClockedZeros
				for i := 0; i < 10; i++ {
					bit := decoder.NextBit()
					if bit {
						t.Errorf("NextBit() should return false (clocked zero) after exhaustion, got true at call %d", i+1)
					}
				}

				// Verify ClockedZeros counter incremented
				if decoder.ClockedZeros <= clockedZerosBefore {
					t.Errorf("ClockedZeros should increment after exhaustion, got %d, expected > %d", decoder.ClockedZeros, clockedZerosBefore)
				}
			})

			// Test with very short transitions (single transition)
			t.Run("SingleTransition", func(t *testing.T) {
				// Create a pattern with just one transition
				mfmBits := []byte{0x80} // Single bit set (MSB)
				transitions, err := GenerateFluxTransitions(mfmBits, bitRate)
				if err != nil {
					t.Fatalf("GenerateFluxTransitions failed: %v", err)
				}

				decoder := NewDecoder(transitions, bitRate)

				// Decode a few bits
				bits := make([]bool, 0)
				for i := 0; i < 20 && !decoder.IsDone(); i++ {
					bits = append(bits, decoder.NextBit())
				}

				// Continue decoding after exhaustion
				clockedZerosBefore := decoder.ClockedZeros
				for i := 0; i < 5; i++ {
					bit := decoder.NextBit()
					if bit {
						t.Errorf("NextBit() should return false (clocked zero) after exhaustion, got true")
					}
				}

				if decoder.ClockedZeros <= clockedZerosBefore {
					t.Errorf("ClockedZeros should increment after exhaustion")
				}
			})

			// Test that decoder doesn't panic when transitions are exhausted
			t.Run("NoPanicOnExhaustion", func(t *testing.T) {
				mfmBits := generateRealisticMFMPattern(32)
				transitions, err := GenerateFluxTransitions(mfmBits, bitRate)
				if err != nil {
					t.Fatalf("GenerateFluxTransitions failed: %v", err)
				}

				decoder := NewDecoder(transitions, bitRate)

				// Decode until done
				for !decoder.IsDone() {
					decoder.NextBit()
				}

				// Call NextBit many times - should not panic
				for i := 0; i < 100; i++ {
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Errorf("NextBit() panicked after exhaustion: %v", r)
							}
						}()
						decoder.NextBit()
					}()
				}
			})
		})
	}
}
