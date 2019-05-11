package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/spf13/cobra"
	"log"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
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

	client, err := kube.GetClient()
	if err != nil {
		log.Fatalf("Kube error")
	}

	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)

	menuList := widgets.NewList()
	menuList.Title = "Cluster"
	menuList.Rows = []string{
		"Overview",
		"Namespaces",
		"Nodes",
		"Storage Classes",
		"Persistent Volumes",
	}
	menuList.SelectedRow = 0
	menuList.TextStyle = ui.NewStyle(ui.ColorYellow)
	menuList.WrapText = false

	pods, err := client.GetPods("")

	if err != nil {
		log.Fatalf("Pods error")
	}

	podTable := widgets.NewTable()
	podTable.Title = "Pods"
	podTable.RowSeparator = false

	for _, pod := range pods.Items {
		podTable.Rows = append(podTable.Rows, []string{
			pod.Name,
			pod.Namespace,
			string(pod.Status.Phase),
		})
	}
	menuList.SelectedRow = 0
	menuList.TextStyle = ui.NewStyle(ui.ColorYellow)
	menuList.WrapText = false

	grid.Set(
		ui.NewRow(1.0,
			ui.NewCol(0.1, menuList),
			ui.NewCol(0.9, podTable),
		),
	)

	ui.Render(grid)
	uiEvents := ui.PollEvents()
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "<Down>":
				menuList.SelectedRow += 1
				ui.Render(grid)
			case "<Up>":
				menuList.SelectedRow -= 1
				ui.Render(grid)
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Clear()
				ui.Render(grid)
			}
		}
	}
}
