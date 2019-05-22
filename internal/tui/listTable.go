package tui

import (
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// TODO: implement table scrolling

type listTableSelector func(row []string) bool

type ListTable struct {
	*widgets.Table
	RowStyle         ui.Style
	HeaderRowsCount  int
	SelectedRowStyle ui.Style
	SelectedRow      int
	OnSelect         listTableSelector
}

func NewListTable() *ListTable {
	return &ListTable{
		HeaderRowsCount: 1,
		Table:           widgets.NewTable(),
	}
}

func NewSelectableListTable(onSelect listTableSelector) *ListTable {
	return &ListTable{
		HeaderRowsCount: 1,
		Table:           widgets.NewTable(),
		OnSelect:        onSelect,
	}
}

func (lt *ListTable) Draw(buf *ui.Buffer) {
	for i := range lt.Table.Rows {
		if i == lt.SelectedRow+lt.HeaderRowsCount {
			lt.Table.RowStyles[i] = lt.SelectedRowStyle
		} else {
			lt.Table.RowStyles[i] = lt.RowStyle
		}
	}
	lt.Table.Draw(buf)
}

func (lt *ListTable) OnEvent(event *ui.Event) bool {
	switch event.ID {
	case "<Down>":
		if lt.SelectedRow >= len(lt.Rows)-lt.HeaderRowsCount-1 {
			return false
		}
		lt.CursorDown()
		return true
	case "<Up>":
		if lt.SelectedRow <= 0 {
			return false
		}
		lt.CursorUp()
		return true
	case "<Enter>":
		row := lt.Rows[lt.SelectedRow+1]
		if lt.OnSelect != nil {
			return lt.OnSelect(row)
		}
		return false
	}
	return false
}

func (lt *ListTable) CursorDown() {
	lt.SelectedRow += 1
}

func (lt *ListTable) CursorUp() {
	lt.SelectedRow -= 1
}

func (lt *ListTable) OnFocusIn() {
	lt.BorderStyle = ui.NewStyle(ui.ColorYellow)
}

func (lt *ListTable) OnFocusOut() {
	lt.BorderStyle = ui.NewStyle(ui.ColorWhite)
}
