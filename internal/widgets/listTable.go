package widgets

import (
	"image"
	"unicode/utf8"

	"github.com/AnatolyRugalev/kube-commander/internal/theme"
	ui "github.com/gizak/termui/v3"
)

type ListRow []string

type ListTable struct {
	*ui.Block
	rows         []ListRow
	header       ListRow
	columnWidths []int
	topRow       int
	selectedRow  int

	// ColumnResizer is called on each Draw. Can be used for custom column sizing.
	ColumnResizer func()

	eventHandler  ListTableHandler
	screenHandler ScreenHandler

	RowStyles        map[int]ui.Style
	RowStyle         ui.Style
	SelectedRowStyle ui.Style
	HeaderStyle      ui.Style
	RowSeparator     bool
	TextAlignment    ui.Alignment
	FillRow          bool
	DrawVerticalLine bool

	IsContext bool
}

type ActionFunc func(handler ListTableHandler, row ListRow) bool

type ListAction struct {
	Name          string
	HotKey        string
	HotKeyDisplay string
	Func          ActionFunc
}

type ListTableHandler interface {
}

type ScreenHandler interface {
	ShowActionsContextMenu(listHandler ListTableHandler, actions []*ListAction, selectedRow ListRow, mouse ui.Mouse)
}

type ListTableHandlerWithHeader interface {
	ListTableHandler
	GetHeaderRow() ListRow
}

type ListTableHandlerWithActions interface {
	ListTableHandler
	GetActions() []*ListAction
}

type ListTableEventable interface {
	ListTableHandler
	OnEvent(event *ui.Event, row ListRow) bool
}

type ListTableCursorChangable interface {
	ListTableHandler
	OnCursorChange(row ListRow) bool
}

type ListTableSelectable interface {
	ListTableHandler
	OnSelect(row ListRow) bool
}

func NewListTable(rows []ListRow, handler ListTableHandler, screenHandler ScreenHandler) *ListTable {
	lt := &ListTable{
		Block: ui.NewBlock(),

		rows:          rows,
		eventHandler:  handler,
		screenHandler: screenHandler,

		RowSeparator:     false,
		RowStyles:        make(map[int]ui.Style),
		ColumnResizer:    func() {},
		DrawVerticalLine: true,
		FillRow:          true,
	}
	if h, ok := handler.(ListTableHandlerWithHeader); ok {
		lt.header = h.GetHeaderRow()
	}
	lt.ColumnResizer = func() {
		rows := lt.rows
		if len(lt.header) > 0 {
			rows = append(rows, lt.header)
		}
		if len(rows) == 0 {
			lt.columnWidths = []int{}
			return
		}
		colCount := len(rows[0])
		var widths []int
		for i := range rows[0] {
			var width = 1
			if i == colCount-1 {
				// Last column
				width = 999
			} else {
				for _, row := range rows {
					if utf8.RuneCountInString(row[i]) > width {
						width = len(row[i])
					}
				}
			}
			widths = append(widths, width+1)
		}
		lt.columnWidths = widths
	}
	// Apply initial styles
	lt.OnFocusOut()

	return lt
}

func (lt *ListTable) Draw(buf *ui.Buffer) {
	for i := range lt.rows {
		if i == lt.selectedRow {
			lt.RowStyles[i] = lt.SelectedRowStyle
		} else {
			lt.RowStyles[i] = lt.RowStyle
		}
	}

	lt.Block.Draw(buf)

	lt.ColumnResizer()

	columnWidths := lt.columnWidths
	if len(columnWidths) == 0 {
		var columnCount int
		if len(lt.header) > 0 {
			columnCount = len(lt.header)
		} else {
			columnCount = 1
		}
		columnWidth := lt.Inner.Dx() / columnCount
		for i := 0; i < columnCount; i++ {
			columnWidths = append(columnWidths, columnWidth)
		}
	}

	// adjusts view into widget
	if lt.selectedRow >= lt.Inner.Dy()+lt.topRow {
		viewport := lt.Inner.Dy() - 2
		lt.topRow = lt.selectedRow - viewport
	} else if lt.selectedRow < lt.topRow {
		lt.topRow = lt.selectedRow
	}

	// draw header if needed
	var yCoordinate int
	if len(lt.header) > 0 {
		yCoordinate = lt.drawRow(buf, columnWidths, lt.header, lt.HeaderStyle, lt.Inner.Min.Y)
	} else {
		yCoordinate = lt.Inner.Min.Y
	}

	// draw rows
	for i := lt.topRow; i < len(lt.rows) && yCoordinate < lt.Inner.Max.Y; i++ {
		rowStyle := lt.RowStyle
		if style, ok := lt.RowStyles[i]; ok {
			rowStyle = style
		}
		yCoordinate = lt.drawRow(buf, columnWidths, lt.rows[i], rowStyle, yCoordinate)
	}

	// draw UP_ARROW if needed
	if lt.topRow > 0 {
		yOffset := 0
		if lt.header != nil {
			yOffset = 1
		}
		buf.SetCell(
			ui.NewCell(ui.UP_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(lt.Inner.Max.X-1, lt.Inner.Min.Y+yOffset),
		)
	}

	// draw DOWN_ARROW if needed
	if len(lt.rows) > int(lt.topRow)+lt.Inner.Dy() {
		buf.SetCell(
			ui.NewCell(ui.DOWN_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(lt.Inner.Max.X-1, lt.Inner.Max.Y-1),
		)
	}
}

func (lt *ListTable) drawRow(buf *ui.Buffer, columnWidths []int, row []string, rowStyle ui.Style, yCoordinate int) int {
	if lt.FillRow {
		blankCell := ui.NewCell(' ', rowStyle)
		buf.Fill(blankCell, image.Rect(lt.Inner.Min.X, yCoordinate, lt.Inner.Max.X, yCoordinate+1))
	}

	colXCoordinate := lt.Inner.Min.X
	// draw row cells
	for j := 0; j < len(row); j++ {
		col := ui.ParseStyles(row[j], rowStyle)
		// draw row cell
		if len(col) > columnWidths[j] || lt.TextAlignment == ui.AlignLeft {
			for _, cx := range ui.BuildCellWithXArray(col) {
				k, cell := cx.X, cx.Cell
				if k == columnWidths[j] || colXCoordinate+k == lt.Inner.Max.X {
					cell.Rune = ui.ELLIPSES
					buf.SetCell(cell, image.Pt(colXCoordinate+k-1, yCoordinate))
					break
				} else {
					buf.SetCell(cell, image.Pt(colXCoordinate+k, yCoordinate))
				}
			}
		} else if lt.TextAlignment == ui.AlignCenter {
			xCoordinateOffset := (columnWidths[j] - len(col)) / 2
			stringXCoordinate := xCoordinateOffset + colXCoordinate
			for _, cx := range ui.BuildCellWithXArray(col) {
				k, cell := cx.X, cx.Cell
				buf.SetCell(cell, image.Pt(stringXCoordinate+k, yCoordinate))
			}
		} else if lt.TextAlignment == ui.AlignRight {
			stringXCoordinate := ui.MinInt(colXCoordinate+columnWidths[j], lt.Inner.Max.X) - len(col)
			for _, cx := range ui.BuildCellWithXArray(col) {
				k, cell := cx.X, cx.Cell
				buf.SetCell(cell, image.Pt(stringXCoordinate+k, yCoordinate))
			}
		}
		colXCoordinate += columnWidths[j] + 1
	}

	separatorStyle := lt.Block.BorderStyle

	// draw vertical separators
	if lt.DrawVerticalLine {
		separatorXCoordinate := lt.Inner.Min.X
		verticalCell := ui.NewCell(ui.VERTICAL_LINE, separatorStyle)
		for i, width := range columnWidths {
			if lt.FillRow && i < len(columnWidths)-1 {
				verticalCell.Style.Bg = rowStyle.Bg
			} else {
				verticalCell.Style.Bg = lt.Block.BorderStyle.Bg
			}

			separatorXCoordinate += width
			buf.SetCell(verticalCell, image.Pt(separatorXCoordinate, yCoordinate))
			separatorXCoordinate++
		}
	}

	yCoordinate++

	// draw horizontal separator
	horizontalCell := ui.NewCell(ui.HORIZONTAL_LINE, separatorStyle)
	if lt.RowSeparator && yCoordinate < lt.Inner.Max.Y {
		buf.Fill(horizontalCell, image.Rect(lt.Inner.Min.X, yCoordinate, lt.Inner.Max.X, yCoordinate+1))
		yCoordinate++
	}
	return yCoordinate
}

func (lt *ListTable) OnEvent(event *ui.Event) bool {
	if len(lt.rows) == 0 {
		return false
	}

	mouseSelectEvent := "<MouseLeftDouble>"
	mouseChangeEvent := "<MouseLeft>"
	if lt.IsContext {
		mouseSelectEvent = "<MouseLeft>"
		mouseChangeEvent = "<MouseRelease>"
	}

	switch event.ID {
	case "<Down>", "<MouseWheelDown>":
		lt.Down()
		return true
	case "<Up>", "<MouseWheelUp>":
		lt.Up()
		return true
	case "<PageDown>":
		lt.PageDown()
		return true
	case "<PageUp>":
		lt.PageUp()
		return true
	case "<Enter>", mouseSelectEvent:
		if s, ok := lt.eventHandler.(ListTableSelectable); ok {
			return s.OnSelect(lt.SelectedRow())
		}
		return false
	case mouseChangeEvent:
		m := event.Payload.(ui.Mouse)
		return lt.setCursorViaMouse(m)
	case "<MouseRight>":
		if h, ok := lt.eventHandler.(ListTableHandlerWithActions); ok && lt.screenHandler != nil {
			m := event.Payload.(ui.Mouse)
			lt.setCursorViaMouse(m)
			lt.screenHandler.ShowActionsContextMenu(lt.eventHandler, h.GetActions(), lt.SelectedRow(), m)
			return true
		}
		return false
	}

	if a, ok := lt.eventHandler.(ListTableHandlerWithActions); ok {
		for _, action := range a.GetActions() {
			if event.ID == action.HotKey {
				action.Func(lt.eventHandler, lt.SelectedRow())
				return true
			}
		}
	}

	return false
}

func (lt *ListTable) Scroll(amount int) {
	sel := lt.selectedRow + amount
	lt.setCursor(sel)
}

func (lt *ListTable) Up() {
	lt.Scroll(-1)
}

func (lt *ListTable) Down() {
	lt.Scroll(1)
}

func (lt *ListTable) PageUp() {
	lt.Scroll(-1 * (lt.Inner.Dy() - 1))
}

func (lt *ListTable) PageDown() {
	lt.Scroll(lt.Inner.Dy() - 1)
}

func (lt *ListTable) SelectedRow() []string {
	if len(lt.rows) == 0 {
		return nil
	}
	return lt.rows[lt.selectedRow]
}

func (lt *ListTable) setCursor(idx int) bool {
	if idx >= 0 && idx < len(lt.rows) {
		changed := lt.selectedRow != idx
		lt.selectedRow = idx
		if c, ok := lt.eventHandler.(ListTableCursorChangable); ok && changed {
			c.OnCursorChange(lt.SelectedRow())
			return true
		}
		return true
	}
	return false
}

func (lt *ListTable) setCursorViaMouse(mouse ui.Mouse) bool {
	yOffset := 1
	if len(lt.header) > 0 {
		yOffset++
	}
	relativeY := mouse.Y - lt.Rectangle.Min.Y
	relativeX := mouse.X - lt.Rectangle.Min.X
	if relativeX < 1 || relativeX > lt.Rectangle.Dx()-1 {
		return false
	}
	return lt.setCursor(relativeY - yOffset + lt.topRow)
}

func (lt *ListTable) OnFocusIn() {
	lt.BorderStyle = theme.Theme["grid"].Active
	lt.TitleStyle = theme.Theme["title"].Active
	lt.RowStyle = theme.Theme["listItem"].Active
	lt.HeaderStyle = theme.Theme["listHeader"].Active
	lt.SelectedRowStyle = theme.Theme["listItemSelected"].Active
}

func (lt *ListTable) OnFocusOut() {
	lt.BorderStyle = theme.Theme["grid"].Inactive
	lt.TitleStyle = theme.Theme["title"].Inactive
	lt.RowStyle = theme.Theme["listItem"].Inactive
	lt.HeaderStyle = theme.Theme["listHeader"].Inactive
	lt.SelectedRowStyle = theme.Theme["listItemSelected"].Inactive
}

func (lt *ListTable) GetHotKeys() []*HotKey {
	var keys []*HotKey
	if a, ok := lt.eventHandler.(ListTableHandlerWithActions); ok {
		for _, action := range a.GetActions() {
			keys = append(keys, &HotKey{
				Name: action.Name,
				Key:  action.HotKeyDisplay,
			})
		}
	}
	return keys
}
