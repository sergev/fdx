package hfe

import "fmt"

// ReadPRI reads a file in PRI format and returns a Disk structure.
func ReadPRI(filename string) (*Disk, error) {
	return nil, fmt.Errorf("PRI format not yet implemented")
}

// WritePRI writes a Disk structure to a PRI format file.
func WritePRI(filename string, disk *Disk) error {
	return fmt.Errorf("PRI format not yet implemented")
}
