package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"log"

	ui "github.com/gizak/termui/v3"
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

var screen = NewScreen()

func Start() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()
	mouseMoveEvents(true)
	defer mouseMoveEvents(false)

	screen.Init()

	menuList := NewMenuList()
	screen.SetMenu(menuList)
	screen.SetNamespace(kube.GetNamespace())
	namespaces := NewNamespacesTable()
	screen.Focus(menuList)
	screen.Focus(namespaces)
	screen.ReplaceRightPane(namespaces)
	screen.RenderAll()
	screen.Run()
}
