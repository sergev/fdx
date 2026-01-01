The floppy tool is a CLI program which works with floppy disks via USB adapter.

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

    floppy status
    floppy read [FILE]
    floppy write FILE
    floppy format
    floppy erase

## License

This project is licensed under the MIT License.
