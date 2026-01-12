package hfe

import "fmt"

// ReadPDI reads a file in PDI format and returns a Disk structure.
func ReadPDI(filename string) (*Disk, error) {
	return nil, fmt.Errorf("PDI format not yet implemented")
}

// WritePDI writes a Disk structure to a PDI format file.
func WritePDI(filename string, disk *Disk) error {
	return fmt.Errorf("PDI format not yet implemented")
}
