package mfm

import (
	"testing"
)

// Verify function mfmToFluxTransitions().
// Encode two MFM bytes 0x0f 0x06 with bitRateKhz=500
func TestMfmToFluxTransitions(t *testing.T) {
	bitRateKhz := uint16(500)

	//       ---4--- ---4--- ---a--- ---9---
	//  MFM: 0 1 0 0 0 1 0 0 1 0 1 0 1 0 0 1
	//          _______       ___     _____
	// Flux: __/       \_____/   \___/     \_

	mfmBits := []byte{0x44, 0xa9}
	expectedTransitions := []uint64{2000, 6000, 9000, 11000, 13000, 16000}

	// Call the function
	transitions, err := GenerateFluxTransitions(mfmBits, bitRateKhz)

	// Verify no error
	if err != nil {
		t.Fatalf("mfmToFluxTransitions() returned error: %v", err)
	}

	// Manual comparison: check length
	if len(transitions) != len(expectedTransitions) {
		t.Errorf("mfmToFluxTransitions() returned %d transitions, expected %d", len(transitions), len(expectedTransitions))
		t.Errorf("Got transitions: %v", transitions)
		t.Errorf("Expected transitions: %v", expectedTransitions)
		return
	}

	// Manual comparison: check each element
	for i := 0; i < len(expectedTransitions); i++ {
		if transitions[i] != expectedTransitions[i] {
			t.Errorf("mfmToFluxTransitions() transition[%d] = %d, expected %d", i, transitions[i], expectedTransitions[i])
		}
	}

	// If we get here and there were errors, the test will have failed above
	// This is just to ensure we validate all elements
	if t.Failed() {
		t.Errorf("Full transitions array: %v", transitions)
		t.Errorf("Expected transitions array: %v", expectedTransitions)
	}
}

