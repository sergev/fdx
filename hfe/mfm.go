package hfe

import "fmt"

// ReadMFM reads a file in MFM format and returns a Disk structure.
func ReadMFM(filename string) (*Disk, error) {
	return nil, fmt.Errorf("MFM format not yet implemented")
}

// WriteMFM writes a Disk structure to an MFM format file.
func WriteMFM(filename string, disk *Disk) error {
	return fmt.Errorf("MFM format not yet implemented")
}
