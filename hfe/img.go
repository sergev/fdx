package hfe

import "fmt"

// ReadIMG reads a file in IMG or IMA format and returns a Disk structure.
func ReadIMG(filename string) (*Disk, error) {
	return nil, fmt.Errorf("IMG format not yet implemented")
}

// WriteIMG writes a Disk structure to an IMG or IMA format file.
func WriteIMG(filename string, disk *Disk) error {
	return fmt.Errorf("IMG format not yet implemented")
}
