package hfe

import "fmt"

// ReadTD0 reads a file in TD0 format and returns a Disk structure.
func ReadTD0(filename string) (*Disk, error) {
	return nil, fmt.Errorf("TD0 format not yet implemented")
}

// WriteTD0 writes a Disk structure to a TD0 format file.
func WriteTD0(filename string, disk *Disk) error {
	return fmt.Errorf("TD0 format not yet implemented")
}
