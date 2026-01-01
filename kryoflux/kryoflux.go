package kryoflux

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	"floppy/adapter"

	"github.com/google/gousb"
	"go.bug.st/serial/enumerator"
)

//go:embed firmware.bin
var firmwareData []byte

const (
	VendorID  = 0x03eb
	ProductID = 0x6124
	Interface = 1

	EndpointBulkOut = 0x01
	EndpointBulkIn  = 0x82

	ControlRequestType = 0xc3 // REQTYPE_IN_VENDOR_OTHER

	RequestReset    = 0x05
	RequestDevice   = 0x06
	RequestMotor    = 0x07
	RequestDensity  = 0x08
	RequestSide     = 0x09
	RequestTrack    = 0x0a
	RequestStream   = 0x0b
	RequestMinTrack = 0x0c
	RequestMaxTrack = 0x0d
	RequestStatus   = 0x80
	RequestInfo     = 0x81

	FWLoadAddress    = 0x00202000
	FWWriteChunkSize = 16384
	FWReadChunkSize  = 6400

	ControlTimeout = 5 * time.Second // Timeout for USB control transfers (matches legacy C code)
)

// Client wraps a USB connection to a KryoFlux device
type Client struct {
	ctx             *gousb.Context
	dev             *gousb.Device
	intf            *gousb.Interface
	done            func()
	bulkOut         *gousb.OutEndpoint
	bulkIn          *gousb.InEndpoint
	deviceInfo1     string // From REQUEST_INFO index 1
	deviceInfo2     string // From REQUEST_INFO index 2
}

// NewClient creates a new KryoFlux client using USB communication
// The portDetails parameter is ignored as KryoFlux uses USB directly
func NewClient(portDetails *enumerator.PortDetails) (adapter.FloppyAdapter, error) {
	ctx := gousb.NewContext()

	// Open device by VID/PID using OpenDevices
	// Compare as uint16 since DeviceDesc.Vendor/Product need uint16 comparison
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return uint16(desc.Vendor) == VendorID && uint16(desc.Product) == ProductID
	})
	if err != nil {
		ctx.Close()
		return nil, fmt.Errorf("failed to enumerate USB devices: %w", err)
	}
	if len(devs) == 0 {
		ctx.Close()
		return nil, fmt.Errorf("KryoFlux device not found (VID=0x%04X PID=0x%04X)", VendorID, ProductID)
	}

	// Use the first matching device
	dev := devs[0]
	// Close any additional devices if multiple were found
	for i := 1; i < len(devs); i++ {
		devs[i].Close()
	}

	// Get config 1 and claim interface 1 (as per C code: KRYOFLUX_INTERFACE = 1)
	cfg, err := dev.Config(1)
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("failed to get config 1: %w", err)
	}

	intf, err := cfg.Interface(Interface, 0)
	if err != nil {
		cfg.Close()
		dev.Close()
		return nil, fmt.Errorf("failed to claim interface %d: %w", Interface, err)
	}

	// Create done function that closes interface and config
	done := func() {
		intf.Close()
		cfg.Close()
	}

	// Get bulk endpoints
	bulkOut, err := intf.OutEndpoint(EndpointBulkOut)
	if err != nil {
		done()
		dev.Close()
		return nil, fmt.Errorf("failed to open bulk out endpoint: %w", err)
	}

	bulkIn, err := intf.InEndpoint(EndpointBulkIn)
	if err != nil {
		done()
		dev.Close()
		return nil, fmt.Errorf("failed to open bulk in endpoint: %w", err)
	}

	client := &Client{
		ctx:          ctx,
		dev:          dev,
		intf:         intf,
		done:         done,
		bulkOut:      bulkOut,
		bulkIn:       bulkIn,
	}

	// Check if firmware is present
	fwPresent, err := client.checkFirmwarePresent()
	if err != nil {
		fwPresent = false
	}

	if !fwPresent {
                fmt.Printf("Uploading KryoFlux firmware...\n")

		// Upload firmware
		err = client.uploadFirmware()
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to upload firmware: %w", err)
		}

		// Close interface and device explicitly
		done()
		dev.Close()
		ctx.Close()

		// Wait for device re-enumeration
		time.Sleep(1 * time.Second)

		// Reopen device
		ctx2 := gousb.NewContext()
		devs2, err := ctx2.OpenDevices(func(desc *gousb.DeviceDesc) bool {
			return uint16(desc.Vendor) == VendorID && uint16(desc.Product) == ProductID
		})
		if err != nil {
			ctx2.Close()
			return nil, fmt.Errorf("failed to reopen device after firmware upload: %w", err)
		}
		if len(devs2) == 0 {
			ctx2.Close()
			return nil, fmt.Errorf("device not found after firmware upload")
		}

		dev2 := devs2[0]
		// Close any additional devices if multiple were found
		for i := 1; i < len(devs2); i++ {
			devs2[i].Close()
		}

		// Get config 1 and claim interface 1
		cfg2, err := dev2.Config(1)
		if err != nil {
			dev2.Close()
			ctx2.Close()
			return nil, fmt.Errorf("failed to get config 1 after firmware upload: %w", err)
		}

		// Claim interface 1
		// Retry in case device isn't fully ready after re-enumeration
		var intf2 *gousb.Interface
		const maxInterfaceRetries = 25
		const interfaceRetryDelay = 200 * time.Millisecond
		for retry := 0; retry < maxInterfaceRetries; retry++ {
			intf2, err = cfg2.Interface(Interface, 0)
			if err == nil {
				break
			}
			if retry < maxInterfaceRetries-1 {
				time.Sleep(interfaceRetryDelay)
			}
		}
		if err != nil {
			cfg2.Close()
			dev2.Close()
			ctx2.Close()
			return nil, fmt.Errorf("failed to claim interface %d after firmware upload (tried %d times): %w", Interface, maxInterfaceRetries, err)
		}

		done2 := func() {
			intf2.Close()
			cfg2.Close()
		}

		bulkOut2, err := intf2.OutEndpoint(EndpointBulkOut)
		if err != nil {
			done2()
			dev2.Close()
			ctx2.Close()
			return nil, fmt.Errorf("failed to open bulk out endpoint after firmware upload: %w", err)
		}

		bulkIn2, err := intf2.InEndpoint(EndpointBulkIn)
		if err != nil {
			done2()
			dev2.Close()
			ctx2.Close()
			return nil, fmt.Errorf("failed to open bulk in endpoint after firmware upload: %w", err)
		}

		client = &Client{
			ctx:          ctx2,
			dev:          dev2,
			intf:         intf2,
			done:         done2,
			bulkOut:      bulkOut2,
			bulkIn:       bulkIn2,
		}

		// Verify firmware is now present
		fwPresent, err = client.checkFirmwarePresent()
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to verify firmware after upload: %w", err)
		}
		if !fwPresent {
			client.Close()
			return nil, fmt.Errorf("firmware not present after upload")
		}
	}

	// Reset device and get info
	err = client.reset()
	if err != nil {
		// Don't fail completely if reset fails - device might still work
		// client.Close()
		return nil, fmt.Errorf("failed to reset device: %w", err)
	}

	return client, nil
}

// controlIn performs a control transfer IN request
func (c *Client) controlIn(request byte, index uint16, silent bool) ([]byte, error) {
	buf := make([]byte, 512)
	length, err := c.dev.Control(ControlRequestType, request, 0, index, buf)
	if err != nil {
		if !silent {
			return nil, fmt.Errorf("control transfer failed: %w", err)
		}
		return nil, err
	}

	if length > 512 {
		length = 512
	}
	buf = buf[:length]

	// Parse response: expect text ending with "=index"
	// Null terminate for string operations
	responseBuf := make([]byte, length+1)
	copy(responseBuf, buf[:length])
	responseBuf[length] = 0
	response := string(responseBuf[:length])

	eqIdx := strings.IndexByte(response, '=')
	if eqIdx >= 0 {
		valueStr := response[eqIdx+1:]
		// Extract just the leading number (may be followed by comma and more data)
		// e.g., "1, name=KryoFlux..." should extract "1"
		valueStr = strings.TrimSpace(valueStr)
		// Find the first non-digit character (comma, space, etc.)
		endIdx := 0
		for endIdx < len(valueStr) && (valueStr[endIdx] >= '0' && valueStr[endIdx] <= '9') {
			endIdx++
		}
		if endIdx > 0 {
			valueStr = valueStr[:endIdx]
		}
		value, err := strconv.ParseInt(valueStr, 10, 16)
		if err != nil || int(value) != int(index&0xff) {
			if !silent {
				return nil, fmt.Errorf("device request failed: response value %s does not match index %d", valueStr, index&0xff)
			}
			return nil, fmt.Errorf("device request validation failed")
		}
		return buf[:length], nil
	}

	// If no '=' found, still return the data (might be valid in some cases)
	return buf[:length], nil
}

// controlInWithTimeout performs a control transfer IN request with a timeout
// This wraps the USB control transfer to prevent hanging if the device doesn't respond
func (c *Client) controlInWithTimeout(request byte, index uint16, silent bool) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}

	// Channel to receive the result from the goroutine
	resultChan := make(chan result, 1)

	// Execute control transfer in a goroutine
	go func() {
		data, err := c.controlIn(request, index, silent)
		resultChan <- result{data: data, err: err}
	}()

	// Wait for either completion or timeout
	select {
	case res := <-resultChan:
		return res.data, res.err
	case <-time.After(ControlTimeout):
		return nil, fmt.Errorf("control transfer timeout after %v", ControlTimeout)
	}
}

// checkFirmwarePresent checks if firmware is present by querying REQUEST_STATUS
func (c *Client) checkFirmwarePresent() (bool, error) {
	// Try twice to get stable result (as per C code)
	lastPresent, err := c.tryCheckStatus()
	if err != nil {
		return false, nil // Return false if error (firmware not present)
	}

	// Check up to 10 times to avoid infinite loop
	for i := 0; i < 10; i++ {
		present, err := c.tryCheckStatus()
		if err != nil {
			return false, nil
		}
		if present == lastPresent {
			return present, nil
		}
		lastPresent = present
	}

	// If we couldn't get a stable result, return the last value
	return lastPresent, nil
}

// tryCheckStatus attempts to check status (silent version)
func (c *Client) tryCheckStatus() (bool, error) {
	_, err := c.controlInWithTimeout(RequestStatus, 0, true)
	return err == nil, err
}

// sendBootloaderString sends a string to the bootloader via bulk out
func (c *Client) sendBootloaderString(s string) error {
	data := []byte(s)
	_, err := c.bulkOut.Write(data)
	return err
}

// recvBootloaderString receives a string from the bootloader via bulk in
func (c *Client) recvBootloaderString(size int) (string, error) {
	buf := make([]byte, size)
	tot := 0
	for tot < size {
		length, err := c.bulkIn.Read(buf[tot:])
		if err != nil {
			return "", err
		}
		tot += length
		if tot >= 2 && buf[tot-1] == 0xd && buf[tot-2] == 0xa {
			break
		}
	}
	if tot >= size {
		tot = size - 1
	}
	return string(buf[:tot]), nil
}

// uploadFirmware uploads firmware to the device
func (c *Client) uploadFirmware() error {
	// Use embedded firmware
	fwData := firmwareData
	fwSize := uint32(len(fwData))

	// Query bootloader with N#
	err := c.sendBootloaderString("N#")
	if err != nil {
		return fmt.Errorf("failed to send N# command: %w", err)
	}
	_, err = c.recvBootloaderString(512)
	if err != nil {
		return fmt.Errorf("failed to query bootloader (N#): %w", err)
	}

	// Query bootloader with V#
	err = c.sendBootloaderString("V#")
	if err != nil {
		return fmt.Errorf("failed to send V# command: %w", err)
	}
	_, err = c.recvBootloaderString(512)
	if err != nil {
		return fmt.Errorf("failed to query bootloader (V#): %w", err)
	}

	// Send Set command
	setCmd := fmt.Sprintf("S%08x,%08x#", FWLoadAddress, fwSize)
	err = c.sendBootloaderString(setCmd)
	if err != nil {
		return fmt.Errorf("failed to send Set command: %w", err)
	}

	// Write firmware in chunks
	for offs := uint32(0); offs < fwSize; offs += FWWriteChunkSize {
		chunkSize := int(FWWriteChunkSize)
		if int(offs)+chunkSize > int(fwSize) {
			chunkSize = int(fwSize - offs)
		}
		_, err := c.bulkOut.Write(fwData[int(offs) : int(offs)+chunkSize])
		if err != nil {
			return fmt.Errorf("failed to write firmware chunk at offset %d: %w", offs, err)
		}
	}

	// Send Read command to verify
	readCmd := fmt.Sprintf("R%08x,%08x#", FWLoadAddress, fwSize)
	err = c.sendBootloaderString(readCmd)
	if err != nil {
		return fmt.Errorf("failed to send Read command: %w", err)
	}

	// Verify firmware by reading it back
	verifyBuf := make([]byte, FWReadChunkSize)
	for offs := uint32(0); offs < fwSize; {
		chunkSize := int(FWReadChunkSize)
		if int(offs)+chunkSize > int(fwSize) {
			chunkSize = int(fwSize - offs)
		}

		length, err := c.bulkIn.Read(verifyBuf[:chunkSize])
		if err != nil {
			return fmt.Errorf("failed to read firmware chunk for verification at offset %d: %w", offs, err)
		}

		if length > 0 {
			// Compare with original firmware
			if length > chunkSize {
				length = chunkSize
			}
			for i := 0; i < length; i++ {
				if verifyBuf[i] != fwData[int(offs)+i] {
					return fmt.Errorf("firmware verification failed at offset %d", offs+uint32(i))
				}
			}
		}

		offs += uint32(length)
	}

	// Send Go command to execute firmware
	goCmd := fmt.Sprintf("G%08x#", FWLoadAddress)
	err = c.sendBootloaderString(goCmd)
	if err != nil {
		return fmt.Errorf("failed to send Go command: %w", err)
	}

	return nil
}

// reset resets the device and queries INFO requests
func (c *Client) reset() error {
	// Reset request
	_, err := c.controlIn(RequestReset, 0, false)
	if err != nil {
		return fmt.Errorf("reset request failed: %w", err)
	}

	// Get INFO 1
	info1, err := c.controlIn(RequestInfo, 1, false)
	if err != nil {
		return fmt.Errorf("info request 1 failed: %w", err)
	}
	c.deviceInfo1 = strings.TrimSpace(string(info1))

	// Get INFO 2
	info2, err := c.controlIn(RequestInfo, 2, false)
	if err != nil {
		return fmt.Errorf("info request 2 failed: %w", err)
	}
	c.deviceInfo2 = strings.TrimSpace(string(info2))

	return nil
}

// getInfo queries REQUEST_INFO with the given index
func (c *Client) getInfo(index uint16) (string, error) {
	data, err := c.controlIn(RequestInfo, index, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// getStatus queries REQUEST_STATUS
func (c *Client) getStatus() (string, error) {
	data, err := c.controlIn(RequestStatus, 0, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// PrintStatus prints KryoFlux status information to stdout
func (c *Client) PrintStatus() {
	fmt.Printf("KryoFlux Adapter\n")

	// Query status
	status, err := c.getStatus()
	if err != nil {
		fmt.Printf("Status: Error querying status: %v\n", err)
	} else {
		fmt.Printf("Status: %s\n", status)
	}

	// Query INFO 1
	info1, err := c.getInfo(1)
	if err != nil {
		fmt.Printf("Device Info 1: Error querying info: %v\n", err)
	} else {
		fmt.Printf("Device Info 1: %s\n", info1)
	}

	// Query INFO 2
	info2, err := c.getInfo(2)
	if err != nil {
		fmt.Printf("Device Info 2: Error querying info: %v\n", err)
	} else {
		fmt.Printf("Device Info 2: %s\n", info2)
	}
}

// Read reads the entire floppy disk and writes it to the specified filename
func (c *Client) Read(filename string) error {
	return fmt.Errorf("Read() not yet implemented for KryoFlux adapter")
}

// Write writes data from the specified filename to the floppy disk
func (c *Client) Write(filename string) error {
	return fmt.Errorf("Write() not yet implemented for KryoFlux adapter")
}

// Format formats the floppy disk
func (c *Client) Format() error {
	return fmt.Errorf("Format() not yet implemented for KryoFlux adapter")
}

// Erase erases the floppy disk
func (c *Client) Erase() error {
	return fmt.Errorf("Erase() not yet implemented for KryoFlux adapter")
}

// Close closes the USB connection
func (c *Client) Close() error {
	if c.done != nil {
		c.done()
	}
	if c.dev != nil {
		c.dev.Close()
	}
	if c.ctx != nil {
		return c.ctx.Close()
	}
	return nil
}
