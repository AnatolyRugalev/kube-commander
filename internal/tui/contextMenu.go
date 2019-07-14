package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/gizak/termui/v3"
	"image"
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
	menu := NewListTableDialog("", rows, handler)
	point := &image.Point{X: mouse.X, Y: mouse.Y}
	s.ShowContextMenu(point, menu)
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
		s.ShowDialog(NewErrorDialog(err, nil))
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
	menu := NewListTableDialog("Select namespace", rows, &NamespaceSelectorHandler{})
	menu.ScrollTo(selectedRow)
	s.ShowDialog(menu)
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
