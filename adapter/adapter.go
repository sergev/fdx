package adapter

import "go.bug.st/serial/enumerator"

// FloppyAdapter defines the interface for floppy disk adapters
type FloppyAdapter interface {
	// PrintStatus prints adapter status information to stdout
	PrintStatus()
	// Read reads the entire floppy disk and writes it to the specified filename
	Read(filename string) error
}

// NewClientFunc is a function type that creates a new adapter client
type NewClientFunc func(portDetails *enumerator.PortDetails) (FloppyAdapter, error)

