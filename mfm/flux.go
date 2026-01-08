package mfm

import (
	"fmt"
)

// GenerateFluxTransitions converts MFM bitcells to flux transition times.
// MFM bitcells are bits where transitions occur when bit values change.
// Return transition times in nanoseconds relative to track start.
func GenerateFluxTransitions(mfmBits []byte, bitRateKhz uint16) ([]uint64, error) {
	if len(mfmBits) == 0 {
		return nil, fmt.Errorf("empty MFM data")
	}

	// Calculate bitcell period in nanoseconds
	// bitRateKhz is in kbps, so bitRate_bps = bitRateKhz * 1000
	bitRateBps := float64(bitRateKhz) * 1000.0 * 2
	bitcellPeriodNs := uint64(1e9 / bitRateBps)

	var transitions []uint64
	currentTime := uint64(0)

	// Process each bit in the MFM bitcell stream
	bitCount := len(mfmBits) * 8
	for i := 0; i < bitCount; i++ {
		// Extract bit at position i (MSB-first)
		byteIdx := i / 8
		bitIdx := 7 - (i % 8) // MSB-first
		currentBit := (mfmBits[byteIdx] & (1 << bitIdx)) != 0

		// Advance time by one bitcell period before checking for transition
		currentTime += bitcellPeriodNs

		// Add transition time when bit changes
		if currentBit {
			transitions = append(transitions, currentTime)
		}
	}
	return transitions, nil
}

// CoverFullRotation extends transitions array to cover a full rotation period.
// Appends transitions at 2-bitcell intervals until the rotation duration is reached.
func CoverFullRotation(transitions []uint64, bitRateKhz uint16, floppyRPM uint16) []uint64 {
	// Calculate rotation duration in nanoseconds
	// Rotation duration = 60 seconds / RPM = 60e9 nanoseconds / RPM
	rotationDurationNs := uint32(60e9 / float64(floppyRPM))

	// Calculate bitcell period in nanoseconds
	// bitRateKhz is in kbps, so bitRate_bps = bitRateKhz * 1000
	bitRateBps := float64(bitRateKhz) * 1000.0 * 2
	bitcellPeriodNs := uint64(1e9 / bitRateBps)

	// Calculate 2-bitcell period
	twoBitcellPeriodNs := 2 * bitcellPeriodNs

	// Get last transition time (or 0 if empty)
	lastTime := uint64(0)
	if len(transitions) > 0 {
		lastTime = transitions[len(transitions)-1]
	}

	// Append transitions at 2-bitcell intervals until we reach the rotation duration
	currentTime := lastTime
	for currentTime+twoBitcellPeriodNs <= uint64(rotationDurationNs) {
		currentTime += twoBitcellPeriodNs
		transitions = append(transitions, currentTime)
	}

	return transitions
}
