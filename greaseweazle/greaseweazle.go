package greaseweazle

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sergev/floppy/adapter"

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

func init() {
	adapter.RegisterAdapter(VendorID, ProductID, NewClient)
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
	err = client.SetBusType()
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

// GetFluxStatus retrieves the status of the last read/write operation
func (c *Client) GetFluxStatus() error {
	cmd := []byte{CMD_GET_FLUX_STATUS, 2}
	return c.doCommand(cmd)
}

// Reset to initial state
func (c *Client) Reset() error {
	cmd := []byte{CMD_RESET, 2}
	return c.doCommand(cmd)
}

// Set bus type
func (c *Client) SetBusType() error {
	cmd := []byte{CMD_SET_BUS_TYPE, 3, BUS_IBMPC}
	return c.doCommand(cmd)
}

// Format formats the floppy disk
func (c *Client) Format() error {
	return fmt.Errorf("Format() not yet implemented for Greaseweazle adapter")
}
