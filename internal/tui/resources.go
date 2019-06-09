package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
)

func GetDefaultActions(handler widgets.ListTableHandler) []*widgets.ListAction {
	var actions []*widgets.ListAction
	if _, ok := handler.(widgets.DataTableResource); ok {
		actions = append(actions,
			&widgets.ListAction{
				Name:          "Describe",
				HotKey:        "d",
				HotKeyDisplay: "D",
				Func:          OnResourceDescribe,
			},
			&widgets.ListAction{
				Name:          "Edit",
				HotKey:        "e",
				HotKeyDisplay: "E",
				Func:          OnResourceEdit,
			},
		)
	}

	if _, ok := handler.(widgets.DataTableDeletable); ok {
		actions = append(actions,
			&widgets.ListAction{
				Name:          "Delete",
				HotKey:        "<Delete>",
				HotKeyDisplay: "Del",
				Func:          OnResourceDelete,
			},
		)
	}
	return actions
}

func OnResourceDescribe(handler widgets.ListTableHandler, idx int, row widgets.ListRow) bool {
	h, ok := handler.(widgets.DataTableResource)
	if !ok {
		return false
	}
	name := row[0]
	if n, ok := h.(widgets.DataTableResourceNamespace); ok {
		screen.SwitchToCommand(kube.Viewer(kube.DescribeNs(n.Namespace(), n.TypeName(), name)))
	} else {
		screen.SwitchToCommand(kube.Viewer(kube.Describe(h.TypeName(), name)))
	}
	return true
}

func OnResourceEdit(handler widgets.ListTableHandler, idx int, row widgets.ListRow) bool {
	h, ok := handler.(widgets.DataTableResource)
	if !ok {
		return false
	}
	name := row[0]
	if n, ok := h.(widgets.DataTableResourceNamespace); ok {
		screen.SwitchToCommand(kube.EditNs(n.Namespace(), n.TypeName(), name))
	} else {
		screen.SwitchToCommand(kube.Edit(h.TypeName(), name))
	}
	return true
}

func OnResourceDelete(handler widgets.ListTableHandler, idx int, row widgets.ListRow) bool {
	h, ok := handler.(widgets.DataTableDeletable)
	if !ok {
		return false
	}
	text := fmt.Sprintf("Are you sure you want to delete %s?", h.DeleteDescription(idx, row))
	ShowConfirmDialog(text, func() error {
		return h.Delete(idx, row)
	})
	return true
}
