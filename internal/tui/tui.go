package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	ui "github.com/gizak/termui/v3"
	"github.com/spf13/cobra"
	"log"
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

	screen.Init()

	menuList := NewMenuList()
	screen.SetMenu(menuList)
	screen.Focus(menuList)
	screen.ReplaceRightPane(NewWelcomeScreen())
	screen.Render()

	uiEvents := ui.PollEvents()
	for {
		select {
		case e := <-uiEvents:
			redraw, exit := screen.OnEvent(&e)
			if exit {
				return
			}
			if redraw {
				screen.Render()
			}
		}
	}
}
