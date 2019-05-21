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

	menuList := NewMenuList(screen)
	screen.SetMenu(menuList)
	menuList.OnCursorChange(func(item Pane) {
		if loadable, ok := item.(Loadable); ok {
			preloader := NewPreloader()
			screen.SetRightPane(preloader)
			ui.Render(screen)
			go func() {
				err := loadable.Reload()
				if err != nil {
					preloader.Text = err.Error()
					screen.SetRightPane(preloader)
				} else {
					screen.SetRightPane(item)
				}
				ui.Render(screen)
			}()
		} else {
			screen.SetRightPane(item)
		}
	})
	menuList.OnActivate(func(focusable Pane) {
		screen.Focus(focusable)
	})
	screen.Focus(menuList)
	screen.SetRightPane(NewWelcomeScreen())
	ui.Render(screen)

	uiEvents := ui.PollEvents()
	for {
		select {
		case e := <-uiEvents:
			redraw, exit := screen.OnEvent(&e)
			if exit {
				return
			}
			if redraw {
				ui.Render(screen)
			}
		}
	}
}
