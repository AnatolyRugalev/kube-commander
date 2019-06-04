package widgets

import (
	"image"
	"sync"

	"github.com/AnatolyRugalev/kube-commander/internal/theme"
	"github.com/gizak/termui/v3"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type actionFunc func() error

type Action struct {
	Name      string
	HotKey    string
	OnExecute actionFunc
}

type ActionList struct {
	*widgets.Table
	Actions     []Action
	selectedRow int

	mux sync.Mutex

	rowStyle         ui.Style
	selectedRowStyle ui.Style
	dropDownVisible  bool
}

func NewActionList() *ActionList {
	actions := &ActionList{
		Table:            widgets.NewTable(),
		selectedRow:      -1,
		rowStyle:         theme.Theme["listItem"].Active,
		selectedRowStyle: theme.Theme["listItemSelected"].Active,
		dropDownVisible:  false,
	}
	actions.Table.RowSeparator = false
	actions.Table.FillRow = true
	actions.Table.TextAlignment = ui.AlignLeft
	actions.Table.Rows = [][]string{}
	return actions
}

func (a *ActionList) AddAction(name, hotKey string, onExec actionFunc) {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.Actions = append(a.Actions, Action{Name: name, HotKey: hotKey, OnExecute: onExec})
	a.Rows = append(a.Rows, []string{name, hotKey})
}

func (a *ActionList) setRect(x, y int) {
	y2 := y + len(a.Rows)
	// TODO: calc width
	x2 := x + 35
	a.Table.SetRect(x, y, x2, y2+2)
}

func (a *ActionList) Draw(buf *ui.Buffer) {
	if !a.dropDownVisible {
		return
	}
	for i := range a.Rows {
		if i == a.selectedRow {
			a.RowStyles[i] = a.selectedRowStyle
		} else {
			a.RowStyles[i] = a.rowStyle
		}
	}
	a.Table.Draw(buf)
}

func (a *ActionList) Show(x, y int) {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.dropDownVisible = true
	a.setRect(x, y)
}

func (a *ActionList) Hide() {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.dropDownVisible = false
}

func (a *ActionList) OnEvent(event *termui.Event) bool {
	switch event.ID {
	case "<MouseRelease>":
		m := event.Payload.(ui.Mouse)
		if a.locateAndFocus(m.X, m.Y) {
			return true
		}
		return false
	case "<MouseLeft>":
		a.Hide()
		return true
	}

	return false
}

func (a *ActionList) setSelected(rowIdx int) {
	a.mux.Lock()
	defer a.mux.Unlock()
	if rowIdx >= 0 && rowIdx < len(a.Rows) {
		a.selectedRow = rowIdx
	}
}

func (a *ActionList) locateAndFocus(x, y int) bool {
	rect := image.Rect(x, y, x+1, y+1)
	if rect.In(a.Table.Bounds()) {
		a.setSelected(y - a.Min.Y - 1)
		return true
	}

	return false
}
