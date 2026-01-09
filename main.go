package main

import (
	_ "github.com/sergev/fdx/greaseweazle"
	_ "github.com/sergev/fdx/kryoflux"
	_ "github.com/sergev/fdx/supercardpro"
	"github.com/sergev/fdx/adapter"
)

func main() {
	adapter.Execute()
}
