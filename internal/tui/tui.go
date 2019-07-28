package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
)

var Application = &struct {
	Debug bool `mapstructure:"debug"`
}{}

func init() {
	cfg.AddPkg(&cfg.Pkg{
		Struct: Application,
		PersistentFlags: cfg.FlagsDeclaration{
			"debug": {false, "Enables debug to STDERR", "KUBEDEBUG"},
		},
	})
}

var screen *Screen
var app = NewApp()

func Start() error {
	return app.Run()
}
