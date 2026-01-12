package hfe

import "fmt"

// ReadEPL reads a file in EPL format and returns a Disk structure.
func ReadEPL(filename string) (*Disk, error) {
	return nil, fmt.Errorf("EPL format not yet implemented")
}

// WriteEPL writes a Disk structure to an EPL format file.
func WriteEPL(filename string, disk *Disk) error {
	return fmt.Errorf("EPL format not yet implemented")
}
