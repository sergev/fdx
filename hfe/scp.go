package hfe

import "fmt"

// ReadSCP reads a file in SCP format and returns a Disk structure.
func ReadSCP(filename string) (*Disk, error) {
	return nil, fmt.Errorf("SCP format not yet implemented")
}

// WriteSCP writes a Disk structure to an SCP format file.
func WriteSCP(filename string, disk *Disk) error {
	return fmt.Errorf("SCP format not yet implemented")
}
