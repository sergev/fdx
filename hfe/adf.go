package hfe

import "fmt"

// ReadADF reads a file in ADF format and returns a Disk structure.
func ReadADF(filename string) (*Disk, error) {
	return nil, fmt.Errorf("ADF format not yet implemented")
}

// WriteADF writes a Disk structure to a ADF format file.
func WriteADF(filename string, disk *Disk) error {
	return fmt.Errorf("ADF format not yet implemented")
}
