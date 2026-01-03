package greaseweazle

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"floppy/adapter"
	"floppy/hfe"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

const (
	VendorID  = 0x1209 // Open source hardware projects
	ProductID = 0x4d69 // Keir Fraser Greaseweazle
)

// Command codes
const (
	CMD_GET_INFO        = 0
	CMD_UPDATE          = 1
	CMD_SEEK            = 2
	CMD_HEAD            = 3
	CMD_SET_PARAMS      = 4
	CMD_GET_PARAMS      = 5
	CMD_MOTOR           = 6
	CMD_READ_FLUX       = 7
	CMD_WRITE_FLUX      = 8
	CMD_GET_FLUX_STATUS = 9
	CMD_SWITCH_FW_MODE  = 11
	CMD_SELECT          = 12
	CMD_DESELECT        = 13
	CMD_SET_BUS_TYPE    = 14
	CMD_SET_PIN         = 15
	CMD_RESET           = 16
	CMD_ERASE_FLUX      = 17
	CMD_SOURCE_BYTES    = 18
	CMD_SINK_BYTES      = 19
	CMD_GET_PIN         = 20
)

// GET_INFO indices
const (
	GETINFO_FIRMWARE      = 0
	GETINFO_BW_STATS      = 1
	GETINFO_CURRENT_DRIVE = 7
	GETINFO_DRIVE_0       = 8 // GETINFO_DRIVE(0)
	GETINFO_DRIVE_1       = 9 // GETINFO_DRIVE(1)
)

// Drive info flags
const (
	GW_DF_CYL_VALID = 1 << 0 // _GW_DF_cyl_valid
	GW_DF_MOTOR_ON  = 1 << 1 // _GW_DF_motor_on
	GW_DF_IS_FLIPPY = 1 << 2 // _GW_DF_is_flippy
)

// ACK return codes
const (
	ACK_OKAY           = 0
	ACK_BAD_COMMAND    = 1
	ACK_NO_INDEX       = 2
	ACK_NO_TRK0        = 3
	ACK_FLUX_OVERFLOW  = 4
	ACK_FLUX_UNDERFLOW = 5
	ACK_WRPROT         = 6
	ACK_NO_UNIT        = 7
	ACK_NO_BUS         = 8
	ACK_BAD_UNIT       = 9
	ACK_BAD_PIN        = 10
	ACK_BAD_CYLINDER   = 11
)

// Sentinel error for unsupported pins
var ErrBadPin = errors.New("pin not supported")

// Flux stream opcodes
const (
	FLUXOP_INDEX = 1
	FLUXOP_SPACE = 2
)

// PLL and MFM constants
const (
	MFM_NOMINAL_PERIOD_NS = 2000 // 250 kbps MFM: 1 bitcell = 2000ns
	PLL_DAMPING           = 0.2  // Damping factor for PLL
	PLL_WINDOW_TOLERANCE  = 0.25 // ±25% window tolerance
)

// Bus type codes
const (
	BUS_NONE    = 0
	BUS_IBMPC   = 1
	BUS_SHUGART = 2
)

// Client wraps a serial port connection to a Greaseweazle device
type Client struct {
	port         serial.Port
	firmwareInfo FirmwareInfo
	serialNumber string
}

// NewClient creates a new Greaseweazle client using the provided port details
// It opens the serial port, fetches the firmware version during initialization, and stores all information
// Returns a FloppyAdapter interface implementation
func NewClient(portDetails *enumerator.PortDetails) (adapter.FloppyAdapter, error) {
	// Open the serial port
	mode := &serial.Mode{
		BaudRate: 9600,
	}
	port, err := serial.Open(portDetails.Name, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portDetails.Name, err)
	}

	client := &Client{
		port:         port,
		serialNumber: portDetails.SerialNumber,
	}

	// Fetch firmware version during initialization
	fwInfo, err := client.fetchFirmwareVersion()
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to fetch firmware version: %w", err)
	}
	client.firmwareInfo = fwInfo

	/* Twiddle the baud rate, which indicates to the Greaseweazle that the
	 * data stream has been reset. */
	err = port.SetMode(&serial.Mode{BaudRate: 10000})
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to set baud rate to 10000: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	err = port.SetMode(&serial.Mode{BaudRate: 9600})
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to set baud rate to 9600: %w", err)
	}

	/* Configure the hardware. */
	cmd := []byte{CMD_SET_BUS_TYPE, 3, BUS_IBMPC}
	err = client.doCommand(cmd)
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to set bus type: %w", err)
	}

	return client, nil
}

// ackError converts an ACK error code to a readable error message
func ackError(code byte) error {
	msg := "unknown error"
	switch code {
	case ACK_OKAY:
		return nil
	case ACK_BAD_COMMAND:
		msg = "bad command"
	case ACK_NO_INDEX:
		msg = "no index"
	case ACK_NO_TRK0:
		msg = "no track 0"
	case ACK_FLUX_OVERFLOW:
		msg = "overflow"
	case ACK_FLUX_UNDERFLOW:
		msg = "underflow"
	case ACK_WRPROT:
		msg = "write protected"
	case ACK_NO_UNIT:
		msg = "no unit"
	case ACK_NO_BUS:
		msg = "no bus"
	case ACK_BAD_UNIT:
		msg = "invalid unit"
	case ACK_BAD_PIN:
		msg = "invalid pin"
	case ACK_BAD_CYLINDER:
		msg = "invalid track"
	}
	return fmt.Errorf("Greaseweazle error: %s", msg)
}

// doCommand sends a command and reads the ACK response
func (c *Client) doCommand(cmd []byte) error {
	// Send command
	_, err := c.port.Write(cmd)
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	// Read ACK response (2 bytes: command echo, status)
	ack := make([]byte, 2)
	_, err = io.ReadFull(c.port, ack)
	if err != nil {
		return fmt.Errorf("failed to read ACK: %w", err)
	}

	// Validate command echo matches
	if ack[0] != cmd[0] {
		return fmt.Errorf("command returned garbage (0x%02x != 0x%02x with status 0x%02x)",
			ack[0], cmd[0], ack[1])
	}

	// Check status
	return ackError(ack[1])
}

// FirmwareInfo contains all firmware information from GETINFO_FIRMWARE response
type FirmwareInfo struct {
	FwMajor        uint8
	FwMinor        uint8
	IsMainFirmware bool // == 0 means bootloader
	MaxCmd         uint8
	SampleFreqHz   uint32
	HwModel        uint8
	HwSubmodel     uint8
	USBSpeed       uint8
	MCUID          uint8
	MCUMhz         uint16
	MCUSRAMKB      uint16
	USBBufKB       uint16
}

// BwStats contains bandwidth statistics from GETINFO_BW_STATS response
type BwStats struct {
	MinBw struct {
		Bytes uint32
		Usecs uint32
	}
	MaxBw struct {
		Bytes uint32
		Usecs uint32
	}
}

// fetchFirmwareVersion retrieves all firmware information from the Greaseweazle device
// This is called during initialization and the result is stored in the Client struct
func (c *Client) fetchFirmwareVersion() (FirmwareInfo, error) {
	var info FirmwareInfo

	// Send CMD_GET_INFO command: [CMD_GET_INFO, length=3, GETINFO_FIRMWARE]
	cmd := []byte{CMD_GET_INFO, 3, GETINFO_FIRMWARE}
	err := c.doCommand(cmd)
	if err != nil {
		return info, fmt.Errorf("failed to send GET_INFO command: %w", err)
	}

	// Read 32-byte response
	response := make([]byte, 32)
	_, err = io.ReadFull(c.port, response)
	if err != nil {
		return info, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse all fields according to packed struct layout:
	// byte 0: fw_major (uint8)
	// byte 1: fw_minor (uint8)
	// byte 2: is_main_firmware (uint8, 0 = bootloader)
	// byte 3: max_cmd (uint8)
	// bytes 4-7: sample_freq (uint32, little-endian)
	// byte 8: hw_model (uint8)
	// byte 9: hw_submodel (uint8)
	// byte 10: usb_speed (uint8)
	// byte 11: mcu_id (uint8)
	// bytes 12-13: mcu_mhz (uint16, little-endian)
	// bytes 14-15: mcu_sram_kb (uint16, little-endian)
	// bytes 16-17: usb_buf_kb (uint16, little-endian)
	info.FwMajor = response[0]
	info.FwMinor = response[1]
	info.IsMainFirmware = response[2] != 0
	info.MaxCmd = response[3]
	info.SampleFreqHz = binary.LittleEndian.Uint32(response[4:8])
	info.HwModel = response[8]
	info.HwSubmodel = response[9]
	info.USBSpeed = response[10]
	info.MCUID = response[11]
	info.MCUMhz = binary.LittleEndian.Uint16(response[12:14])
	info.MCUSRAMKB = binary.LittleEndian.Uint16(response[14:16])
	info.USBBufKB = binary.LittleEndian.Uint16(response[16:18])

	return info, nil
}

// fetchBwStats retrieves bandwidth statistics from the Greaseweazle device
func (c *Client) fetchBwStats() (BwStats, error) {
	var stats BwStats

	// Send CMD_GET_INFO command: [CMD_GET_INFO, length=3, GETINFO_BW_STATS]
	cmd := []byte{CMD_GET_INFO, 3, GETINFO_BW_STATS}
	err := c.doCommand(cmd)
	if err != nil {
		return stats, fmt.Errorf("failed to send GET_INFO BW_STATS command: %w", err)
	}

	// Read 16-byte response (4 uint32_t values in little-endian format)
	response := make([]byte, 16)
	_, err = io.ReadFull(c.port, response)
	if err != nil {
		return stats, fmt.Errorf("failed to read BW_STATS response: %w", err)
	}

	// Parse all fields according to packed struct layout:
	// bytes 0-3: min_bw.bytes (uint32, little-endian)
	// bytes 4-7: min_bw.usecs (uint32, little-endian)
	// bytes 8-11: max_bw.bytes (uint32, little-endian)
	// bytes 12-15: max_bw.usecs (uint32, little-endian)
	stats.MinBw.Bytes = binary.LittleEndian.Uint32(response[0:4])
	stats.MinBw.Usecs = binary.LittleEndian.Uint32(response[4:8])
	stats.MaxBw.Bytes = binary.LittleEndian.Uint32(response[8:12])
	stats.MaxBw.Usecs = binary.LittleEndian.Uint32(response[12:16])

	return stats, nil
}

// getPinValue reads the pin level for the specified pin number
// Returns true for High (1), false for Low (0), or ErrBadPin if the pin is not supported
func (c *Client) getPinValue(pin byte) (bool, error) {
	// Send CMD_GET_PIN command: [CMD_GET_PIN, length=3, pin#]
	cmd := []byte{CMD_GET_PIN, 3, pin}
	_, err := c.port.Write(cmd)
	if err != nil {
		return false, fmt.Errorf("failed to write command: %w", err)
	}

	// Read ACK response (2 bytes: command echo, status)
	ack := make([]byte, 2)
	_, err = io.ReadFull(c.port, ack)
	if err != nil {
		return false, fmt.Errorf("failed to read ACK: %w", err)
	}

	// Validate command echo matches
	if ack[0] != cmd[0] {
		return false, fmt.Errorf("command returned garbage (0x%02x != 0x%02x with status 0x%02x)",
			ack[0], cmd[0], ack[1])
	}

	// Check status
	if ack[1] == ACK_BAD_PIN {
		return false, ErrBadPin
	}

	if ack[1] != ACK_OKAY {
		return false, ackError(ack[1])
	}

	// Read pin level byte (1=High, 0=Low)
	pinLevel := make([]byte, 1)
	_, err = io.ReadFull(c.port, pinLevel)
	if err != nil {
		return false, fmt.Errorf("failed to read pin level: %w", err)
	}

	return pinLevel[0] == 1, nil
}

// Display bandwidth statistics
func (c *Client) PrintBwStats() {
	bwStats, err := c.fetchBwStats()
	if err != nil {
		fmt.Printf("Warning: Failed to fetch bandwidth statistics: %v\n", err)
	} else {
		// Calculate throughput for min bandwidth (MB/s)
		var minBwMBs float64
		if bwStats.MinBw.Usecs > 0 {
			minBwMBs = float64(bwStats.MinBw.Bytes) / float64(bwStats.MinBw.Usecs) * 1000000.0 / 1024.0 / 1024.0
		}

		// Calculate throughput for max bandwidth (MB/s)
		var maxBwMBs float64
		if bwStats.MaxBw.Usecs > 0 {
			maxBwMBs = float64(bwStats.MaxBw.Bytes) / float64(bwStats.MaxBw.Usecs) * 1000000.0 / 1024.0 / 1024.0
		}

		fmt.Printf("\nBandwidth Statistics:\n")
		fmt.Printf("  Min: %d bytes in %d μs (%.2f MB/s)\n", bwStats.MinBw.Bytes, bwStats.MinBw.Usecs, minBwMBs)
		fmt.Printf("  Max: %d bytes in %d μs (%.2f MB/s)\n", bwStats.MaxBw.Bytes, bwStats.MaxBw.Usecs, maxBwMBs)
	}
}

// Display pin status
func (c *Client) PrintPins() {
	fmt.Printf("\nPin Status:\n")
	for pin := byte(1); pin <= 34; pin++ {
		pinLevel, err := c.getPinValue(pin)
		if err == ErrBadPin {
			// Skip unsupported pins
			continue
		}
		if err != nil {
			// Log warning for other errors but continue
			fmt.Printf("  Pin %d: Error reading pin: %v\n", pin, err)
			continue
		}

		levelStr := "Low"
		if pinLevel {
			levelStr = "High"
		}
		fmt.Printf("  Pin %d: %s\n", pin, levelStr)
	}
}

// PrintStatus prints all firmware information to stdout
func (c *Client) PrintStatus() {
	fw := c.firmwareInfo
	firmwareMode := "Bootloader"
	if fw.IsMainFirmware {
		firmwareMode = "Main Firmware"
	}

	usbSpeedStr := "Unknown"
	switch fw.USBSpeed {
	case 0:
		usbSpeedStr = "Full Speed"
	case 1:
		usbSpeedStr = "High Speed"
	default:
		usbSpeedStr = fmt.Sprintf("Unknown (%d)", fw.USBSpeed)
	}

	// Map hardware model to MCU name
	mcuName := "Unknown"
	switch fw.HwModel {
	case 1:
		mcuName = "STM32F1"
	case 7:
		mcuName = "STM32F7"
	case 4:
		mcuName = "AT32F4"
	default:
		mcuName = fmt.Sprintf("Unknown (model %d)", fw.HwModel)
	}

	fmt.Printf("Greaseweazle Firmware Version: %d.%d (%s)\n", fw.FwMajor, fw.FwMinor, firmwareMode)
	fmt.Printf("Serial Number: %s\n", c.serialNumber)
	fmt.Printf("Max Command: %d\n", fw.MaxCmd)
	fmt.Printf("Sample Frequency: %.1f MHz\n", float64(fw.SampleFreqHz)*1.0e-6)
	fmt.Printf("Hardware Model: %d.%d\n", fw.HwModel, fw.HwSubmodel)
	fmt.Printf("USB Speed: %s\n", usbSpeedStr)
	fmt.Printf("MCU: %s\n", mcuName)
	fmt.Printf("MCU Clock: %d MHz\n", fw.MCUMhz)
	fmt.Printf("MCU SRAM: %d KB\n", fw.MCUSRAMKB)
	fmt.Printf("USB Buffer: %d KB\n", fw.USBBufKB)

	// Display bandwidth statistics
	//c.PrintBwStats()

	// Display pin status
	//c.PrintPins()
}

// Seek moves the read/write head to the specified cylinder
func (c *Client) Seek(cylinder byte) error {
	cmd := []byte{CMD_SEEK, 3, cylinder}
	return c.doCommand(cmd)
}

// SetHead selects the specified head (0=bottom, 1=top)
func (c *Client) SetHead(head byte) error {
	cmd := []byte{CMD_HEAD, 3, head}
	return c.doCommand(cmd)
}

// SelectDrive selects the specified drive as the current unit
func (c *Client) SelectDrive(drive byte) error {
	cmd := []byte{CMD_SELECT, 3, drive}
	return c.doCommand(cmd)
}

// SetMotor turns the drive motor on or off
func (c *Client) SetMotor(drive byte, on bool) error {
	var motorState byte = 0
	if on {
		motorState = 1
	}
	cmd := []byte{CMD_MOTOR, 4, drive, motorState}
	return c.doCommand(cmd)
}

// ReadFlux reads raw flux data from the current track
// ticks: maximum ticks to read (0 = no limit)
// maxIndex: maximum index pulses to read (0 = no limit, typically 2 for 2 revolutions)
func (c *Client) ReadFlux(ticks uint32, maxIndex uint16) ([]byte, error) {
	// Build CMD_READ_FLUX command: [CMD_READ_FLUX, 8, ticks (le32), maxIndex (le16)]
	cmd := make([]byte, 8)
	cmd[0] = CMD_READ_FLUX
	cmd[1] = 8
	binary.LittleEndian.PutUint32(cmd[2:6], ticks)
	binary.LittleEndian.PutUint16(cmd[6:8], maxIndex)

	err := c.doCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to send READ_FLUX command: %w", err)
	}

	// Read flux data until we encounter a 0 byte (end of stream marker)
	var data []byte
	buf := make([]byte, 1)
	for {
		_, err := io.ReadFull(c.port, buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read flux data: %w", err)
		}
		if buf[0] == 0 {
			break
		}
		data = append(data, buf[0])
	}

	return data, nil
}

// GetFluxStatus retrieves the status of the last read/write operation
func (c *Client) GetFluxStatus() error {
	cmd := []byte{CMD_GET_FLUX_STATUS, 2}
	return c.doCommand(cmd)
}

// readN28 decodes a 28-bit value from Greaseweazle N28 encoding
// Returns the decoded value and the number of bytes consumed
func readN28(data []byte, offset int) (uint32, int, error) {
	if offset+4 > len(data) {
		return 0, 0, fmt.Errorf("insufficient data for N28 encoding at offset %d", offset)
	}

	b0 := data[offset]
	b1 := data[offset+1]
	b2 := data[offset+2]
	b3 := data[offset+3]

	value := ((uint32(b0) & 0xfe) >> 1) |
		((uint32(b1) & 0xfe) << 6) |
		((uint32(b2) & 0xfe) << 13) |
		((uint32(b3) & 0xfe) << 20)

	return value, 4, nil
}

// PLLState represents the state of the Phase-Locked Loop
type PLLState struct {
	period float64 // Expected bitcell period in nanoseconds
	phase  float64 // Current phase accumulator (0.0 to 1.0)
}

// absInt64 returns the absolute value of an int64
func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// calculateRPMFromFlux extracts index pulse timings from flux data and calculates RPM
// Returns the calculated RPM, or 300 (default) if calculation fails
func (c *Client) calculateRPMFromFlux(fluxData []byte) uint16 {
	if len(fluxData) == 0 {
		return 300 // Default RPM
	}

	var indexPulses []uint64 // Index pulse times in nanoseconds

	clockPeriodNs := 1e9 / float64(c.firmwareInfo.SampleFreqHz) // Nanoseconds per tick
	ticksAccumulated := uint64(0)

	i := 0
	for i < len(fluxData) {
		b := fluxData[i]

		if b == 0xFF {
			// Special opcode
			if i+1 >= len(fluxData) {
				break
			}

			opcode := fluxData[i+1]
			i += 2

			switch opcode {
			case FLUXOP_INDEX:
				// Index pulse marker
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					break
				}
				i += consumed
				indexTime := ticksAccumulated + uint64(n28)
				indexPulses = append(indexPulses, uint64(float64(indexTime)*clockPeriodNs))

			case FLUXOP_SPACE:
				// Time gap with no transitions
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					break
				}
				i += consumed
				ticksAccumulated += uint64(n28)

			default:
				// Unknown opcode, skip
			}
		} else if b < 250 {
			// Direct interval: 1-249 ticks
			ticksAccumulated += uint64(b)
			i++
		} else {
			// Extended interval: 250-254
			if i+1 >= len(fluxData) {
				break
			}
			delta := 250 + uint64(b-250)*255 + uint64(fluxData[i+1]) - 1
			ticksAccumulated += delta
			i += 2
		}
	}

	// Need at least 2 index pulses to calculate rotation period
	if len(indexPulses) < 2 {
		return 300 // Default RPM
	}

	// Calculate average rotation period from consecutive index pulses
	var totalPeriodNs uint64 = 0
	periodCount := 0

	for i := 1; i < len(indexPulses); i++ {
		period := indexPulses[i] - indexPulses[i-1]
		if period > 0 {
			totalPeriodNs += period
			periodCount++
		}
	}

	if periodCount == 0 || totalPeriodNs == 0 {
		return 300 // Default RPM
	}

	// Calculate average period in seconds
	avgPeriodNs := float64(totalPeriodNs) / float64(periodCount)
	avgPeriodSeconds := avgPeriodNs / 1e9

	// Calculate RPM: 60 seconds per minute / period in seconds
	rpm := 60.0 / avgPeriodSeconds

	// Round to either 300 or 360 RPM (standard floppy drive speeds)
	// Use 330 RPM as the threshold (midpoint between 300 and 360)
	if rpm < 330 {
		return 300
	}
	return 360
}

// calculateBitRateFromFlux extracts flux transitions and calculates the average bitcell period
// to determine the bitrate in kbps. Returns the calculated bitrate, or 250 (default) if calculation fails.
func (c *Client) calculateBitRateFromFlux(fluxData []byte) uint16 {
	if len(fluxData) == 0 {
		return 250 // Default bitrate
	}

	// Decode Greaseweazle flux stream to get transition times
	var transitions []uint64 // Times in nanoseconds

	clockPeriodNs := 1e9 / float64(c.firmwareInfo.SampleFreqHz) // Nanoseconds per tick
	ticksAccumulated := uint64(0)

	i := 0
	for i < len(fluxData) {
		b := fluxData[i]

		if b == 0xFF {
			// Special opcode
			if i+1 >= len(fluxData) {
				break
			}

			opcode := fluxData[i+1]
			i += 2

			switch opcode {
			case FLUXOP_INDEX:
				// Index pulse marker - skip it for bitrate calculation
				_, consumed, err := readN28(fluxData, i)
				if err != nil {
					break
				}
				i += consumed
				// Don't add to transitions, just skip

			case FLUXOP_SPACE:
				// Time gap with no transitions
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					break
				}
				i += consumed
				ticksAccumulated += uint64(n28)

			default:
				// Unknown opcode, skip
			}
		} else if b < 250 {
			// Direct interval: 1-249 ticks
			ticksAccumulated += uint64(b)
			transitionTime := uint64(float64(ticksAccumulated) * clockPeriodNs)
			transitions = append(transitions, transitionTime)
			i++
		} else {
			// Extended interval: 250-254
			if i+1 >= len(fluxData) {
				break
			}
			delta := 250 + uint64(b-250)*255 + uint64(fluxData[i+1]) - 1
			ticksAccumulated += delta
			transitionTime := uint64(float64(ticksAccumulated) * clockPeriodNs)
			transitions = append(transitions, transitionTime)
			i += 2
		}
	}

	// Need at least 2 transitions to calculate bitcell period
	if len(transitions) < 2 {
		return 250 // Default bitrate
	}

	// Use simplified PLL to estimate bitcell period
	// Initialize with nominal period
	pllPeriod := float64(MFM_NOMINAL_PERIOD_NS)

	// Use first few transitions to lock PLL
	lastTransitionTime := transitions[0]
	sampleCount := 0
	maxSamples := 100
	if len(transitions) < maxSamples {
		maxSamples = len(transitions)
	}

	for i := 1; i < maxSamples; i++ {
		deltaTime := float64(transitions[i] - lastTransitionTime)

		// Calculate how many periods this interval represents
		periods := deltaTime / pllPeriod

		// Calculate phase error
		phaseError := periods - float64(int(periods+0.5))

		// Adjust period (with damping)
		pllPeriod += PLL_DAMPING * phaseError * pllPeriod

		// Clamp period to reasonable range (±50% of nominal)
		if pllPeriod < MFM_NOMINAL_PERIOD_NS*0.5 {
			pllPeriod = MFM_NOMINAL_PERIOD_NS * 0.5
		}
		if pllPeriod > MFM_NOMINAL_PERIOD_NS*1.5 {
			pllPeriod = MFM_NOMINAL_PERIOD_NS * 1.5
		}

		lastTransitionTime = transitions[i]
		sampleCount++
	}

	if sampleCount == 0 {
		return 250 // Default bitrate
	}

	// Convert bitcell period to bitrate
	// HFE BitRate is the bitcell rate in kbps (not the data rate)
	// For MFM: bitcell rate = 1,000,000 / period_ns
	bitcellRateKbps := 1_000_000.0 / pllPeriod

	// Round to standard floppy drive bitrates: 250, 500, or 1000 kbps
	// Use thresholds: < 375 -> 250, < 750 -> 500, >= 750 -> 1000
	if bitcellRateKbps < 375 {
		return 250
	} else if bitcellRateKbps < 750 {
		return 500
	}
	return 1000
}

// decodeFluxToMFM recovers raw MFM bitcells from Greaseweazle flux data using PLL,
// and returns MFM bitcells as bytes (bitcells packed MSB-first, not decoded data bits)
func (c *Client) decodeFluxToMFM(fluxData []byte) ([]byte, error) {
	if len(fluxData) == 0 {
		return nil, fmt.Errorf("empty flux data")
	}

	// Step 1: Decode Greaseweazle flux stream to get transition times
	var transitions []uint64 // Times in nanoseconds
	var indexPulses []uint64 // Index pulse times

	clockPeriodNs := 1e9 / float64(c.firmwareInfo.SampleFreqHz) // Nanoseconds per tick
	ticksAccumulated := uint64(0)

	i := 0
	for i < len(fluxData) {
		b := fluxData[i]

		if b == 0xFF {
			// Special opcode
			if i+1 >= len(fluxData) {
				return nil, fmt.Errorf("incomplete opcode at offset %d", i)
			}

			opcode := fluxData[i+1]
			i += 2

			switch opcode {
			case FLUXOP_INDEX:
				// Index pulse marker
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					return nil, fmt.Errorf("failed to read INDEX N28: %w", err)
				}
				i += consumed
				indexTime := ticksAccumulated + uint64(n28)
				indexPulses = append(indexPulses, uint64(float64(indexTime)*clockPeriodNs))
				// Index pulse doesn't advance the cursor

			case FLUXOP_SPACE:
				// Time gap with no transitions
				n28, consumed, err := readN28(fluxData, i)
				if err != nil {
					return nil, fmt.Errorf("failed to read SPACE N28: %w", err)
				}
				i += consumed
				ticksAccumulated += uint64(n28)

			default:
				return nil, fmt.Errorf("unknown opcode 0x%02x at offset %d", opcode, i-1)
			}
		} else if b < 250 {
			// Direct interval: 1-249 ticks
			ticksAccumulated += uint64(b)
			transitionTime := uint64(float64(ticksAccumulated) * clockPeriodNs)
			transitions = append(transitions, transitionTime)
			i++
		} else {
			// Extended interval: 250-254
			if i+1 >= len(fluxData) {
				return nil, fmt.Errorf("incomplete extended interval at offset %d", i)
			}
			delta := 250 + uint64(b-250)*255 + uint64(fluxData[i+1]) - 1
			ticksAccumulated += delta
			transitionTime := uint64(float64(ticksAccumulated) * clockPeriodNs)
			transitions = append(transitions, transitionTime)
			i += 2
		}
	}

	if len(transitions) == 0 {
		return nil, fmt.Errorf("no flux transitions found")
	}

	// Step 2: Apply PLL to recover clock and generate bitcell boundaries
	pll := PLLState{
		period: MFM_NOMINAL_PERIOD_NS,
		phase:  0.0,
	}

	var bitcells []bool // MFM bitcells: true = transition (1), false = no transition (0)

	if len(transitions) < 2 {
		return nil, fmt.Errorf("insufficient transitions for PLL lock")
	}

	// Initialize PLL with first few transitions
	lastTransitionTime := transitions[0]

	for i := 1; i < len(transitions) && i < 100; i++ {
		deltaTime := float64(transitions[i] - lastTransitionTime)

		// Calculate expected phase at this time
		expectedPhase := pll.phase + (deltaTime / pll.period)

		// Calculate phase error (how many periods this interval represents)
		periods := deltaTime / pll.period
		phaseError := periods - float64(int(periods+0.5))

		// Adjust period
		pll.period += PLL_DAMPING * phaseError * pll.period

		// Clamp period to reasonable range (±50% of nominal)
		if pll.period < MFM_NOMINAL_PERIOD_NS*0.5 {
			pll.period = MFM_NOMINAL_PERIOD_NS * 0.5
		}
		if pll.period > MFM_NOMINAL_PERIOD_NS*1.5 {
			pll.period = MFM_NOMINAL_PERIOD_NS * 1.5
		}

		pll.phase = expectedPhase - float64(int(expectedPhase))
		lastTransitionTime = transitions[i]
	}

	// Step 3: Generate bitcell boundaries and decode MFM
	currentTime := transitions[0]
	transitionIdx := 0

	// Generate bitcells until we run out of transitions
	for transitionIdx < len(transitions) {
		// Calculate bitcell boundaries
		bitcellStart := currentTime
		bitcellEnd := currentTime + uint64(pll.period)

		// Look for transitions in this bitcell window
		windowMin := bitcellStart
		windowMax := bitcellEnd + uint64(pll.period*PLL_WINDOW_TOLERANCE)

		var transitionsInBitcell []uint64
		checkIdx := transitionIdx

		// Collect all transitions in the bitcell window
		for checkIdx < len(transitions) {
			if transitions[checkIdx] < windowMin {
				// Transition too early, skip it (noise?)
				checkIdx++
				continue
			}
			if transitions[checkIdx] > windowMax {
				// Transition too late, stop looking
				break
			}
			transitionsInBitcell = append(transitionsInBitcell, transitions[checkIdx])
			checkIdx++
		}

		// Store raw MFM bitcells: check if there's a transition at the end of the bitcell
		// For HFE format, we store the actual bitcells (1 = transition, 0 = no transition)
		// not the decoded data bits. Each bitcell represents whether there's a flux transition.
		hasEndTransition := false

		endWindowMin := bitcellEnd - uint64(pll.period*PLL_WINDOW_TOLERANCE)
		endWindowMax := bitcellEnd + uint64(pll.period*PLL_WINDOW_TOLERANCE)

		for _, transTime := range transitionsInBitcell {
			if transTime >= endWindowMin && transTime <= endWindowMax {
				hasEndTransition = true
				break
			}
		}

		// Store raw bitcell: 1 if transition at end, 0 if no transition
		bitcells = append(bitcells, hasEndTransition)

		// Advance transition index past transitions we've processed
		transitionIdx = checkIdx

		// Update PLL based on actual transition timing
		if len(transitionsInBitcell) > 0 {
			// Use the first transition for PLL update
			actualPeriod := float64(transitionsInBitcell[0] - bitcellStart)
			if actualPeriod > 0 {
				phaseError := (actualPeriod - pll.period) / pll.period
				pll.period += PLL_DAMPING * phaseError * pll.period

				// Clamp period
				if pll.period < MFM_NOMINAL_PERIOD_NS*0.5 {
					pll.period = MFM_NOMINAL_PERIOD_NS * 0.5
				}
				if pll.period > MFM_NOMINAL_PERIOD_NS*1.5 {
					pll.period = MFM_NOMINAL_PERIOD_NS * 1.5
				}
			}
		}

		currentTime = bitcellEnd

		// Limit output size to prevent excessive memory usage
		if len(bitcells) > 100000 {
			break
		}
	}

	if len(bitcells) == 0 {
		return nil, fmt.Errorf("no bitcells generated")
	}

	// Step 4: Pack bitcells as bytes (MSB-first)
	// Each bitcell becomes one bit in the output
	var mfmBytes []byte
	currentByte := byte(0)
	bitCount := 0

	for _, bit := range bitcells {
		if bit {
			currentByte |= 1 << (7 - bitCount)
		}
		bitCount++

		// When we have 8 bits, save the byte and start a new one
		if bitCount == 8 {
			mfmBytes = append(mfmBytes, currentByte)
			currentByte = 0
			bitCount = 0
		}
	}

	// Add any remaining partial byte
	if bitCount > 0 {
		mfmBytes = append(mfmBytes, currentByte)
	}

	if len(mfmBytes) == 0 {
		return nil, fmt.Errorf("no MFM bytes generated")
	}

	return mfmBytes, nil
}

// Read reads the entire floppy disk and writes it to the specified filename as HFE format
func (c *Client) Read(filename string) error {
	// Select drive 0 and turn on motor
	err := c.SelectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	err = c.SetMotor(0, true)
	if err != nil {
		return fmt.Errorf("failed to turn on motor: %w", err)
	}
	defer c.SetMotor(0, false) // Turn off motor when done

	// Initialize HFE disk structure
	disk := &hfe.Disk{
		Header: hfe.Header{
			NumberOfTrack:       80,
			NumberOfSide:        2,
			TrackEncoding:       hfe.ENC_ISOIBM_MFM,
			BitRate:             250,              // Will be calculated from flux data
			FloppyRPM:           300,              // Will be calculated from flux data
			FloppyInterfaceMode: hfe.IFM_IBMPC_DD, // Default to double density
			WriteProtected:      0xFF,             // Not write protected
			WriteAllowed:        0xFF,             // Write allowed
			SingleStep:          0xFF,             // Single step mode
			Track0S0AltEncoding: 0xFF,             // Use default encoding
			Track0S0Encoding:    hfe.ENC_ISOIBM_MFM,
			Track0S1AltEncoding: 0xFF, // Use default encoding
			Track0S1Encoding:    hfe.ENC_ISOIBM_MFM,
		},
		Tracks: make([]hfe.TrackData, 80),
	}

	// Iterate through 80 cylinders (0-79) and 2 heads (0-1)
	for cyl := 0; cyl < 80; cyl++ {
		for head := 0; head < 2; head++ {
			// Print progress message
			if cyl != 0 || head != 0 {
				fmt.Printf("\rReading track %d, side %d...", cyl, head)
			}

			// Seek to cylinder
			err = c.Seek(byte(cyl))
			if err != nil {
				return fmt.Errorf("failed to seek to cylinder %d: %w", cyl, err)
			}

			// Set head
			err = c.SetHead(byte(head))
			if err != nil {
				return fmt.Errorf("failed to set head %d: %w", head, err)
			}

			// Read flux data (0 ticks = no limit, 2 index pulses = 2 revolutions)
			fluxData, err := c.ReadFlux(0, 2)
			if err != nil {
				return fmt.Errorf("failed to read flux data from cylinder %d, head %d: %w", cyl, head, err)
			}

			// Calculate RPM and BitRate from first track (cylinder 0, head 0)
			if cyl == 0 && head == 0 {
				calculatedRPM := c.calculateRPMFromFlux(fluxData)
				fmt.Printf("Rotation Speed: %d RPM\n", calculatedRPM)

				calculatedBitRate := c.calculateBitRateFromFlux(fluxData)
				fmt.Printf("Bit Rate: %d kbps\n", calculatedBitRate)

				disk.Header.FloppyRPM = calculatedRPM
				disk.Header.BitRate = calculatedBitRate
			}

			// Decode flux data to MFM bitstream
			mfmBitstream, err := c.decodeFluxToMFM(fluxData)
			if err != nil {
				return fmt.Errorf("failed to decode flux data to MFM from cylinder %d, head %d: %w", cyl, head, err)
			}

			// Check flux status
			err = c.GetFluxStatus()
			if err != nil {
				return fmt.Errorf("flux status error after reading cylinder %d, head %d: %w", cyl, head, err)
			}

			// Store MFM bitstream in appropriate side
			if head == 0 {
				disk.Tracks[cyl].Side0 = mfmBitstream
			} else {
				disk.Tracks[cyl].Side1 = mfmBitstream
			}
		}
	}
	fmt.Printf(" Done\n")

	// Write HFE file
	fmt.Printf("Writing HFE file...\n")
	err = hfe.Write(filename, disk)
	if err != nil {
		return fmt.Errorf("failed to write HFE file: %w", err)
	}

	return nil
}

// Write writes data from the specified filename to the floppy disk
func (c *Client) Write(filename string) error {
	return fmt.Errorf("Write() not yet implemented for Greaseweazle adapter")
}

// Format formats the floppy disk
func (c *Client) Format() error {
	return fmt.Errorf("Format() not yet implemented for Greaseweazle adapter")
}

// Erase erases the floppy disk
func (c *Client) Erase() error {
	return fmt.Errorf("Erase() not yet implemented for Greaseweazle adapter")
}
