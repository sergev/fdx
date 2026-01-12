package hfe

import "fmt"

// ReadDCF reads a file in DCF format and returns a Disk structure.
func ReadDCF(filename string) (*Disk, error) {
	return nil, fmt.Errorf("DCF format not yet implemented")
}

// WriteDCF writes a Disk structure to a DCF format file.
func WriteDCF(filename string, disk *Disk) error {
	return fmt.Errorf("DCF format not yet implemented")
}
