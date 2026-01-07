package greaseweazle

import (
	"testing"
)

// Verify function mfmToFluxTransitions().
// Encode two MFM bytes 0x0f 0x06 with bitRateKhz=500
func TestMfmToFluxTransitions(t *testing.T) {
	mfmBits := []byte{0x0f, 0x06}
	bitRateKhz := uint16(500)
	expectedTransitions := []uint64{4000, 8000, 13000, 15000}

	// Call the function
	transitions, err := mfmToFluxTransitions(mfmBits, bitRateKhz)

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
