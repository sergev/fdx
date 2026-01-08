package adapter

import "go.bug.st/serial/enumerator"

// AdapterFactory is a function that creates an adapter from port details
type AdapterFactory func(portDetails *enumerator.PortDetails) (FloppyAdapter, error)

// AdapterInfo contains information about an adapter type
type AdapterInfo struct {
	VendorID  uint16
	ProductID uint16
	Factory   AdapterFactory
}

var registeredAdapters []AdapterInfo

// RegisterAdapter registers an adapter factory with its VID/PID
func RegisterAdapter(vendorID, productID uint16, factory AdapterFactory) {
	registeredAdapters = append(registeredAdapters, AdapterInfo{
		VendorID:  vendorID,
		ProductID: productID,
		Factory:   factory,
	})
}

// RegisterUSBAdapter registers an adapter that doesn't use serial ports
func RegisterUSBAdapter(factory AdapterFactory) {
	registeredAdapters = append(registeredAdapters, AdapterInfo{
		VendorID:  0, // Special marker for USB-only adapters
		ProductID: 0,
		Factory:   factory,
	})
}
