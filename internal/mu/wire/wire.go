//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"github.com/icebase/mu/internal/mu/app"
)

func NewApp() *app.App {
	wire.Build(app.New, app.ConfigFromEnv)
	return &app.App{}
}
