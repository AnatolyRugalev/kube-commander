package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
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

func Start() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	screen := NewScreen()
	screen.Init()

	podTable := NewPodsTable("kube-system")
	nsTable := NewNamespacesTable()

	menuList := NewMenuList(map[string]Focusable{
		"Namespaces": nsTable,
		"Pods":       podTable,
	})
	menuList.OnCursorChange(func(item Focusable) {
		if loadable, ok := item.(Loadable); ok {
			// TODO: preloader component
			// TODO: asynchronous loading
			preloader := widgets.NewParagraph()
			preloader.Text = "Loading..."
			screen.SetPanes(menuList, preloader)
			ui.Render(screen)
			loaded := make(chan error)
			loadable.Reload(loaded)
			go func() {
				err := <-loaded
				if err != nil {
					preloader.Text = err.Error()
					screen.SetPanes(menuList, preloader)
					ui.Render(screen)
				} else {
					screen.SetPanes(menuList, item)
				}
			}()
		} else {
			screen.SetPanes(menuList, item)
		}
	})
	menuList.OnActivate(func(focusable Focusable) {
		screen.Focus(focusable)
	})
	screen.SetPanes(menuList, nil)
	screen.Focus(menuList)
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
