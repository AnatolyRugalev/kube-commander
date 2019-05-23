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
	HeaderStyle      ui.Style
	SelectedRowStyle ui.Style
	SelectedRow      int
	OnSelect         listTableSelector
}

func NewListTable() *ListTable {
	lt := &ListTable{
		HeaderRowsCount: 1,
		Table:           widgets.NewTable(),
	}
	lt.BorderStyle = theme["default"]
	lt.TitleStyle = theme["title"]
	lt.RowStyle = theme["default"]
	lt.HeaderStyle = theme["header"]
	lt.SelectedRowStyle = theme["selectedOutOfFocus"]
	lt.RowSeparator = false
	lt.FillRow = true
	return lt
}

func NewSelectableListTable(onSelect listTableSelector) *ListTable {
	lt := NewListTable()
	lt.OnSelect = onSelect
	return lt
}

func (lt *ListTable) Draw(buf *ui.Buffer) {
	for i := range lt.Table.Rows {
		if i == lt.SelectedRow+lt.HeaderRowsCount {
			lt.Table.RowStyles[i] = lt.SelectedRowStyle
		} else {
			lt.Table.RowStyles[i] = lt.RowStyle
		}
	}
	lt.Table.RowStyles[0] = lt.HeaderStyle
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
	lt.BorderStyle = theme["focus"]
	lt.SelectedRowStyle = theme["selectedInFocus"]
}

func (lt *ListTable) OnFocusOut() {
	lt.BorderStyle = theme["default"]
	lt.SelectedRowStyle = theme["selectedOutOfFocus"]
}
