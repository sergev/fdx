package main

import (
	_ "github.com/sergev/floppy/greaseweazle"
	_ "github.com/sergev/floppy/kryoflux"
	_ "github.com/sergev/floppy/supercardpro"
	"github.com/sergev/floppy/adapter"
)

func main() {
	adapter.Execute()
}
