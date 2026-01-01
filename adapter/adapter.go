package adapter

import "go.bug.st/serial/enumerator"

// FloppyAdapter defines the interface for floppy disk adapters
type FloppyAdapter interface {
	// PrintStatus prints adapter status information to stdout
	PrintStatus()
}

// NewClientFunc is a function type that creates a new adapter client
type NewClientFunc func(portDetails *enumerator.PortDetails) (FloppyAdapter, error)

