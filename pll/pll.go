package pll

// PLL and MFM constants
// SCP PLL algorithm constants (from legacy/mfmdisk/scp.c)
const (
	// CLOCK_MAX_ADJ is the +/- adjustment range (90%-110% of ideal)
	CLOCK_MAX_ADJ = 10 // +/- 10% adjustment range (90%-110% of CLOCK_CENTRE)
	// PERIOD_ADJ_PCT is the period adjustment percentage
	PERIOD_ADJ_PCT = 5 // Period adjustment percentage
	// PHASE_ADJ_PCT is the phase adjustment percentage
	PHASE_ADJ_PCT = 60 // Phase adjustment percentage
)

// Decoder decodes flux transitions into bits using an SCP-style Phase-Locked Loop.
// Based on pll_t from legacy/mfmdisk/scp.c
// It combines PLL state with flux iteration functionality.
type Decoder struct {
	// PLL state fields
	PeriodIdeal  float64 // Expected clock period in nanoseconds
	Period       float64 // Current clock period in nanoseconds
	Flux         float64 // Accumulated flux time in nanoseconds
	Time         float64 // Total time elapsed in nanoseconds
	ClockedZeros int     // Count of consecutive clocked zeros

	// Flux iterator fields
	transitions []uint64 // Absolute transition times in nanoseconds
	index       int      // Current index into transitions
	lastTime    uint64   // Last transition time (for calculating intervals)
}

// NewDecoder creates a new PLL decoder with the given transitions and bit rate.
// It initializes both the PLL state and flux iterator.
func NewDecoder(transitions []uint64, bitRateKhz uint16) *Decoder {
	return &Decoder{
		// Initialize PLL state
		PeriodIdeal:  1e6 / float64(bitRateKhz) / 2,
		Period:       1e6 / float64(bitRateKhz) / 2,
		Flux:         0,
		Time:         0,
		ClockedZeros: 0,
		// Initialize flux iterator
		transitions: transitions,
		index:       0,
		lastTime:    0,
	}
}

// NextFlux returns the next flux interval in nanoseconds (time until next transition).
// Returns 0 if no more transitions are available.
func (pll *Decoder) NextFlux() uint64 {
	if pll.index >= len(pll.transitions) {
		return 0 // No more transitions
	}

	nextTime := pll.transitions[pll.index]
	interval := nextTime - pll.lastTime
	pll.lastTime = nextTime
	pll.index++
	return interval
}

// IsDone returns true if all transitions have been consumed.
func (pll *Decoder) IsDone() bool {
	return pll.index >= len(pll.transitions)
}

// NextBit decodes and returns next bit from the flux input stream.
// Based on pll_next_bit() from legacy/mfmdisk/scp.c
// Returns: false for clocked zero, true for transition detected
func (pll *Decoder) NextBit() bool {
	//fmt.Printf("--- pllNextBit() period = %.0f, time = %.0f, flux = %.0f, periodIdeal = %.0f\n", pll.Period, pll.Time, pll.Flux, pll.PeriodIdeal)

	// Accumulate flux until it exceeds period/2
	for pll.Flux < pll.Period/2 {
		fluxInterval := pll.NextFlux()
		if fluxInterval == 0 {
			// No more transitions, return false (clocked zero)
			pll.ClockedZeros++
			//fmt.Printf("---     No more transitions, clockedZeros = %d\n", pll.ClockedZeros)
			return false // 0
		}
		pll.Flux += float64(fluxInterval)
		//fmt.Printf("---     increment flux = %.0f\n", pll.Flux)
	}

	// Advance time by one clock period
	pll.Time += pll.Period
	pll.Flux -= pll.Period
	//fmt.Printf("---     advance time = %.0f, flux = %.0f\n", pll.Time, pll.Flux)

	// Check if we have a clocked zero (flux >= period/2 after subtraction)
	if pll.Flux >= pll.Period/2 {
		pll.ClockedZeros++
		//fmt.Printf("---     return 0, clockedZeros = %d\n", pll.ClockedZeros)
		return false // 0
	}

	// Transition detected - adjust PLL parameters
	// PLL: Adjust clock period according to phase mismatch
	if pll.ClockedZeros <= 3 {
		// In sync: adjust base clock by a fraction of phase mismatch
		pll.Period += pll.Flux * PERIOD_ADJ_PCT / 100
		//fmt.Printf("---     in sync: adjust period = %.0f\n", pll.Period)
	} else {
		// Out of sync: adjust base clock towards centre
		pll.Period += (pll.PeriodIdeal - pll.Period) * PERIOD_ADJ_PCT / 100
		//fmt.Printf("---     out of sync: normalize period = %.0f\n", pll.Period)
	}

	// Clamp the period adjustment range
	// the minimum allowed clock period
	pMin := (pll.PeriodIdeal * (100 - CLOCK_MAX_ADJ)) / 100
	if pll.Period < pMin {
		pll.Period = pMin
		//fmt.Printf("---     clamp to min: period = %.0f\n", pll.Period)
	}

	// the maximum allowed clock period
	pMax := (pll.PeriodIdeal * (100 + CLOCK_MAX_ADJ)) / 100
	if pll.Period > pMax {
		pll.Period = pMax
		//fmt.Printf("---     clamp to max: period = %.0f\n", pll.Period)
	}

	// PLL: Adjust clock phase according to mismatch
	// PHASE_ADJ_PCT=100% -> timing window snaps to observed flux
	newFlux := pll.Flux * (100 - PHASE_ADJ_PCT) / 100
	pll.Time += pll.Flux - newFlux
	pll.Flux = newFlux
	//fmt.Printf("---     adjust phase: newFlux = %.0f, time = %.0f, flux = %.0f\n", newFlux, pll.Time, pll.Flux)

	pll.ClockedZeros = 0
	return true // 1
}
