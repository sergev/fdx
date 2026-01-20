The floppy tool is a CLI program which works with floppy disks via USB adapter.

## Supported Adapters

This tool supports three types of USB floppy drive adapters:

1. [Greaseweazle](https://github.com/keirf/greaseweazle) - An open source USB device capable of reading and writing raw data on nearly any type of floppy disk.

2. [SuperCard Pro](https://www.cbmstuff.com/index.php?route=product/product&product_id=52) - A flux level copier/imager/converter system for archiving floppy disks.

3. [KryoFlux](https://webstore.kryoflux.com/catalog/product_info.php?cPath=1&products_id=30) - A professional hardware solution for floppy disk preservation and imaging.

The tool automatically detects and uses the first available adapter from the list above.

## Installation

The tool can be installed using the following command:

    go install github.com/sergev/floppy@latest

Note: The Golang compiler must be present on your system for this installation method to work.

## Usage

    floppy status
    floppy read [DEST.EXT]
    floppy write SRC.EXT
    floppy format
    floppy erase
    floppy convert SRC.EXT DEST.EXT

## Status

- Currently, supported file formats are [HFE](docs/HFE_File_Format.md),
  [IMG](https://en.wikipedia.org/wiki/IMG_(file_format)),
  [IMD](http://dunfield.classiccmp.org/img42841/readme.txt),
  [ADF](https://en.wikipedia.org/wiki/Amiga_Disk_File) and
  [BKD](https://en.wikipedia.org/wiki/ANDOS).
- Other file formats are planned for future releases.
- For KryoFlux adapters, writing to floppies is not supported.

## License

This project is licensed under the MIT License.
