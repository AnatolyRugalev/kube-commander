package tui

import (
	"log"

	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	ui "github.com/gizak/termui/v3"
	"github.com/spf13/cobra"
)

func init() {
	cfg.AddCommand(&cobra.Command{
		Use: "tui",
		Run: func(cmd *cobra.Command, args []string) {
			Start()
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
	namespaces := NewNamespacesTable()
	screen.Focus(menuList)
	screen.Focus(namespaces)
	screen.ReplaceRightPane(namespaces)
	screen.RenderAll()
	screen.Run()
}
