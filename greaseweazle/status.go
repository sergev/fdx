package greaseweazle

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/sergev/floppy/config"
)

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

// Show RPM
func (c *Client) PrintRotationSpeed() {
	// Use head #0.
	err := c.SetHead(0)
	if err != nil {
		return
	}

	err = c.SetMotor(0, true)
	if err != nil {
		return
	}
	defer c.SetMotor(0, false) // Turn off motor when done

	// Read flux data (0 ticks = no limit, 2 index pulses = 2 revolutions)
	fluxData, err := c.ReadFlux(0, 2)
	if err != nil {
		fmt.Printf("Floppy Disk: Not inserted\n")
		return
	}
	fmt.Printf("Floppy Disk: Inserted\n")

	// Calculate RPM from first track (cylinder 0, head 0)
	rpm, _ := c.calculateRPMAndBitRate(fluxData)
	if rpm > 0 {
		fmt.Printf("Rotation Speed: %d RPM\n", rpm)
	}
}

// PrintStatus prints all firmware information to stdout
func (c *Client) PrintStatus() {
	fw := c.firmwareInfo

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

	fmt.Printf("Greaseweazle Firmware Version: %d.%d\n", fw.FwMajor, fw.FwMinor)
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

	// Show whether drive 0 is connected.
	// Reset, then try to seek to track #0.
	driveIsConnected := (c.Reset() == nil) &&
		(c.SetBusType() == nil) &&
		(c.SelectDrive(0) == nil) &&
		(c.Seek(0) == nil)
	if !driveIsConnected {
		fmt.Printf("Floppy Drive: Not detected\n")
	} else {
		fmt.Printf("Floppy Drive: %s\n", config.DriveName)
		c.PrintRotationSpeed()
	}
}
