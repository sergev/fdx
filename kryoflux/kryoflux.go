package kryoflux

import (
	"fmt"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
	"floppy/adapter"
)

const (
	VendorID  = 0x03eb
	ProductID = 0x6124
)

const baudRate = 115200

// Client wraps a serial port connection to a KryoFlux device
type Client struct {
	port         serial.Port
	serialNumber string
}

// NewClient creates a new KryoFlux client using the provided port details
// It opens the serial port and initializes the connection
func NewClient(portDetails *enumerator.PortDetails) (adapter.FloppyAdapter, error) {
	// Open the serial port
	mode := &serial.Mode{
		BaudRate: baudRate,
	}
	port, err := serial.Open(portDetails.Name, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portDetails.Name, err)
	}

	client := &Client{
		port:         port,
		serialNumber: portDetails.SerialNumber,
	}

	// TODO: Add KryoFlux specific initialization when protocol is known
	// For now, we just open the port and store the connection

	return client, nil
}

// PrintStatus prints KryoFlux status information to stdout
func (c *Client) PrintStatus() {
	fmt.Printf("KryoFlux Adapter\n")
	fmt.Printf("Serial Number: %s\n", c.serialNumber)
	fmt.Printf("Baud Rate: %d\n", baudRate)
	fmt.Printf("Status: Connected\n")
	fmt.Printf("Note: Full protocol implementation pending\n")
}

// Close closes the serial port connection
func (c *Client) Close() error {
	if c.port != nil {
		return c.port.Close()
	}
	return nil
}

