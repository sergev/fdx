package kryoflux

import (
	"fmt"

	"github.com/sergev/floppy/hfe"
)

// Write writes data from the HFE disk object to the floppy disk
func (c *Client) Write(disk *hfe.Disk, numberOfTracks int) error {
	return fmt.Errorf("Write is not supported for KryoFlux adapter")
}
