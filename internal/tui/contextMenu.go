package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/gizak/termui/v3"
)

func (s *Screen) ShowActionsContextMenu(listHandler widgets.ListTableHandler, actions []*widgets.ListAction, selectedRow widgets.ListRow, mouse termui.Mouse) {
	var rows []widgets.ListRow
	for _, action := range actions {
		rows = append(rows, widgets.ListRow{action.Name, action.HotKeyDisplay})
	}
	handler := &ContextMenuHandler{
		selectedRow: selectedRow,
		listHandler: listHandler,
		actions:     actions,
	}
	menu := widgets.NewListTable(rows, handler, nil)
	menu.IsContext = true

	x, y := mouse.X, mouse.Y

	y2 := y + len(rows) + 2
	// TODO: calc width
	x2 := x + 30

	if y2 >= s.Inner.Max.Y {
		y = s.Inner.Max.Y - len(rows) - 2
		y2 = s.Inner.Max.Y
	}

	menu.SetRect(x, y, x2, y2)
	s.setPopup(menu)
	s.Focus(menu)
}

type ContextMenuHandler struct {
	selectedRow widgets.ListRow
	listHandler widgets.ListTableHandler
	actions     []*widgets.ListAction
}

func (cmh *ContextMenuHandler) OnSelect(row widgets.ListRow) bool {
	name := row[0]
	for _, action := range cmh.actions {
		if action.Name == name {
			screen.popFocus()
			screen.removePopup()
			return action.Func(cmh.listHandler, cmh.selectedRow)
		}
	}
	return true
}
