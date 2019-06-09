package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/gizak/termui/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (cmh *ContextMenuHandler) OnSelect(idx int, row widgets.ListRow) bool {
	name := row[0]
	for _, action := range cmh.actions {
		if action.Name == name {
			screen.popFocus()
			screen.removePopup()
			return action.Func(cmh.listHandler, idx, cmh.selectedRow)
		}
	}
	return true
}

func (s *Screen) ShowNamespaceSelection() {
	namespaces, err := kube.GetClient().CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		ShowErrorDialog(err, nil)
		return
	}
	var rows []widgets.ListRow
	var selectedRow int
	for i, namespace := range namespaces.Items {
		rows = append(rows, widgets.ListRow{namespace.Name})
		if namespace.Name == screen.selectedNamespace {
			selectedRow = i
		}
	}
	menu := widgets.NewListTable(rows, &NamespaceSelectorHandler{}, nil)
	menu.ScrollTo(selectedRow)
	menu.Title = "Select namespace"
	menu.IsContext = true
	width := 30
	height := len(rows) + 2
	y1 := screen.Rectangle.Max.Y/2 - height/2
	x1 := screen.Rectangle.Max.X/2 - width/2
	menu.SetRect(x1, y1, x1+width, y1+height)
	s.setPopup(menu)
	s.Focus(menu)
}

func (s *Screen) FocusToNamespace(namespace string) {
	screen.ResetFocus()
	screen.SetNamespace(namespace)
	screen.menu.ScrollTo(5)
}

type NamespaceSelectorHandler struct {
}

func (nsh *NamespaceSelectorHandler) OnSelect(idx int, row widgets.ListRow) bool {
	screen.popFocus()
	screen.removePopup()
	screen.FocusToNamespace(row[0])
	return true
}
