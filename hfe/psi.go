package hfe

import "fmt"

// ReadPSI reads a file in PSI format and returns a Disk structure.
func ReadPSI(filename string) (*Disk, error) {
	return nil, fmt.Errorf("PSI format not yet implemented")
}

// WritePSI writes a Disk structure to a PSI format file.
func WritePSI(filename string, disk *Disk) error {
	return fmt.Errorf("PSI format not yet implemented")
}
