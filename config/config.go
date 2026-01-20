package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

//go:embed floppy.toml
var defaultConfigData []byte

// Global state variables for the selected drive
var (
	DriveName string
	Cyls      int
	Heads     int
	RPM       int
	MaxKBps   int
	Images    []string
	ImageMap  map[string]string // image name -> filename mapping
)

// Config represents the entire TOML configuration structure
type Config struct {
	Default string  `toml:"default"`
	Drive   []Drive `toml:"drive"`
	Image   []Image `toml:"image"`
}

// Drive represents a floppy drive configuration
type Drive struct {
	Name    string   `toml:"name"`
	Cyls    int      `toml:"cyls"`
	Heads   int      `toml:"heads"`
	RPM     int      `toml:"rpm"`
	MaxKBps int      `toml:"maxkbps"`
	Images  []string `toml:"images"`
}

// Image represents a built-in image configuration
type Image struct {
	Name string `toml:"name"`
	File string `toml:"file"`
}

// configPath determines the config file path based on the operating system
func configPath() (string, error) {
	var configDir string
	var err error

	switch runtime.GOOS {
	case "windows":
		// Use AppData directory for Windows
		configDir, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine user config directory: %w", err)
		}
		// Create floppy subdirectory path
		configDir = filepath.Join(configDir, "floppy")
	default:
		// Linux/macOS: use home directory
		configDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine user home directory: %w", err)
		}
	}

	return filepath.Join(configDir, ".floppy"), nil
}

// Initialize loads and validates the configuration file.
// If the config file doesn't exist, it creates it from the embedded default.
func Initialize() error {
	// 1. Determine config file path
	configPath, err := configPath()
	if err != nil {
		return err
	}

	// 2. Check if config file exists, create from embedded default if not
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create parent directory if needed (for Windows)
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
		}

		// Write embedded default config to file
		if err := os.WriteFile(configPath, defaultConfigData, 0644); err != nil {
			return fmt.Errorf("failed to create default config file at %s: %w", configPath, err)
		}
	}

	// 4. Parse TOML file
	var conf Config
	if _, err := toml.DecodeFile(configPath, &conf); err != nil {
		return fmt.Errorf("failed to parse TOML config at %s: %w", configPath, err)
	}

	// 5. Find and validate `default` key
	if conf.Default == "" {
		return errors.New("`default` key is missing or empty in config")
	}

	// 6. Search drive array for matching name
	var foundDrive *Drive
	for i := range conf.Drive {
		if conf.Drive[i].Name == conf.Default {
			foundDrive = &conf.Drive[i]
			break
		}
	}

	if foundDrive == nil {
		return fmt.Errorf("default drive %q not found in drive array", conf.Default)
	}

	// 7. Validate drive fields (positive integers, non-empty images list)
	if foundDrive.Cyls <= 0 {
		return fmt.Errorf("drive %q has invalid cyls: %d (must be positive)", conf.Default, foundDrive.Cyls)
	}
	if foundDrive.Heads <= 0 {
		return fmt.Errorf("drive %q has invalid heads: %d (must be positive)", conf.Default, foundDrive.Heads)
	}
	if foundDrive.RPM <= 0 {
		return fmt.Errorf("drive %q has invalid rpm: %d (must be positive)", conf.Default, foundDrive.RPM)
	}
	if foundDrive.MaxKBps <= 0 {
		return fmt.Errorf("drive %q has invalid maxkbps: %d (must be positive)", conf.Default, foundDrive.MaxKBps)
	}
	if len(foundDrive.Images) == 0 {
		return fmt.Errorf("drive %q has no images listed", conf.Default)
	}

	// 8. Store drive properties in global variables
        DriveName = conf.Default
	Cyls = foundDrive.Cyls
	Heads = foundDrive.Heads
	RPM = foundDrive.RPM
	MaxKBps = foundDrive.MaxKBps
	Images = make([]string, len(foundDrive.Images))
	copy(Images, foundDrive.Images)

	// 9. Verify each item in images array exists in image array
	// and build ImageMap for looking up filenames by image name
	imageMap := make(map[string]bool)
	ImageMap = make(map[string]string)
	for _, img := range conf.Image {
		imageMap[img.Name] = true
		ImageMap[img.Name] = img.File
	}

	for _, imgName := range foundDrive.Images {
		if !imageMap[imgName] {
			return fmt.Errorf("image %q listed under drive %q not found in image array", imgName, conf.Default)
		}
	}

	return nil
}

// GetImageFilename returns the filename for a given image name.
// Returns an error if the image name is not found in the configuration.
func GetImageFilename(imageName string) (string, error) {
	filename, ok := ImageMap[imageName]
	if !ok {
		return "", fmt.Errorf("image %q not found in configuration", imageName)
	}
	return filename, nil
}
