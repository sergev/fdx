package supercardpro

import (
	"fmt"
	"io"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
	"floppy/adapter"
)

const (
	VendorID  = 0x0403
	ProductID = 0x6015
)

const baudRate = 115200

// SCP command codes
const (
	SCPCMD_SCPINFO = 0xd0 // get SCP info
)

// SCP status codes
const (
	SCP_STATUS_OK = 0x4f // command successful
)

// Client wraps a serial port connection to a SuperCard Pro device
type Client struct {
	port         serial.Port
	serialNumber string
}

// NewClient creates a new SuperCard Pro client using the provided port details
// It opens the serial port and initializes the connection
func NewClient(portDetails *enumerator.PortDetails) (adapter.FloppyAdapter, error) {
	// Open the serial port
	mode := &serial.Mode{
		BaudRate: 38400,
	}
	port, err := serial.Open(portDetails.Name, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portDetails.Name, err)
	}

	client := &Client{
		port:         port,
		serialNumber: portDetails.SerialNumber,
	}

	// TODO: Add SuperCard Pro specific initialization when protocol is known
	// For now, we just open the port and store the connection

	return client, nil
}

// scpSend sends a command to the SuperCard Pro device using the SCP protocol
// Protocol: [cmd byte][len byte][data...][checksum byte]
// Checksum = 0x4a + sum of all bytes before it
// Response: [cmd echo byte][status byte]
// Status 0x4f = success, other values = error codes
func (c *Client) scpSend(cmd byte, data []byte) error {
	dataLen := len(data)
	if dataLen > 255 {
		return fmt.Errorf("data length %d exceeds maximum 255", dataLen)
	}

	// Build command packet: [cmd][len][data...][checksum]
	packet := make([]byte, 3+dataLen)
	packet[0] = cmd
	packet[1] = byte(dataLen)
	if dataLen > 0 {
		copy(packet[2:2+dataLen], data)
	}

	// Calculate checksum: 0x4a + sum of cmd, len, and data bytes
	checksum := byte(0x4a)
	for i := 0; i < 2+dataLen; i++ {
		checksum += packet[i]
	}
	packet[2+dataLen] = checksum

	// Write packet to serial port
	_, err := c.port.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to write command packet: %w", err)
	}

	// Read response: [cmd_echo][status]
	response := make([]byte, 2)
	_, err = io.ReadFull(c.port, response)
	if err != nil {
		return fmt.Errorf("failed to read command response: %w", err)
	}

	// Validate echo matches sent command
	if response[0] != cmd {
		return fmt.Errorf("command echo mismatch: sent 0x%02x, received 0x%02x", cmd, response[0])
	}

	// Check status
	if response[1] != SCP_STATUS_OK {
		return fmt.Errorf("command failed with status 0x%02x", response[1])
	}

	return nil
}

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
	err := c.scpSend(SCPCMD_SCPINFO, nil)
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
}

// Read reads the entire floppy disk and writes it to the specified filename
func (c *Client) Read(filename string) error {
	return fmt.Errorf("Read() not yet implemented for SuperCard Pro adapter")
}

// Write writes data from the specified filename to the floppy disk
func (c *Client) Write(filename string) error {
	return fmt.Errorf("Write() not yet implemented for SuperCard Pro adapter")
}

// Format formats the floppy disk
func (c *Client) Format() error {
	return fmt.Errorf("Format() not yet implemented for SuperCard Pro adapter")
}

// Erase erases the floppy disk
func (c *Client) Erase() error {
	return fmt.Errorf("Erase() not yet implemented for SuperCard Pro adapter")
}

// Close closes the serial port connection
func (c *Client) Close() error {
	if c.port != nil {
		return c.port.Close()
	}
	return nil
}
