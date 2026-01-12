package hfe

import (
	"path/filepath"
	"strings"
)

// ImageFormat represents a floppy disk image format
type ImageFormat int

const (
	// ImageFormatUnknown represents an unknown or unrecognized format
	ImageFormatUnknown ImageFormat = iota
	ImageFormatCP2                 // CP2 format - Central Point Software's Copy-II-PC
	ImageFormatDCF                 // DCF format - Disk Copy Fast utility
	ImageFormatEPL                 // EPL format - EPLCopy utility
	ImageFormatHFE                 // HFE format - HxC Floppy Emulator
	ImageFormatIMD                 // IMD format - Dave Dunfield's ImageDisk utility
	ImageFormatIMG                 // IMG or IMA format - a raw, sector-by-sector binary copy of the entire disk
	ImageFormatMFM                 // MFM format - low-level MFM encoded bit stream
	ImageFormatPDI                 // PDI format - Upland's PlanetPress
	ImageFormatPRI                 // PRI format - PCE Raw Image
	ImageFormatPSI                 // PSI format - PCE Sector Image
	ImageFormatSCP                 // SCP format - SuperCard Pro low-level raw magnetic flux transitions
	ImageFormatTD0                 // TD0 format - Teledisk
)

// String returns the string representation of the ImageFormat
func (f ImageFormat) String() string {
	switch f {
	case ImageFormatCP2:
		return "CP2"
	case ImageFormatDCF:
		return "DCF"
	case ImageFormatEPL:
		return "EPL"
	case ImageFormatHFE:
		return "HFE"
	case ImageFormatIMD:
		return "IMD"
	case ImageFormatIMG:
		return "IMG"
	case ImageFormatMFM:
		return "MFM"
	case ImageFormatPDI:
		return "PDI"
	case ImageFormatPRI:
		return "PRI"
	case ImageFormatPSI:
		return "PSI"
	case ImageFormatSCP:
		return "SCP"
	case ImageFormatTD0:
		return "TD0"
	default:
		return "Unknown"
	}
}

// DetectImageFormat detects the image format from a filename based on its extension.
// The extension check is case-insensitive. Returns ImageFormatUnknown if the format
// cannot be determined.
func DetectImageFormat(filename string) ImageFormat {
	ext := filepath.Ext(filename)
	if ext == "" {
		return ImageFormatUnknown
	}

	// Remove leading dot and convert to lowercase for case-insensitive comparison
	ext = strings.ToLower(ext[1:])

	switch ext {
	case "cp2":
		return ImageFormatCP2
	case "dcf":
		return ImageFormatDCF
	case "epl":
		return ImageFormatEPL
	case "hfe":
		return ImageFormatHFE
	case "ima":
		return ImageFormatIMG // IMA is the same as IMG
	case "imd":
		return ImageFormatIMD
	case "img":
		return ImageFormatIMG
	case "mfm":
		return ImageFormatMFM
	case "pdi":
		return ImageFormatPDI
	case "pri":
		return ImageFormatPRI
	case "psi":
		return ImageFormatPSI
	case "scp":
		return ImageFormatSCP
	case "td0":
		return ImageFormatTD0
	default:
		return ImageFormatUnknown
	}
}
