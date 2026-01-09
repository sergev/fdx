package supercardpro

import (
	"fmt"
	"io"
)

// SCPInfo contains hardware and firmware version information
type SCPInfo struct {
	HardwareMajor uint8
	HardwareMinor uint8
	FirmwareMajor uint8
	FirmwareMinor uint8
}

// getSCPInfo retrieves hardware and firmware version information from the device
func (c *Client) getSCPInfo() (SCPInfo, error) {
	var info SCPInfo

	// Send SCPCMD_SCPINFO command with no data
	err := c.scpSend(SCPCMD_SCPINFO, nil, nil)
	if err != nil {
		return info, fmt.Errorf("failed to send SCPINFO command: %w", err)
	}

	// Read 2 bytes: [hardware_version][firmware_version]
	response := make([]byte, 2)
	_, err = io.ReadFull(c.port, response)
	if err != nil {
		return info, fmt.Errorf("failed to read version info: %w", err)
	}

	// Parse versions: upper nibble = major, lower nibble = minor
	info.HardwareMajor = response[0] >> 4
	info.HardwareMinor = response[0] & 0x0f
	info.FirmwareMajor = response[1] >> 4
	info.FirmwareMinor = response[1] & 0x0f

	return info, nil
}

// PrintStatus prints SuperCard Pro status information to stdout
func (c *Client) PrintStatus() {

	// Fetch and display hardware and firmware versions
	info, err := c.getSCPInfo()
	if err != nil {
		// Failed to fetch version information
		fmt.Printf("SuperCard Pro Firmware Version: Unknown\n")
	} else {
		fmt.Printf("SuperCard Pro Hardware Version: %d.%d\n", info.HardwareMajor, info.HardwareMinor)
		fmt.Printf("Firmware Version: %d.%d\n", info.FirmwareMajor, info.FirmwareMinor)
	}
	fmt.Printf("Serial Number: %s\n", c.serialNumber)

	// Check whether drive 0 is connected.
	// Try to select drive 0 and seek to track 0.
	selectErr := c.selectDrive(0)
	seekErr := c.seekTrack(0)
	driveIsConnected := (selectErr == nil) && (seekErr == nil)

	if !driveIsConnected {
		fmt.Printf("Floppy Drive: Disconnected\n")
		// Clean up if we partially succeeded (drive was selected but seek failed)
		if selectErr == nil {
			c.deselectDrive(0)
		}
	} else {
		fmt.Printf("Floppy Drive: Connected\n")
		// Measure and display RPM
		// Note: selectDrive already turned on the motor, and seekTrack already positioned the head
		// Read flux data for 2 revolutions to calculate RPM
		fluxData, err := c.readFlux(2)
		if err == nil {
			rpm, _ := c.calculateRPMAndBitRate(fluxData)
			if rpm > 0 {
				fmt.Printf("Rotation Speed: %d RPM\n", rpm)
			}
		}
		// Clean up: deselect drive and turn off motor
		c.deselectDrive(0)
	}
}
