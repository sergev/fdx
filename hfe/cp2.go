package hfe

import "fmt"

// ReadCP2 reads a file in CP2 format and returns a Disk structure.
func ReadCP2(filename string) (*Disk, error) {
	return nil, fmt.Errorf("CP2 format not yet implemented")
}

// WriteCP2 writes a Disk structure to a CP2 format file.
func WriteCP2(filename string, disk *Disk) error {
	return fmt.Errorf("CP2 format not yet implemented")
}
