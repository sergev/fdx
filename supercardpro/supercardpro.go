package supercardpro

import (
	"encoding/binary"
	"floppy/adapter"
	"fmt"
	"io"
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
	SCPCMD_WRITEFLUX   = 0xa2 // write flux level
	SCPCMD_SENDRAM_USB = 0xa9 // send data from buffer to USB
	SCPCMD_LOADRAM_USB = 0xaa // load data from USB to buffer
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

func init() {
	adapter.RegisterAdapter(VendorID, ProductID, NewClient)
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

	// Special handling for LOADRAM_USB: write data after the initial 8-byte command
	// For LOADRAM_USB, the data parameter should contain [offset(be32), length(be32), actual_data...]
	// But we only send the 8-byte header in the command packet, then write the data separately
	// This is handled in loadRAM() which passes only the header to scpSend

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

// loadRAM loads flux data into device RAM buffer
// fluxData should be uint16 samples (big-endian), total length = nrSamples * 2 bytes
func (c *Client) loadRAM(fluxData []byte) error {
	if len(fluxData)%2 != 0 {
		return fmt.Errorf("flux data length must be even (uint16 samples)")
	}

	// Build LOADRAM_USB command header: [offset(be32), length(be32)]
	// The command packet contains only the 8-byte header [cmd, len=8, offset, length, checksum]
	// Then we write the actual data separately
	ramCmdHeader := make([]byte, 8)
	binary.BigEndian.PutUint32(ramCmdHeader[0:4], 0)                     // offset = 0
	binary.BigEndian.PutUint32(ramCmdHeader[4:8], uint32(len(fluxData))) // length

	// Build command packet manually: [cmd][len][data...][checksum]
	// We need to send the command packet, then the data, then read the response
	packet := make([]byte, 3+8)
	packet[0] = SCPCMD_LOADRAM_USB
	packet[1] = 8 // length of header data
	copy(packet[2:10], ramCmdHeader)

	// Calculate checksum: 0x4a + sum of cmd, len, and data bytes
	checksum := byte(0x4a)
	for i := 0; i < 10; i++ {
		checksum += packet[i]
	}
	packet[10] = checksum

	// Write command packet to serial port
	_, err := c.port.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to write LOADRAM_USB command packet: %w", err)
	}

	// Write the actual flux data (device expects this immediately after command packet)
	_, err = c.port.Write(fluxData)
	if err != nil {
		return fmt.Errorf("failed to write flux data: %w", err)
	}

	// Read the response (cmd_echo, status) that comes after the data
	response := make([]byte, 2)
	_, err = io.ReadFull(c.port, response)
	if err != nil {
		return fmt.Errorf("failed to read command response: %w", err)
	}

	// Validate echo matches sent command
	if response[0] != SCPCMD_LOADRAM_USB {
		return fmt.Errorf("command echo mismatch: sent 0x%02x, received 0x%02x", SCPCMD_LOADRAM_USB, response[0])
	}

	// Check status
	if response[1] != SCP_STATUS_OK {
		return fmt.Errorf("LOADRAM_USB command failed with status 0x%02x", response[1])
	}

	return nil
}

// Write flux data with wipe track flag enabled
// nrSamples is the number of uint16 flux samples
// nrRevs is the number of revolutions to write (1 for erase, typically 2-5 for normal writes)
func (c *Client) writeFlux(nrSamples uint32, nrRevs uint8) error {
	// Build WRITEFLUX command: [nr_samples(be32), nr_revs]
	// nr_revs: number of revolutions to write (1 = faster erase, 5 = multiple revolutions)
	writeCmd := make([]byte, 5)
	binary.BigEndian.PutUint32(writeCmd[0:4], nrSamples) // number of flux samples
	writeCmd[4] = nrRevs                                 // number of revolutions

	err := c.scpSend(SCPCMD_WRITEFLUX, writeCmd, nil)
	if err != nil {
		return fmt.Errorf("failed to write flux with wipe: %w", err)
	}

	return nil
}

// Close closes the serial port connection
func (c *Client) Close() error {
	if c.port != nil {
		return c.port.Close()
	}
	return nil
}

// Format formats the floppy disk
func (c *Client) Format() error {
	return fmt.Errorf("Format() not yet implemented for SuperCard Pro adapter")
}
