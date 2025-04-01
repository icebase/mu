package main

import (
	"log/slog"

	"github.com/icebase/mu/internal/mu/wire"
)

func main() {
	app := wire.NewApp()
	err := app.Init()
	if err != nil {
		slog.Error("init failed",
			"err", err)
		return
	}
	app.Run()
}
