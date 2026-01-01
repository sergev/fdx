package kryoflux

import (
	_ "embed"
	"fmt"
	"os"
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

	// Stream reading constants
	AsyncReadBufferSize  = 6400
	AsyncReadBufferCount = 100
	StreamOnValue        = 0x601
)

// Client wraps a USB connection to a KryoFlux device
type Client struct {
	ctx         *gousb.Context
	dev         *gousb.Device
	intf        *gousb.Interface
	done        func()
	bulkOut     *gousb.OutEndpoint
	bulkIn      *gousb.InEndpoint
	deviceInfo1 string // From REQUEST_INFO index 1
	deviceInfo2 string // From REQUEST_INFO index 2
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
		ctx:     ctx,
		dev:     dev,
		intf:    intf,
		done:    done,
		bulkOut: bulkOut,
		bulkIn:  bulkIn,
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
			ctx:     ctx2,
			dev:     dev2,
			intf:    intf2,
			done:    done2,
			bulkOut: bulkOut2,
			bulkIn:  bulkIn2,
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
	fmt.Printf("KryoFlux Adapter Info:\n")
	fmt.Printf("%s\n", c.deviceInfo1)
	fmt.Printf("%s\n", c.deviceInfo2)

	// Query status
	//status, err := c.getStatus()
	//if err != nil {
	//	fmt.Printf("Status: Error querying status: %v\n", err)
	//} else {
	//	fmt.Printf("Status: %s\n", status)
	//}

}

// configure configures the device with the specified parameters
func (c *Client) configure(device, density, minTrack, maxTrack int) error {
	_, err := c.controlIn(RequestDevice, uint16(device), false)
	if err != nil {
		return fmt.Errorf("failed to set device: %w", err)
	}

	_, err = c.controlIn(RequestDensity, uint16(density), false)
	if err != nil {
		return fmt.Errorf("failed to set density: %w", err)
	}

	_, err = c.controlIn(RequestMinTrack, uint16(minTrack), false)
	if err != nil {
		return fmt.Errorf("failed to set min track: %w", err)
	}

	_, err = c.controlIn(RequestMaxTrack, uint16(maxTrack), false)
	if err != nil {
		return fmt.Errorf("failed to set max track: %w", err)
	}

	return nil
}

// motorOn turns on the motor and positions the head at the specified side and track
func (c *Client) motorOn(side, track int) error {
	_, err := c.controlIn(RequestMotor, 1, false)
	if err != nil {
		return fmt.Errorf("failed to turn motor on: %w", err)
	}

	_, err = c.controlIn(RequestSide, uint16(side), false)
	if err != nil {
		return fmt.Errorf("failed to set side: %w", err)
	}

	_, err = c.controlIn(RequestTrack, uint16(track), false)
	if err != nil {
		return fmt.Errorf("failed to set track: %w", err)
	}

	return nil
}

// motorOff turns off the motor
func (c *Client) motorOff() error {
	_, err := c.controlIn(RequestMotor, 0, false)
	if err != nil {
		return fmt.Errorf("failed to turn motor off: %w", err)
	}
	return nil
}

// streamOn starts the stream
func (c *Client) streamOn() error {
	_, err := c.controlIn(RequestStream, StreamOnValue, false)
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}
	return nil
}

// streamOff stops the stream
func (c *Client) streamOff() error {
	_, err := c.controlIn(RequestStream, 0, false)
	if err != nil {
		return fmt.Errorf("failed to stop stream: %w", err)
	}
	return nil
}

// streamState tracks the state during stream validation
type streamState struct {
	complete      bool
	failed        bool
	resultFound   bool
	currentPos    uint32
	skipCount     uint32
	failureReason string
	firstOOBSeen  bool // Track if we've seen the first OOB marker with position
}

// validateStreamData validates KryoFlux stream data according to the format specification
// Returns true if validation should continue, false if stream is complete or failed
func (c *Client) validateStreamData(state *streamState, data []byte) bool {
	if state.complete || state.failed {
		return false
	}

	dataLen := uint32(len(data))
	offset := uint32(0)

	// Handle skip count from previous incomplete sequence
	if state.skipCount > 0 {
		n := state.skipCount
		if n > dataLen {
			n = dataLen
		}
		state.skipCount -= n
		state.currentPos += n
		offset += n
		dataLen -= n
	}

	// Process the data
	for dataLen > 0 {
		if offset >= uint32(len(data)) {
			break
		}
		val := data[offset]

		if val <= 7 {
			// Value: 2-byte sequence
			if dataLen < 2 {
				state.skipCount = 2 - dataLen
				state.currentPos += dataLen
				return true
			}
			state.currentPos += 2
			offset += 2
			dataLen -= 2
		} else if val >= 0xe {
			// Sample: 1-byte
			state.currentPos++
			offset++
			dataLen--
		} else {
			switch val {
			case 0x08, 0x09, 0x0a:
				// Nop1-Nop3: variable length (1-3 bytes)
				noffset := int(val - 7)
				if dataLen < uint32(noffset) {
					state.skipCount = uint32(noffset) - dataLen
					state.currentPos += dataLen
					return true
				}
				state.currentPos += uint32(noffset)
				offset += uint32(noffset)
				dataLen -= uint32(noffset)
			case 0x0b:
				// Overflow16: 1-byte
				state.currentPos++
				offset++
				dataLen--
			case 0x0c:
				// Value16: 3-byte sequence
				if dataLen < 3 {
					state.skipCount = 3 - dataLen
					state.currentPos += dataLen
					return true
				}
				state.currentPos += 3
				offset += 3
				dataLen -= 3
			case 0x0d:
				// OOB marker: 4-byte header + data
				if dataLen < 4 {
					state.failed = true
					state.failureReason = "no room for OOB header"
					return false
				}
				oobType := data[offset+1]
				oobSize := uint32(data[offset+2]) | (uint32(data[offset+3]) << 8)

				if oobType == 0x0d && oobSize == 0x0d0d {
					// End of stream marker
					if !state.resultFound {
						state.failed = true
						state.failureReason = "end of data marker encountered before end of stream marker"
						return false
					}
					state.complete = true
					// Return false to stop processing (callback returns !stream_complete)
					return false
				}

				if dataLen-4 < oobSize {
					state.failed = true
					state.failureReason = "no room for OOB data"
					return false
				}

				// Validate stream position for type 1 or 3
				if oobType == 1 || oobType == 3 {
					if oobSize < 4 {
						state.failed = true
						state.failureReason = "no room for stream position"
						return false
					}
					streamPos := uint32(data[offset+4]) |
						(uint32(data[offset+5]) << 8) |
						(uint32(data[offset+6]) << 16) |
						(uint32(data[offset+7]) << 24)
					if !state.firstOOBSeen {
						// First OOB marker with position - sync our position to it
						// This handles the case where the device sends some data before the first OOB marker
						state.currentPos = streamPos
						state.firstOOBSeen = true
					} else if streamPos != state.currentPos {
						state.failed = true
						state.failureReason = fmt.Sprintf("bad stream position %d != %d", streamPos, state.currentPos)
						return false
					}
				}

				// Check result for type 3
				if oobType == 3 {
					if oobSize < 8 {
						state.failed = true
						state.failureReason = "no room for result value"
						return false
					}
					state.resultFound = true
					result := uint32(data[offset+8]) |
						(uint32(data[offset+9]) << 8) |
						(uint32(data[offset+10]) << 16) |
						(uint32(data[offset+11]) << 24)
					if result != 0 {
						state.failed = true
						switch result {
						case 1:
							state.failureReason = "buffering problem - data transfer delivery to host could not keep up with disk read"
						case 2:
							state.failureReason = "no index signal detected"
						default:
							state.failureReason = fmt.Sprintf("unknown stream end result %d", result)
						}
						return false
					}
				}

				// OOB markers are metadata and don't count toward stream position
				// Just skip over them
				offset += oobSize + 4
				dataLen -= oobSize + 4
			default:
				// Unknown/invalid marker (should be 0x08-0x0d, but we got something else)
				state.failed = true
				state.failureReason = fmt.Sprintf("unknown/invalid marker: 0x%02x", val)
				return false
			}
		}
	}

	return !state.complete && !state.failed
}

// writePreamble writes the stream preamble with timestamp to the file
func (c *Client) writePreamble(file *os.File) error {
	now := time.Now()
	timestamp := fmt.Sprintf("host_date=%04d.%02d.%02d, host_time=%02d:%02d:%02d",
		now.Year(), int(now.Month()), now.Day(),
		now.Hour(), now.Minute(), now.Second())

	buf := make([]byte, 4+len(timestamp)+1)
	buf[0] = 0x0d
	buf[1] = 4
	buf[2] = byte(len(timestamp) + 1)
	buf[3] = 0
	copy(buf[4:], timestamp)
	buf[4+len(timestamp)] = 0

	_, err := file.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write preamble: %w", err)
	}
	return nil
}

// captureStream captures a stream from the device and writes it to the file
func (c *Client) captureStream(file *os.File) error {
	state := &streamState{
		complete:     false,
		failed:       false,
		resultFound:  false,
		currentPos:   0,
		skipCount:    0,
		firstOOBSeen: false,
	}

	// Channel for reading data
	dataChan := make(chan []byte, AsyncReadBufferCount)
	errChan := make(chan error, 1)
	doneChan := make(chan bool, 1)
	readDoneChan := make(chan bool, 1)
	streamStarted := false

	// Start async read in goroutine
	go func() {
		defer func() {
			readDoneChan <- true
		}()
		buf := make([]byte, AsyncReadBufferSize)
		for {
			select {
			case <-doneChan:
				return
			default:
				length, err := c.bulkIn.Read(buf)
				if err != nil {
					// Check if we're done - if so, don't report the error
					select {
					case <-doneChan:
						return
					default:
						// If stream completed successfully, EOF/error might be expected
						if state.complete {
							return
						}
						errChan <- err
						return
					}
				}
				if length == 0 {
					continue
				}
				data := make([]byte, length)
				copy(data, buf[:length])
				select {
				case dataChan <- data:
				case <-doneChan:
					return
				}
			}
		}
	}()

	// Start stream
	err := c.streamOn()
	if err != nil {
		doneChan <- true
		<-readDoneChan // Wait for goroutine to finish
		return fmt.Errorf("failed to start stream: %w", err)
	}
	streamStarted = true

	// Process incoming data
	processing := true
	for processing {
		select {
		case data := <-dataChan:
			// Validate the data first
			shouldContinue := c.validateStreamData(state, data)
			// Write the data if validation passed (even if stream is now complete)
			if !state.failed {
				_, err := file.Write(data)
				if err != nil {
					doneChan <- true
					<-readDoneChan // Wait for goroutine to finish
					// Try to stop stream silently if it was started
					if streamStarted {
						c.controlIn(RequestStream, 0, true)
					}
					return fmt.Errorf("failed to write stream data: %w", err)
				}
			}
			// Stop processing if validation says to stop or if stream completed/failed
			if !shouldContinue || state.complete || state.failed {
				processing = false
			}
		case err := <-errChan:
			// If stream is complete, the error might be expected (device stopped sending)
			if state.complete {
				processing = false
				break
			}
			doneChan <- true
			<-readDoneChan // Wait for goroutine to finish
			// Try to stop stream silently if it was started
			if streamStarted {
				c.controlIn(RequestStream, 0, true)
			}
			return fmt.Errorf("failed to read stream data: %w", err)
		case <-time.After(30 * time.Second):
			// Timeout after 30 seconds
			doneChan <- true
			<-readDoneChan // Wait for goroutine to finish
			// Try to stop stream silently if it was started
			if streamStarted {
				c.controlIn(RequestStream, 0, true)
			}
			return fmt.Errorf("stream read timeout")
		}
	}

	// Drain any remaining data from channel
	draining := true
	for draining {
		select {
		case data := <-dataChan:
			// Validate and write remaining data
			c.validateStreamData(state, data)
			if !state.failed {
				file.Write(data)
			}
		default:
			draining = false
		}
	}

	// Signal goroutine to stop and wait for it to finish
	doneChan <- true
	<-readDoneChan

	// Stop stream only if we started it successfully
	if streamStarted {
		// Use silent mode to avoid errors if device is already stopped or in error state
		_, err = c.controlIn(RequestStream, 0, true)
		if err != nil {
			// Don't fail if stream is already stopped or device doesn't respond
			// This can happen if the device is in an error state or stream already ended
			// Log but don't return error
		}
	}

	if state.failed {
		if state.failureReason != "" {
			return fmt.Errorf("stream validation failed: %s", state.failureReason)
		}
		return fmt.Errorf("stream validation failed")
	}
	if !state.complete {
		return fmt.Errorf("stream did not complete properly (resultFound=%v, currentPos=%d)", state.resultFound, state.currentPos)
	}

	return nil
}

// Read reads the entire floppy disk and writes it to the specified filename
func (c *Client) Read(filename string) error {
	// Configure device with default values (device=0, density=0, minTrack=0, maxTrack=83)
	err := c.configure(0, 0, 0, 83)
	if err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Open output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Loop through tracks (0-79) and sides (0-1)
	for track := 0; track < 80; track++ {
		for side := 0; side < 2; side++ {
                        // Print progress message
                        fmt.Printf("\rReading track %d, side %d...", track, side)

			// Turn on motor and position head
			err = c.motorOn(side, track)
			if err != nil {
				return fmt.Errorf("failed to position head at track %d, side %d: %w", track, side, err)
			}

			// Write preamble for this track/side
			err = c.writePreamble(file)
			if err != nil {
				return fmt.Errorf("failed to write preamble for track %d, side %d: %w", track, side, err)
			}

			// Capture stream data
			err = c.captureStream(file)
			if err != nil {
				return fmt.Errorf("failed to capture stream from track %d, side %d: %w", track, side, err)
			}
		}
	}
	fmt.Printf(" Done\n")

	// Turn off motor
	err = c.motorOff()
	if err != nil {
		return fmt.Errorf("failed to turn off motor: %w", err)
	}

	return nil
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
