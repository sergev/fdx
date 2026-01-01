package supercardpro

import (
	"encoding/binary"
	"floppy/adapter"
	"fmt"
	"io"
	"os"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

const (
	VendorID  = 0x0403
	ProductID = 0x6015
)

const baudRate = 115200

// SCP command codes
const (
	SCPCMD_SELA        = 0x80 // select drive A
	SCPCMD_SELB        = 0x81 // select drive B
	SCPCMD_DSELA       = 0x82 // deselect drive A
	SCPCMD_DSELB       = 0x83 // deselect drive B
	SCPCMD_MTRAON      = 0x84 // turn motor A on
	SCPCMD_MTRBON      = 0x85 // turn motor B on
	SCPCMD_MTRAOFF     = 0x86 // turn motor A off
	SCPCMD_MTRBOFF     = 0x87 // turn motor B off
	SCPCMD_SEEK0       = 0x88 // seek track 0
	SCPCMD_STEPTO      = 0x89 // step to specified track
	SCPCMD_SIDE        = 0x8d // select side
	SCPCMD_SETPARAMS   = 0x91 // set parameters
	SCPCMD_READFLUX    = 0xa0 // read flux level
	SCPCMD_GETFLUXINFO = 0xa1 // get info for last flux read
	SCPCMD_SENDRAM_USB = 0xa9 // send data from buffer to USB
	SCPCMD_SCPINFO     = 0xd0 // get SCP info
)

// SCP status codes
const (
	SCP_STATUS_OK = 0x4f // command successful
)

// FluxInfo contains information about a single revolution of flux data
type FluxInfo struct {
	IndexTime  uint32 // Index pulse time
	NrBitcells uint32 // Number of bitcells
}

// FluxData contains flux information and data for up to 5 revolutions
type FluxData struct {
	Info [5]FluxInfo // Information for up to 5 revolutions
	Data []byte      // Flux data (512KB raw bytes from device)
}

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
// For SCPCMD_SENDRAM_USB, reads 512KB of data before reading the response
func (c *Client) scpSend(cmd byte, data []byte, readData []byte) error {
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

	// Special handling for SENDRAM_USB: read 512KB before reading response
	if cmd == SCPCMD_SENDRAM_USB && readData != nil {
		_, err = io.ReadFull(c.port, readData)
		if err != nil {
			return fmt.Errorf("failed to read RAM data: %w", err)
		}
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

// selectDrive selects a drive and turns on its motor
func (c *Client) selectDrive(drive uint) error {
	// Select drive (SELA for drive 0, SELB for drive 1)
	var cmd byte = SCPCMD_SELA
	if drive == 1 {
		cmd = SCPCMD_SELB
	}
	err := c.scpSend(cmd, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to select drive %d: %w", drive, err)
	}

	// Turn on motor (MTRAON for drive 0, MTRBON for drive 1)
	var motorCmd byte = SCPCMD_MTRAON
	if drive == 1 {
		motorCmd = SCPCMD_MTRBON
	}
	err = c.scpSend(motorCmd, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to turn on motor for drive %d: %w", drive, err)
	}

	return nil
}

// deselectDrive deselects a drive and turns off its motor
func (c *Client) deselectDrive(drive uint) error {
	// Turn off motor (MTRAOFF for drive 0, MTRBOFF for drive 1)
	var motorCmd byte = SCPCMD_MTRAOFF
	if drive == 1 {
		motorCmd = SCPCMD_MTRBOFF
	}
	err := c.scpSend(motorCmd, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to turn off motor for drive %d: %w", drive, err)
	}

	// Deselect drive (DSELA for drive 0, DSELB for drive 1)
	var cmd byte = SCPCMD_DSELA
	if drive == 1 {
		cmd = SCPCMD_DSELB
	}
	err = c.scpSend(cmd, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to deselect drive %d: %w", drive, err)
	}

	return nil
}

// seekTrack seeks to the specified track
func (c *Client) seekTrack(track uint) error {
	// Calculate cylinder and side
	cyl := track >> 1
	side := track & 1

	// Seek to cylinder
	if cyl == 0 {
		err := c.scpSend(SCPCMD_SEEK0, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to seek to track 0: %w", err)
		}
	} else {
		cylByte := byte(cyl)
		err := c.scpSend(SCPCMD_STEPTO, []byte{cylByte}, nil)
		if err != nil {
			return fmt.Errorf("failed to step to cylinder %d: %w", cyl, err)
		}
	}

	// Select side
	sideByte := byte(side)
	err := c.scpSend(SCPCMD_SIDE, []byte{sideByte}, nil)
	if err != nil {
		return fmt.Errorf("failed to select side %d: %w", side, err)
	}

	// Apply seek settle delay (20ms default, simplified - no step_delay_ms subtraction)
	time.Sleep(20 * time.Millisecond)

	return nil
}

// readFlux reads flux data for the specified number of revolutions
func (c *Client) readFlux(nrRevs uint) (*FluxData, error) {
	// Prepare READFLUX command data: [nr_revs, 1] (1 = wait for index)
	info := []byte{byte(nrRevs), 1}
	err := c.scpSend(SCPCMD_READFLUX, info, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send READFLUX command: %w", err)
	}

	// Get flux info
	err = c.scpSend(SCPCMD_GETFLUXINFO, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send GETFLUXINFO command: %w", err)
	}

	// Read 40 bytes (5 revolutions × 8 bytes: 4 bytes index_time + 4 bytes nr_bitcells)
	infoData := make([]byte, 40)
	_, err = io.ReadFull(c.port, infoData)
	if err != nil {
		return nil, fmt.Errorf("failed to read flux info: %w", err)
	}

	// Parse flux info and convert from big-endian to host byte order
	fluxData := &FluxData{}
	for i := 0; i < 5; i++ {
		offset := i * 8
		fluxData.Info[i].IndexTime = binary.BigEndian.Uint32(infoData[offset : offset+4])
		fluxData.Info[i].NrBitcells = binary.BigEndian.Uint32(infoData[offset+4 : offset+8])
	}

	// Prepare RAM transfer command: 2 uint32_t values in big-endian
	// Offset: 0, Length: 512*1024
	ramCmd := make([]byte, 8)
	binary.BigEndian.PutUint32(ramCmd[0:4], 0)        // offset
	binary.BigEndian.PutUint32(ramCmd[4:8], 512*1024) // length

	// Allocate buffer for flux data (512KB)
	fluxData.Data = make([]byte, 512*1024)

	// Send SENDRAM_USB command - this will read 512KB into fluxData.Data
	err = c.scpSend(SCPCMD_SENDRAM_USB, ramCmd, fluxData.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to read flux data: %w", err)
	}

	return fluxData, nil
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
	// Select drive 0
	err := c.selectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	defer c.deselectDrive(0)

	// Open output file for writing
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Read tracks 0-159 (inclusive)
	for track := uint(0); track <= 159; track++ {
		// Print progress message
		fmt.Printf("\rReading track %d, side %d...", track / 2, track % 2)

		// Seek to track
		err = c.seekTrack(track)
		if err != nil {
			return fmt.Errorf("failed to seek to track %d: %w", track, err)
		}

		// Read flux data (2 revolutions)
		fluxData, err := c.readFlux(2)
		if err != nil {
			return fmt.Errorf("failed to read flux data from track %d: %w", track, err)
		}

		// Write raw flux data to file (bytes as received from device)
		_, err = file.Write(fluxData.Data)
		if err != nil {
			return fmt.Errorf("failed to write flux data for track %d: %w", track, err)
		}
	}
	fmt.Printf(" Done\n")

	return nil
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
