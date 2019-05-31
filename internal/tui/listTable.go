package tui

import (
	"unicode/utf8"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// TODO: implement table scrolling

type ListTable struct {
	*widgets.Table
	extension        ListExtension
	RowStyle         ui.Style
	HeaderRowsCount  int
	HeaderStyle      ui.Style
	SelectedRowStyle ui.Style
	SelectedRow      int
}

func NewListTable(extension ListExtension) *ListTable {
	lt := &ListTable{
		HeaderRowsCount: 1,
		Table:           widgets.NewTable(),
		extension:       extension,
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
	lt.resetRows()
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
	case "<Enter>", eventMouseLeftDouble:
		if s, ok := lt.extension.(ListExtensionSelectable); ok {
			row := lt.Rows[lt.SelectedRow+1]
			return s.OnSelect(row)
		}
		return false
	case "<Delete>":
		if d, ok := lt.extension.(ListExtensionDeletable); ok {
			row := lt.Rows[lt.SelectedRow+1]
			ShowConfirmDialog(d.DeleteDialogText(row), func() error {
				return d.OnDelete(row)
			})
			return true
		}
		return false
	case "<MouseLeft>":
		m := event.Payload.(ui.Mouse)
		lt.setCursor(m.Y - lt.HeaderRowsCount - 1)
		return true
	}

	if e, ok := lt.extension.(ListExtensionEventable); ok {
		row := lt.Rows[lt.SelectedRow+1]
		return e.OnEvent(event, row)
	}
	return false
}

func (lt *ListTable) CursorDown() {
	lt.SelectedRow += 1
}

func (lt *ListTable) CursorUp() {
	lt.SelectedRow -= 1
}

func (lt *ListTable) setCursor(idx int) {
	if idx >= 0 && idx <= len(lt.Rows) {
		lt.SelectedRow = idx
	}
}

func (lt *ListTable) resetRows() {
	lt.Rows = [][]string{
		lt.extension.getTitleRow(),
	}
}

func (lt *ListTable) Reload() error {
	lt.resetRows()
	data, err := lt.extension.loadData()
	if err != nil {
		return err
	}
	for _, row := range data {
		lt.Rows = append(lt.Rows, row)
	}
	// If deleting last row
	if lt.SelectedRow >= len(lt.Rows)-1 {
		lt.SelectedRow = len(lt.Rows) - 2
	}
	return nil
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
