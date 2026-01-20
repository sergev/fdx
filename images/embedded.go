package images

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
)

//go:embed andos.bkd.gz
var andosBkdGz []byte

//go:embed blank.adf.gz
var blankAdfGz []byte

//go:embed bsd1.44.img.gz
var bsd1_44ImgGz []byte

//go:embed fat1.2.img.gz
var fat1_2ImgGz []byte

//go:embed fat1.44.img.gz
var fat1_44ImgGz []byte

//go:embed fat1.6.img.gz
var fat1_6ImgGz []byte

//go:embed fat160.img.gz
var fat160ImgGz []byte

//go:embed fat180.img.gz
var fat180ImgGz []byte

//go:embed fat320.img.gz
var fat320ImgGz []byte

//go:embed fat360.img.gz
var fat360ImgGz []byte

//go:embed fat400.img.gz
var fat400ImgGz []byte

//go:embed fat720.img.gz
var fat720ImgGz []byte

//go:embed fat800.img.gz
var fat800ImgGz []byte

//go:embed linux1.44.img.gz
var linux1_44ImgGz []byte

var imageMap = map[string][]byte{
	"andos.bkd.gz":     andosBkdGz,
	"blank.adf.gz":     blankAdfGz,
	"bsd1.44.img.gz":   bsd1_44ImgGz,
	"fat1.2.img.gz":    fat1_2ImgGz,
	"fat1.44.img.gz":   fat1_44ImgGz,
	"fat1.6.img.gz":    fat1_6ImgGz,
	"fat160.img.gz":    fat160ImgGz,
	"fat180.img.gz":    fat180ImgGz,
	"fat320.img.gz":    fat320ImgGz,
	"fat360.img.gz":    fat360ImgGz,
	"fat400.img.gz":    fat400ImgGz,
	"fat720.img.gz":    fat720ImgGz,
	"fat800.img.gz":    fat800ImgGz,
	"linux1.44.img.gz": linux1_44ImgGz,
}

// GetImage retrieves and decompresses an embedded image file.
// The filename parameter should be the base filename as referenced in config
// (e.g., "fat160.img"), and this function will automatically append ".gz"
// to look up the embedded compressed file.
func GetImage(filename string) ([]byte, error) {
	// Append .gz extension to filename for lookup
	gzFilename := filename + ".gz"

	compressedData, ok := imageMap[gzFilename]
	if !ok {
		return nil, fmt.Errorf("embedded image not found: %s (looked for %s)", filename, gzFilename)
	}

	// Decompress using gzip
	gzReader, err := gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader for %s: %w", filename, err)
	}
	defer gzReader.Close()

	// Read all decompressed data
	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress %s: %w", filename, err)
	}

	return decompressed, nil
}
