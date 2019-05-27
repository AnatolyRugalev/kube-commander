package tui

import (
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"unicode/utf8"
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
	lt.BorderStyle = theme["grid"].inactive
	lt.TitleStyle = theme["title"].inactive
	lt.RowStyle = theme["listItem"].inactive
	lt.HeaderStyle = theme["listHeader"].inactive
	lt.SelectedRowStyle = theme["listItemSelected"].inactive
	lt.RowSeparator = false
	lt.FillRow = true
	lt.ColumnResizer = func() {
		if len(lt.Rows) == 0 {
			lt.ColumnWidths = []int{}
			return
		}
		colCount := len(lt.Rows[0])
		var widths []int
		for i := range lt.Rows[0] {
			var width = 1
			if i == colCount-1 {
				// Last column
				width = 999
			} else {
				for _, row := range lt.Rows {
					if utf8.RuneCountInString(row[i]) > width {
						width = len(row[i])
					}
				}
			}
			widths = append(widths, width+1)
		}
		lt.ColumnWidths = widths
	}
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
	case "<Delete>":
		nodeName := lt.Rows[lt.SelectedRow+1][0]
		res := ShowDialog("Delete", "Are you sure want to delete "+nodeName+"?", ButtonYes, ButtonNo)
		ShowDialog("Delete", "He-hey, you sad "+res)
		return true
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
	lt.BorderStyle = theme["grid"].active
	lt.TitleStyle = theme["title"].active
	lt.RowStyle = theme["listItem"].active
	lt.HeaderStyle = theme["listHeader"].active
	lt.SelectedRowStyle = theme["listItemSelected"].active
}

func (lt *ListTable) OnFocusOut() {
	lt.BorderStyle = theme["grid"].inactive
	lt.TitleStyle = theme["title"].inactive
	lt.RowStyle = theme["listItem"].inactive
	lt.HeaderStyle = theme["listHeader"].inactive
	lt.SelectedRowStyle = theme["listItemSelected"].inactive
}
