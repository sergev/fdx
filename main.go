package main

import (
	_ "floppy/greaseweazle"
	_ "floppy/kryoflux"
	_ "floppy/supercardpro"
	"floppy/adapter"
)

func main() {
	adapter.Execute()
}
