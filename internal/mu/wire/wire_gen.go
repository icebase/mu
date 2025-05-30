// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package wire

import (
	"github.com/icebase/mu/internal/mu/app"
)

// Injectors from wire.go:

func NewApp() *app.App {
	config := app.ConfigFromEnv()
	appApp := app.New(config)
	return appApp
}
