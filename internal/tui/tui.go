package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"log"

	ui "github.com/gizak/termui/v3"
)

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
