package main

import "github.com/icebase/mu/internal/mu/wire"

func main() {
	app := wire.NewApp()
	app.Run()
}
