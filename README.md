The fdx tool is a CLI program which works with floppy disks via USB adapter.

## Installation

The tool can be installed using the following command:

    go install github.com/sergev/fdx@latest

Note: The Golang compiler must be present on your system for this installation method to work.

## Supported Adapters

This tool supports three types of USB floppy drive adapters:

1. **Greaseweazle** - An open source USB device capable of reading and writing raw data on nearly any type of floppy disk.
   Official page: https://github.com/keirf/greaseweazle

2. **SuperCard Pro** - A flux level copier/imager/converter system for archiving floppy disks.
   Official page: https://www.cbmstuff.com/index.php?route=product/product&product_id=52

3. **KryoFlux** - A professional hardware solution for floppy disk preservation and imaging.
   Official page: https://webstore.kryoflux.com/catalog/product_info.php?cPath=1&products_id=30

The tool automatically detects and uses the first available adapter from the list above.

## Usage

    fdx status
    fdx read [FILE]
    fdx write FILE
    fdx format
    fdx erase

## Status

- Currently, only the [HFE](docs/HFE_File_Format.md) file format is supported.
- The `format` operation is not yet available. It will create filesystems of well-known types.
- For KryoFlux adapters, writing to floppies is not supported.
- The IMG file format is currently in development.
- Additional file formats are planned for future releases.

## License

This project is licensed under the MIT License.
