package kryoflux

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"floppy/adapter"
	"floppy/hfe"
	"floppy/pll"

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

	// Default Sample Clock (Hz) - can be overridden by KFInfo blocks
	DefaultSampleClock = 24027428.57142857
)

// DecodedStreamData contains decoded flux transitions and index information
type DecodedStreamData struct {
	FluxTransitions []uint64 // Flux transition times in nanoseconds (relative to first index)
	IndexPulses     []uint64 // Index pulse times in nanoseconds
	SampleClock     float64  // Sample clock frequency in Hz
}

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
	// Use timeout wrapper to prevent blocking
	type result struct {
		data []byte
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		_, err := c.controlIn(RequestMotor, 1, false)
		if err != nil {
			resultChan <- result{nil, fmt.Errorf("failed to turn motor on: %w", err)}
			return
		}
		_, err = c.controlIn(RequestSide, uint16(side), false)
		if err != nil {
			resultChan <- result{nil, fmt.Errorf("failed to set side: %w", err)}
			return
		}
		_, err = c.controlIn(RequestTrack, uint16(track), false)
		if err != nil {
			resultChan <- result{nil, fmt.Errorf("failed to set track: %w", err)}
			return
		}
		resultChan <- result{nil, nil}
	}()

	select {
	case res := <-resultChan:
		return res.err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("motorOn timeout after 5 seconds")
	}
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
	validationIterations := 0
	maxValidationIterations := len(data) * 2 // Safety check
	for dataLen > 0 {
		validationIterations++
		if validationIterations > maxValidationIterations {
			state.failed = true
			state.failureReason = fmt.Sprintf("validation loop exceeded max iterations (%d)", maxValidationIterations)
			return false
		}
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

// captureStreamToMemory captures a stream from the device and returns the raw stream data
func (c *Client) captureStreamToMemory() ([]byte, error) {
	state := &streamState{
		complete:     false,
		failed:       false,
		resultFound:  false,
		currentPos:   0,
		skipCount:    0,
		firstOOBSeen: false,
	}

	var streamData []byte

	// Channel for reading data
	dataChan := make(chan []byte, AsyncReadBufferCount)
	errChan := make(chan error, 1)
	doneChan := make(chan bool, 1)
	readDoneChan := make(chan bool, 1)
	streamStarted := false

	// Start async read in goroutine
	// Use a timeout for each read operation to prevent indefinite blocking
	readTimeout := 3 * time.Second // Increased slightly to reduce false timeouts
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
				// Wrap read in timeout
				type readResult struct {
					length int
					err    error
				}
				readResultChan := make(chan readResult, 1)
				go func() {
					length, err := c.bulkIn.Read(buf)
					readResultChan <- readResult{length: length, err: err}
				}()

				var length int
				var err error
				select {
				case <-doneChan:
					return
				case res := <-readResultChan:
					length, err = res.length, res.err
				case <-time.After(readTimeout):
					// Read timeout - check if we should continue or abort
					select {
					case <-doneChan:
						return
					default:
						// If stream is complete, this is expected - return
						if state.complete {
							return
						}
						// Otherwise, send timeout error
						errChan <- fmt.Errorf("bulk read timeout after %v", readTimeout)
						return
					}
				}

				if err != nil {
					select {
					case <-doneChan:
						return
					default:
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
		<-readDoneChan
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}
	streamStarted = true

	// Process incoming data
	processing := true
	dataReceived := false
	maxTotalTime := 30 * time.Second // Absolute maximum time for stream capture (reduced from 60s)
	noDataTimeout := 5 * time.Second // Timeout if no data received for this duration (reduced from 10s)
	processingIterations := 0

	intervalTimeout := time.NewTimer(noDataTimeout)
	defer intervalTimeout.Stop()
	totalTimeout := time.NewTimer(maxTotalTime)
	defer totalTimeout.Stop()

	for processing {
		processingIterations++
		select {
		case data := <-dataChan:
			dataReceived = true
			// Reset interval timeout since we received data
			if !intervalTimeout.Stop() {
				<-intervalTimeout.C
			}
			intervalTimeout.Reset(noDataTimeout)
			// Validate the data first
			shouldContinue := c.validateStreamData(state, data)
			// Append the data if validation passed
			if !state.failed {
				streamData = append(streamData, data...)
			}
			// Stop processing if validation says to stop or if stream completed/failed
			if !shouldContinue || state.complete || state.failed {
				processing = false
			}
		case err := <-errChan:
			if state.complete {
				processing = false
				break
			}
			// Immediately stop stream and don't wait for goroutine
			doneChan <- true
			if streamStarted {
				c.controlIn(RequestStream, 0, true)
			}
			// Try to read from readDoneChan with timeout
			select {
			case <-readDoneChan:
			case <-time.After(100 * time.Millisecond):
				// Goroutine didn't exit, continue anyway
			}
			return nil, fmt.Errorf("failed to read stream data: %w", err)
		case <-intervalTimeout.C:
			// No data received for intervalTimeout duration - timeout
			// Stop stream and exit - don't try to continue
			doneChan <- true
			if streamStarted {
				c.controlIn(RequestStream, 0, true)
				streamStarted = false
			}

			// Don't wait indefinitely for goroutine - it might be stuck in a blocking read
			select {
			case <-readDoneChan:
			case <-time.After(200 * time.Millisecond):
				// Goroutine didn't exit in time - might be stuck, but we continue anyway
			}

			// If we have some data, return it anyway - might be a partial stream
			if len(streamData) > 0 {
				return streamData, nil
			}

			// No data received at all
			if !dataReceived {
				return nil, fmt.Errorf("stream read timeout: no data received within %v", noDataTimeout)
			}

			// We received data before but now timed out - return what we have
			return streamData, nil
		case <-totalTimeout.C:
			// Absolute maximum time exceeded - timeout
			doneChan <- true
			if streamStarted {
				c.controlIn(RequestStream, 0, true)
				streamStarted = false
			}
			// Don't wait indefinitely for goroutine
			select {
			case <-readDoneChan:
			case <-time.After(200 * time.Millisecond):
			}
			if len(streamData) > 0 {
				return streamData, nil
			}
			return nil, fmt.Errorf("stream read timeout: maximum time %v exceeded", maxTotalTime)
		}
	}

	// Drain any remaining data from channel
	draining := true
	for draining {
		select {
		case data := <-dataChan:
			c.validateStreamData(state, data)
			if !state.failed {
				streamData = append(streamData, data...)
			}
		default:
			draining = false
		}
	}

	// Signal goroutine to stop and wait for it to finish (with timeout)
	doneChan <- true
	select {
	case <-readDoneChan:
		// Goroutine exited normally
	case <-time.After(500 * time.Millisecond):
		// Goroutine didn't exit in time - might be stuck, but continue anyway
	}

	// Stop stream only if we started it successfully
	if streamStarted {
		_, err = c.controlIn(RequestStream, 0, true)
		if err != nil {
			// Log but don't return error
		}
	}

	if state.failed {
		if state.failureReason != "" {
			return nil, fmt.Errorf("stream validation failed: %s", state.failureReason)
		}
		return nil, fmt.Errorf("stream validation failed")
	}
	// Allow partial streams - we'll check if we have enough data in the decoder
	// If stream didn't complete but we have data, return it anyway
	if !state.complete && len(streamData) == 0 {
		return nil, fmt.Errorf("stream did not complete properly and no data received (resultFound=%v, currentPos=%d)", state.resultFound, state.currentPos)
	}

	return streamData, nil
}

// decodeKryoFluxStream decodes KryoFlux stream data to extract flux transitions and index pulses
func (c *Client) decodeKryoFluxStream(data []byte) (*DecodedStreamData, error) {

	result := &DecodedStreamData{
		SampleClock: DefaultSampleClock,
	}

	tickPeriodNs := 1e9 / result.SampleClock // Nanoseconds per tick
	ticksAccumulated := uint64(0)
	ovl16Count := uint64(0) // Count of consecutive Ovl16 blocks

	// Collect all flux transitions with their absolute times
	type fluxTransition struct {
		ticks uint64 // Absolute time in ticks
	}
	var allFluxTransitions []fluxTransition
	var indexPulses []uint64 // Index pulse times in nanoseconds

	i := 0
	maxIterations := len(data) * 2 // Safety check to prevent infinite loops
	iterationCount := 0
	for i < len(data) && iterationCount < maxIterations {
		iterationCount++
		val := data[i]

		if val <= 7 {
			// Flux2 block: 2-byte sequence
			if i+1 >= len(data) {
				return nil, fmt.Errorf("incomplete Flux2 block at offset %d", i)
			}
			fluxValue := (uint32(val) << 8) | uint32(data[i+1])
			fluxValue += uint32(ovl16Count) * 0x10000
			ovl16Count = 0
			ticksAccumulated += uint64(fluxValue)
			allFluxTransitions = append(allFluxTransitions, fluxTransition{ticks: ticksAccumulated})
			i += 2
		} else if val == 0x0b {
			// Ovl16 block: add 0x10000 to next flux value
			ovl16Count++
			i++
		} else if val == 0x0c {
			// Flux3 block: 3-byte sequence
			if i+2 >= len(data) {
				return nil, fmt.Errorf("incomplete Flux3 block at offset %d", i)
			}
			fluxValue := (uint32(data[i+1]) << 8) | uint32(data[i+2])
			fluxValue += uint32(ovl16Count) * 0x10000
			ovl16Count = 0
			ticksAccumulated += uint64(fluxValue)
			allFluxTransitions = append(allFluxTransitions, fluxTransition{ticks: ticksAccumulated})
			i += 3
		} else if val == 0x0d {
			// OOB block: 4-byte header + optional data
			if i+3 >= len(data) {
				return nil, fmt.Errorf("incomplete OOB header at offset %d", i)
			}
			oobType := data[i+1]
			oobSize := uint32(data[i+2]) | (uint32(data[i+3]) << 8)

			if oobType == 0x0d && oobSize == 0x0d0d {
				// EOF marker - stop processing
				break
			}

			if i+4+int(oobSize) > len(data) {
				return nil, fmt.Errorf("incomplete OOB data at offset %d", i)
			}

			// Handle StreamEnd block (type 0x03) - indicates stream has ended
			if oobType == 0x03 && oobSize >= 8 {
				// StreamEnd block: Stream Position (4 bytes), Result Code (4 bytes)
				resultCode := binary.LittleEndian.Uint32(data[i+8 : i+12])
				if resultCode != 0 {
					// Non-zero result code indicates an error, but we can still process what we have
					// Continue processing but break after handling this
				}
				// Break after StreamEnd to stop processing further
				i += 4 + int(oobSize)
				break
			}

			// Handle Index block (type 0x02)
			if oobType == 0x02 && oobSize >= 12 {
				// Index block: Stream Position (4 bytes), Sample Counter (4 bytes), Index Counter (4 bytes)
				sampleCounter := binary.LittleEndian.Uint32(data[i+4 : i+8])
				// Index time = time of last flux transition + sample counter
				indexTime := ticksAccumulated + uint64(sampleCounter)
				indexTimeNs := uint64(float64(indexTime) * tickPeriodNs)
				indexPulses = append(indexPulses, indexTimeNs)
			}

			// Handle KFInfo block (type 0x04) to extract sample clock
			if oobType == 0x04 && oobSize > 0 {
				infoData := string(data[i+4 : i+4+int(oobSize)])
				// Parse sck= value from info string
				if strings.Contains(infoData, "sck=") {
					parts := strings.Split(infoData, ",")
					for _, part := range parts {
						if strings.HasPrefix(part, "sck=") {
							sckStr := strings.TrimPrefix(part, "sck=")
							sckStr = strings.TrimSpace(sckStr)
							if sck, err := strconv.ParseFloat(sckStr, 64); err == nil {
								result.SampleClock = sck
								tickPeriodNs = 1e9 / result.SampleClock
							}
						}
					}
				}
			}

			i += 4 + int(oobSize)
		} else if val >= 0x0e {
			// Flux1 block: 1-byte (0x0E-0xFF)
			fluxValue := uint32(val)
			fluxValue += uint32(ovl16Count) * 0x10000
			ovl16Count = 0
			ticksAccumulated += uint64(fluxValue)
			allFluxTransitions = append(allFluxTransitions, fluxTransition{ticks: ticksAccumulated})
			i++
		} else {
			// NOP blocks: 0x08 (1 byte), 0x09 (2 bytes), 0x0a (3 bytes)
			if val == 0x08 {
				i++
			} else if val == 0x09 {
				i += 2
			} else if val == 0x0a {
				i += 3
			} else {
				return nil, fmt.Errorf("unknown block type 0x%02x at offset %d", val, i)
			}
		}
	}

	if iterationCount >= maxIterations {
		return nil, fmt.Errorf("decoder loop exceeded maximum iterations (possible infinite loop)")
	}

	// Now filter transitions to only include those between first and second index
	// We need to ensure all transitions right before the second index are included
	var fluxTransitions []uint64
	if len(indexPulses) >= 2 {
		firstIndexNs := indexPulses[0]
		secondIndexNs := indexPulses[1]

		// Calculate revolution duration
		revolutionDurationNs := secondIndexNs - firstIndexNs

		// Find the first flux transition after the second index pulse
		// This helps us determine the true end of the first revolution
		// We'll include transitions up to this point (if it's within a reasonable distance)
		var firstTransitionAfterSecondIndexNs uint64 = 0
		maxReasonableDistanceNs := revolutionDurationNs / 20 // 5% of revolution

		for _, flux := range allFluxTransitions {
			fluxTimeNs := uint64(float64(flux.ticks) * tickPeriodNs)
			if fluxTimeNs > secondIndexNs {
				// Found first transition after second index
				if firstTransitionAfterSecondIndexNs == 0 || fluxTimeNs < firstTransitionAfterSecondIndexNs {
					firstTransitionAfterSecondIndexNs = fluxTimeNs
				}
			}
		}

		// Determine the boundary: use the first transition after second index if it's close enough,
		// otherwise use a margin after the second index
		var secondIndexBoundaryNs uint64
		if firstTransitionAfterSecondIndexNs > 0 &&
			(firstTransitionAfterSecondIndexNs-secondIndexNs) <= maxReasonableDistanceNs {
			// Include the first transition after second index if it's close enough
			// This likely represents the end of the first revolution's data
			secondIndexBoundaryNs = firstTransitionAfterSecondIndexNs
		} else {
			// Fallback: use a margin (2% of revolution) after second index
			marginNs := revolutionDurationNs / 50 // 2% margin
			secondIndexBoundaryNs = secondIndexNs + marginNs
		}

		for _, flux := range allFluxTransitions {
			// Convert flux transition time from ticks to nanoseconds for comparison
			// This avoids precision loss from converting index times back to ticks
			fluxTimeNs := uint64(float64(flux.ticks) * tickPeriodNs)

			// Include all transitions from first index up to the determined boundary
			// This ensures we capture the complete revolution including sector 17 data
			if fluxTimeNs >= firstIndexNs && fluxTimeNs <= secondIndexBoundaryNs {
				// Convert to nanoseconds relative to first index
				transitionTime := fluxTimeNs - firstIndexNs
				fluxTransitions = append(fluxTransitions, transitionTime)
			}
		}
	} else if len(indexPulses) == 1 {
		// If only one index, use all transitions after it
		firstIndexNs := indexPulses[0]
		for _, flux := range allFluxTransitions {
			// Convert flux transition time from ticks to nanoseconds for comparison
			fluxTimeNs := uint64(float64(flux.ticks) * tickPeriodNs)
			if fluxTimeNs >= firstIndexNs {
				// Convert to nanoseconds relative to first index
				transitionTime := fluxTimeNs - firstIndexNs
				fluxTransitions = append(fluxTransitions, transitionTime)
			}
		}
	} else if len(indexPulses) == 0 && len(allFluxTransitions) > 0 {
		// If no index pulses found but we have transitions, use all transitions
		// This handles edge cases where index detection failed but data is present
		for _, flux := range allFluxTransitions {
			transitionTime := uint64(float64(flux.ticks) * tickPeriodNs)
			fluxTransitions = append(fluxTransitions, transitionTime)
		}
	}

	result.FluxTransitions = fluxTransitions
	result.IndexPulses = indexPulses

	return result, nil
}

// calculateRPMAndBitRate calculates RPM and bit rate from decoded stream data
func (c *Client) calculateRPMAndBitRate(decoded *DecodedStreamData) (uint16, uint16) {
	if len(decoded.IndexPulses) < 2 {
		return 300, 250 // Default RPM and bit rate
	}

	// Calculate RPM from index pulse intervals
	// IndexPulses contains absolute times, so subtract to get interval
	trackDurationNs := decoded.IndexPulses[1] - decoded.IndexPulses[0]
	rpm := 60e9 / float64(trackDurationNs)

	// Round to either 300 or 360 RPM
	var roundedRPM uint16
	if rpm < 330 {
		roundedRPM = 300
	} else {
		roundedRPM = 360
	}

	// Calculate bit rate from transition count and track duration
	transitionCount := uint64(len(decoded.FluxTransitions))
	bitsPerMsec := transitionCount * 1e6 / trackDurationNs

	// Round to standard floppy drive bitrates: 250, 500, or 1000 kbps
	var roundedBitRate uint16
	if bitsPerMsec < 375 {
		roundedBitRate = 250
	} else if bitsPerMsec < 750 {
		roundedBitRate = 500
	} else {
		roundedBitRate = 1000
	}

	return roundedRPM, roundedBitRate
}

// kfFluxIterator provides flux intervals from KryoFlux decoded stream data
// It implements pll.FluxSource interface
type kfFluxIterator struct {
	transitions     []uint64 // Absolute transition times in nanoseconds
	index           int      // Current index into transitions
	lastTime        uint64   // Last transition time (for calculating intervals)
	exhaustedCalled bool     // Track if NextFlux() has been called when exhausted
}

// NextFlux returns the next flux interval in nanoseconds (time until next transition)
// Returns 0 if no more transitions available
// Implements pll.FluxSource interface
func (fi *kfFluxIterator) NextFlux() uint64 {
	if fi.index >= len(fi.transitions) {
		fi.exhaustedCalled = true // Mark that we've been called when exhausted
		return 0                  // No more transitions
	}

	nextTime := fi.transitions[fi.index]
	interval := nextTime - fi.lastTime
	fi.lastTime = nextTime
	fi.index++
	return interval
}

// decodeFluxToMFM recovers raw MFM bitcells from KryoFlux decoded stream data using PLL,
// and returns MFM bitcells as bytes (bitcells packed MSB-first, not decoded data bits)
func (c *Client) decodeFluxToMFM(decoded *DecodedStreamData, bitRateKhz uint16) ([]byte, error) {
	if len(decoded.FluxTransitions) == 0 {
		return nil, fmt.Errorf("no flux transitions found")
	}

	// Create flux iterator from transition times
	fi := &kfFluxIterator{
		transitions: decoded.FluxTransitions,
		index:       0,
		lastTime:    0, // Start from time 0
	}

	// Initialize PLL
	pllState := &pll.State{}
	pll.Init(pllState, bitRateKhz)

	// Ignore first half-bit (as done in reference implementation)
	_ = pll.NextBit(pllState, fi)

	// Generate MFM bitcells using PLL algorithm
	var bitcells []bool
	pllIterations := 0
	transitionsLen := len(decoded.FluxTransitions)

	// Estimate max iterations: each transition generates roughly 1-2 bits
	// Each iteration generates 2 bits, so we need roughly transitions iterations
	// Add 50% margin for safety, but cap at reasonable maximum
	maxPLLIterations := transitionsLen + (transitionsLen / 2)
	if maxPLLIterations > 200000 {
		maxPLLIterations = 200000
	}

	// Track when we've consumed all or nearly all transitions
	// Once we've consumed this many, we should stop soon
	stopThreshold := transitionsLen
	if transitionsLen > 100 {
		// Allow small buffer - if transitions are being consumed slowly, we may not hit exact length
		stopThreshold = transitionsLen - 10
	}

	iterationsSinceLastTransition := 0
	maxIterationsWithoutTransition := 1000 // Stop if we generate 1000 iterations without consuming a transition

	for {
		pllIterations++

		// Track progress to detect if transitions are being consumed
		prevIndex := fi.index

		// Calculate consumed percentage for termination checks
		consumedPercentage := float64(fi.index) / float64(transitionsLen) * 100.0

		// Stop if we've generated excessive bits but consumed very few transitions (PLL stuck)
		// Check earlier: if we've generated 2x bits but consumed <5% transitions, PLL is likely stuck
		if len(bitcells) >= transitionsLen*2 && consumedPercentage < 5.0 {
			break
		}

		// Stop if we've consumed 75%+ transitions and generated excessive bits
		// Lowered from 80% to catch edge cases like empty tracks (79.9%)
		if consumedPercentage >= 75.0 && len(bitcells) >= transitionsLen*3 {
			break
		}

		if pllIterations > maxPLLIterations {
			return nil, fmt.Errorf("PLL loop exceeded maximum iterations (%d), possible infinite loop", maxPLLIterations)
		}

		// Check if transitions are exhausted or nearly exhausted BEFORE generating more bits
		if fi.index >= stopThreshold || fi.exhaustedCalled {
			// Transitions exhausted - stop immediately
			break
		}

		first := pll.NextBit(pllState, fi)
		second := pll.NextBit(pllState, fi)

		bitcells = append(bitcells, first)
		bitcells = append(bitcells, second)

		// Check if index advanced (transition was consumed)
		if fi.index > prevIndex {
			iterationsSinceLastTransition = 0
		} else {
			iterationsSinceLastTransition++
		}

		// Stop if we've generated many iterations without consuming a transition
		// This indicates all transitions have been consumed and we're just generating from accumulated flux
		// Check if we've consumed at least 75% of transitions OR reached stopThreshold (lowered from 80% for edge cases)
		if iterationsSinceLastTransition >= maxIterationsWithoutTransition {
			// We've gone many iterations without consuming a transition
			if fi.index >= stopThreshold || consumedPercentage >= 75.0 {
				// Either we've reached threshold OR consumed 75%+ of transitions
				break
			}
		}

		// Check again after NextBit calls (they may have advanced fi.index or called NextFlux when exhausted)
		if fi.index >= stopThreshold || fi.exhaustedCalled {
			// Transitions exhausted during this iteration - stop
			break
		}
	}

	if len(bitcells) == 0 {
		return nil, fmt.Errorf("no bitcells generated")
	}

	// Pack bitcells as bytes (MSB-first)
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
	// Configure device with default values (device=0, density=0, minTrack=0, maxTrack=83)
	err := c.configure(0, 0, 0, 83)
	if err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Initialize HFE disk structure
	NumberOfTracks := 82
	disk := &hfe.Disk{
		Header: hfe.Header{
			NumberOfTrack:       uint8(NumberOfTracks),
			NumberOfSide:        2,
			TrackEncoding:       hfe.ENC_ISOIBM_MFM,
			BitRate:             500,              // Will be calculated from flux data
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
		Tracks: make([]hfe.TrackData, NumberOfTracks),
	}

	// Iterate through cylinders and sides
	for cyl := 0; cyl < NumberOfTracks; cyl++ {
		for side := 0; side < 2; side++ {
			// Print progress message
			if cyl != 0 || side != 0 {
				fmt.Printf("\rReading track %d, side %d...", cyl, side)
			}

			// Turn on motor and position head
			err = c.motorOn(side, cyl)
			if err != nil {
				// Log error but continue - some tracks may be inaccessible
				fmt.Printf("\nWarning: failed to position head at track %d, side %d: %v\n", cyl, side, err)
				// Store empty data for this track
				if side == 0 {
					disk.Tracks[cyl].Side0 = []byte{}
				} else {
					disk.Tracks[cyl].Side1 = []byte{}
				}
				// Ensure cleanup
				c.streamOff()
				c.motorOff()
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Capture stream data to memory
			streamData, err := c.captureStreamToMemory()
			if err != nil {
				// Log error but continue with next track - some tracks may be unreadable
				fmt.Printf("\nWarning: failed to capture stream from track %d, side %d: %v\n", cyl, side, err)
				// Store empty data for this track
				if side == 0 {
					disk.Tracks[cyl].Side0 = []byte{}
				} else {
					disk.Tracks[cyl].Side1 = []byte{}
				}
				// Ensure stream is stopped and give device time to recover
				c.streamOff()
				c.motorOff()
				time.Sleep(100 * time.Millisecond) // Brief pause for device recovery
				continue
			}

			// Decode stream data to extract flux transitions
			decoded, err := c.decodeKryoFluxStream(streamData)
			if err != nil {
				// Log error but continue with next track
				fmt.Printf("\nWarning: failed to decode stream from track %d, side %d: %v\n", cyl, side, err)
				// Store empty data for this track
				if side == 0 {
					disk.Tracks[cyl].Side0 = []byte{}
				} else {
					disk.Tracks[cyl].Side1 = []byte{}
				}
				// Ensure cleanup
				c.streamOff()
				c.motorOff()
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Calculate RPM and BitRate from first track (cylinder 0, head 0)
			if cyl == 0 && side == 0 {
				calculatedRPM, calculatedBitRate := c.calculateRPMAndBitRate(decoded)
				fmt.Printf("Rotation Speed: %d RPM\n", calculatedRPM)
				fmt.Printf("Bit Rate: %d kbps\n", calculatedBitRate)

				disk.Header.FloppyRPM = calculatedRPM
				disk.Header.BitRate = calculatedBitRate
			}

			// Decode flux data to MFM bitstream
			mfmBitstream, err := c.decodeFluxToMFM(decoded, disk.Header.BitRate)
			if err != nil {
				// Log error but continue with next track
				fmt.Printf("\nWarning: failed to decode flux data to MFM from track %d, side %d: %v\n", cyl, side, err)
				// Store empty data for this track
				if side == 0 {
					disk.Tracks[cyl].Side0 = []byte{}
				} else {
					disk.Tracks[cyl].Side1 = []byte{}
				}
				// Ensure cleanup
				c.streamOff()
				c.motorOff()
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Store MFM bitstream in appropriate side
			if side == 0 {
				disk.Tracks[cyl].Side0 = mfmBitstream
			} else {
				disk.Tracks[cyl].Side1 = mfmBitstream
			}
		}
	}
	fmt.Printf(" Done\n")

	// Turn off motor
	err = c.motorOff()
	if err != nil {
		return fmt.Errorf("failed to turn off motor: %w", err)
	}

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
