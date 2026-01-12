package hfe

import "fmt"

// ReadIMD reads a file in IMD format and returns a Disk structure.
func ReadIMD(filename string) (*Disk, error) {
	return nil, fmt.Errorf("IMD format not yet implemented")
}

// WriteIMD writes a Disk structure to an IMD format file.
func WriteIMD(filename string, disk *Disk) error {
	return fmt.Errorf("IMD format not yet implemented")
}
