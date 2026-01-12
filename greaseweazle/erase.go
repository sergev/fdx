package greaseweazle

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Erase erases all tracks on the floppy disk
// The erase operation writes a DC erase pattern for 200 seconds per track to ensure complete erasure
// This method iterates over all cylinders (82 tracks) and heads (2 sides), following the same pattern as Read()
func (c *Client) Erase(numberOfTracks int) error {
	// Select drive 0 and turn on motor
	err := c.SelectDrive(0)
	if err != nil {
		return fmt.Errorf("failed to select drive: %w", err)
	}
	err = c.SetMotor(0, true)
	if err != nil {
		return fmt.Errorf("failed to turn on motor: %w", err)
	}
	defer c.SetMotor(0, false) // Turn off motor when done

	// Calculate clock period in nanoseconds from sample frequency
	// clock_period_ns = 1e9 / sample_freq_hz
	clockPeriodNs := 1e9 / float64(c.firmwareInfo.SampleFreqHz)

	// Calculate erase duration in ticks: 200 seconds (200e6 nanoseconds) / clock_period
	// This matches the legacy implementation: 200e6 / _clock
	ticks := uint32(200e6 / clockPeriodNs)

	// Build CMD_ERASE_FLUX command: [CMD_ERASE_FLUX, 6, ticks (le32)]
	cmd := make([]byte, 6)
	cmd[0] = CMD_ERASE_FLUX
	cmd[1] = 6
	binary.LittleEndian.PutUint32(cmd[2:6], ticks)

	// Iterate through all cylinders and heads (same as Read())
	for cyl := 0; cyl < numberOfTracks; cyl++ {
		for head := 0; head < 2; head++ {
			// Print progress message
			if cyl != 0 || head != 0 {
				fmt.Printf("\rErasing track %d, side %d...", cyl, head)
			} else {
				fmt.Printf("Erasing track %d, side %d...", cyl, head)
			}

			// Seek to cylinder
			err = c.Seek(byte(cyl))
			if err != nil {
				return fmt.Errorf("failed to seek to cylinder %d: %w", cyl, err)
			}

			// Set head
			err = c.SetHead(byte(head))
			if err != nil {
				return fmt.Errorf("failed to set head %d: %w", head, err)
			}

			// Send the erase command
			err = c.doCommand(cmd)
			if err != nil {
				return fmt.Errorf("failed to send ERASE_FLUX command for cylinder %d, head %d: %w", cyl, head, err)
			}

			// Read synchronization byte (returned when erase operation completes)
			// Value 0 indicates success
			syncByte := make([]byte, 1)
			_, err = io.ReadFull(c.port, syncByte)
			if err != nil {
				return fmt.Errorf("failed to read erase synchronization byte for cylinder %d, head %d: %w", cyl, head, err)
			}

			if syncByte[0] != 0 {
				return fmt.Errorf("erase operation failed for cylinder %d, head %d with status byte: 0x%02x", cyl, head, syncByte[0])
			}

			// Check final flux status to verify operation completed successfully
			err = c.GetFluxStatus()
			if err != nil {
				return fmt.Errorf("erase operation status check failed for cylinder %d, head %d: %w", cyl, head, err)
			}
		}
	}
	fmt.Printf(" Done\n")

	return nil
}
