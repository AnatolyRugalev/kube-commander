package app

import (
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/events"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/gdamore/tcell"
)

type nsMenuHandler struct {
	app *App
}

func (n nsMenuHandler) HandleRowEvent(event listTable.RowEvent) bool {
	switch ev := event.(type) {
	case *listTable.RowTcellEvent:
		switch ev := ev.TcellEvent().(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEnter:
				row := event.Row()
				if row == nil {
					return false
				}
				selected := row[0].(string)
				if selected == client.AllNamespaces {
					selected = ""
				}
				n.app.selectedNamespace = selected
				n.app.screen.Blur()
				n.app.screen.PostEvent(events.NewNamespaceChanged(event.ListTable(), n.app.selectedNamespace))
				return true
			}
		}
	}
	return false
}

func NewNamespaceSelector(app *App, client client.Client, namespaceResource *client.Resource) (*listTable.ListTable, error) {
	table, err := client.LoadResourceToTable(namespaceResource, "")
	if err != nil {
		return nil, err
	}
	items := []string{
		"All namespaces",
	}
	for _, row := range table.Rows {
		items = append(items, row.Cells[0].(string))
	}

	lt := listTable.NewList(items)
	lt.RegisterRowEventHandler(&nsMenuHandler{app: app})
	return lt, nil
}
