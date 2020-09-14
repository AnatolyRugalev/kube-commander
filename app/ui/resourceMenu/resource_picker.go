package resourceMenu

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
)

type GroupKindFunc func(res *commander.Resource)

func pickResource(workspace commander.Workspace, f GroupKindFunc) {
	picker, err := newResourcePicker(workspace, func(res *commander.Resource) {
		workspace.FocusManager().Blur()
		f(res)
	})
	if err != nil {
		workspace.Status().Error(err)
		return
	}
	workspace.ShowPopup("Select resource", picker)
}

func newResourcePicker(container commander.ResourceContainer, f GroupKindFunc) (*listTable.ListTable, error) {
	columns := []string{
		"Group",
		"Kind",
	}
	var rows []commander.Row
	resources, err := container.ResourceProvider().Resources()
	if err != nil {
		return nil, err
	}
	resMap := make(map[string]*commander.Resource)
	for _, res := range resources {
		rows = append(rows, commander.NewSimpleRow(res.Gk.String(), []string{
			res.Gk.Group,
			res.Gk.Kind,
		}, true))
		resMap[res.Gk.String()] = res
	}
	lt := listTable.NewStaticListTable(columns, rows, listTable.NoHorizontalScroll|listTable.WithFilter|listTable.WithHeaders|listTable.AlwaysFilter, container.ScreenHandler())
	lt.BindOnKeyPress(func(row commander.Row, event *tcell.EventKey) bool {
		if row == nil {
			return false
		}
		if event.Key() == tcell.KeyEnter {
			go func() {
				f(resMap[row.Id()])
			}()
			return true
		}
		return false
	})
	return lt, nil
}
