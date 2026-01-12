package adapter

import (
	"go.bug.st/serial/enumerator"

	"github.com/sergev/floppy/hfe"
)

// FloppyAdapter defines the interface for floppy disk adapters
type FloppyAdapter interface {
	// PrintStatus prints adapter status information to stdout
	PrintStatus()

	// Read reads the entire floppy disk and returns it as an HFE disk object
	Read(numberOfTracks int) (*hfe.Disk, error)

	// Write writes data from the HFE disk object to the floppy disk
	Write(disk *hfe.Disk, numberOfTracks int) error

	// Format formats the floppy disk
	Format() error

	// Erase erases the floppy disk
	Erase(numberOfTracks int) error
}

// NewClientFunc is a function type that creates a new adapter client
type NewClientFunc func(portDetails *enumerator.PortDetails) (FloppyAdapter, error)
