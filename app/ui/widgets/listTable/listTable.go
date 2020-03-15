package listTable

import (
	"errors"
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"strings"
)

type (
	RowFunc         func(rowId int, row commander.Row) bool
	RowKeyEventFunc func(rowId int, row commander.Row, event *tcell.EventKey) bool
)

var (
	DefaultRowFunc         = func(rowId int, row commander.Row) bool { return false }
	DefaultRowKeyEventFunc = func(rowId int, row commander.Row, event *tcell.EventKey) bool { return false }
)

const (
	columnSeparator    = '|'
	columnSeparatorLen = 1
)

const HeaderRowId = -1

var DefaultStyler commander.ListViewStyler = func(list commander.ListView, rowId int, row commander.Row) commander.Style {
	if rowId == HeaderRowId {
		return theme.Default.Underline(true)
	} else if rowId == list.SelectedRowId() {
		if list.IsFocused() {
			return theme.ActiveFocused
		} else {
			return theme.ActiveUnfocused
		}
	}
	return theme.Default
}

type ListTable struct {
	views.WidgetWatchers
	*focus.Focusable

	view       views.View
	columns    []string
	rows       []commander.Row
	showHeader bool
	// internal representation of table values
	table table
	// Currently selected row
	selectedRow int
	// Row to start rendering from (vertical scrolling)
	topRow int
	// Left cell to start rendering from (horizontal scrolling)
	leftCell int

	onChange   RowFunc
	onKeyEvent RowKeyEventFunc

	styler commander.ListViewStyler
}

func (lt *ListTable) Rows() []commander.Row {
	return lt.rows
}

func NewList(lines []string) *ListTable {
	var rows []commander.Row
	for _, line := range lines {
		rows = append(rows, commander.Row{line})
	}
	return NewListTable([]string{""}, rows, false)
}

func NewListTable(columns []string, rows []commander.Row, showHeader bool) *ListTable {
	lt := &ListTable{
		Focusable:  focus.NewFocusable(),
		columns:    columns,
		rows:       rows,
		showHeader: showHeader,

		onKeyEvent: DefaultRowKeyEventFunc,
		onChange:   DefaultRowFunc,
		styler:     DefaultStyler,
	}
	lt.table = lt.renderTable()
	return lt
}

type table struct {
	headers []string
	values  [][]string

	columnDataWidths []int
	dataWidth        int
	dataHeight       int
}

func (lt *ListTable) SelectedRowId() int {
	return lt.selectedRow
}

func (lt *ListTable) SelectedRow() commander.Row {
	if lt.selectedRow < len(lt.rows) {
		return lt.rows[lt.selectedRow]
	}
	return nil
}

func (lt *ListTable) SetStyler(styler commander.ListViewStyler) {
	lt.styler = styler
}

func (lt *ListTable) BindOnChange(rowFunc RowFunc) {
	oldFunc := lt.onChange
	lt.onChange = func(rowId int, row commander.Row) bool {
		if rowFunc(rowId, row) {
			return true
		}
		return oldFunc(rowId, row)
	}
}

func (lt *ListTable) BindOnKeyPress(rowKeyEventFunc RowKeyEventFunc) {
	oldFunc := lt.onKeyEvent
	lt.onKeyEvent = func(rowId int, row commander.Row, event *tcell.EventKey) bool {
		if rowKeyEventFunc(rowId, row, event) {
			return true
		}
		return oldFunc(rowId, row, event)
	}
}

func (lt *ListTable) columnSeparatorsWidth() int {
	return (len(lt.columns) - 1) * columnSeparatorLen
}

func (lt *ListTable) viewWidth() int {
	width, _ := lt.view.Size()
	return width
}

func (lt *ListTable) viewHeight() int {
	_, height := lt.view.Size()
	if lt.showHeader {
		height -= 1
	}
	return height
}

func (lt *ListTable) MaxSize() (w int, h int) {
	w = lt.table.dataWidth + len(lt.table.columnDataWidths) - 1

	h = lt.table.dataHeight
	if lt.showHeader {
		h++
	}
	return w, h
}

func (lt *ListTable) renderTable() table {
	t := table{}
	t.dataHeight = len(lt.rows)
	t.columnDataWidths = []int{}
	if lt.showHeader {
		for _, col := range lt.columns {
			t.headers = append(t.headers, col)
			t.columnDataWidths = append(t.columnDataWidths, len(col))
		}
		t.dataHeight += 1
	} else {
		t.columnDataWidths = make([]int, len(lt.columns))
	}
	for _, row := range lt.rows {
		var mRow []string
		for colId := range lt.columns {
			var (
				err   error
				value string
			)
			if colId > len(row)-1 {
				err = errors.New("no val")
			} else {
				value = row[colId]
			}
			if err != nil {
				value = "err: " + err.Error()
			}
			if len(value) > t.columnDataWidths[colId] {
				t.columnDataWidths[colId] = len(value)
			}
			mRow = append(mRow, value)
		}
		t.values = append(t.values, mRow)
	}
	t.dataWidth = 0
	for _, width := range t.columnDataWidths {
		t.dataWidth += width
	}
	return t
}

func (lt *ListTable) getColumnSizes() []int {
	t := lt.table
	sizes := make([]int, len(t.columnDataWidths))
	copy(sizes, t.columnDataWidths)

	// If there is some additional horizontal space available - spread it in a rational way
	viewWidth, _ := lt.view.Size()
	viewWidth -= lt.columnSeparatorsWidth()
	unusedWidth := viewWidth - t.dataWidth
	addedWidth := 0
	if unusedWidth > 0 {
		for i, size := range sizes {
			var add int
			if i == len(sizes)-1 {
				// expand last row to the maximum to avoid empty cells due to rounding error
				add = unusedWidth - addedWidth
			} else {
				// otherwise give column extra space based on current viewWidth ratio
				ratio := float64(size) / float64(t.dataWidth)
				add = int(ratio * float64(unusedWidth))
			}
			sizes[i] += add
			addedWidth += add
		}
	}

	return sizes
}

func (lt *ListTable) Draw() {
	style := lt.defaultStyle()
	lt.view.Fill(' ', style)
	index := 0
	sizes := lt.getColumnSizes()
	if lt.showHeader {
		lt.drawRow(index, lt.table.headers, sizes, lt.styler(lt, -1, nil))
		index++
	}
	for rowId := lt.topRow; rowId < lt.topRow+lt.viewHeight() && rowId < lt.topRow+len(lt.rows); rowId++ {
		row := lt.table.values[rowId]
		lt.drawRow(index, row, sizes, lt.styler(lt, rowId, row))
		index++
	}
}

func (lt *ListTable) defaultStyle() tcell.Style {
	return tcell.StyleDefault.Background(tcell.ColorTeal)
}

func (lt *ListTable) drawRow(y int, row []string, sizes []int, style tcell.Style) {
	rowString := ""
	for i, val := range row {
		rowString += val
		if len(val) < sizes[i] {
			rowString += strings.Repeat(" ", sizes[i]-len(val))
		}
		if i < len(row)-1 {
			rowString += string(columnSeparator)
		}
	}
	rowString = rowString[lt.leftCell:]
	for x, ch := range rowString {
		lt.view.SetContent(x, y, ch, nil, style)
	}
}

func (lt *ListTable) Resize() {
	lt.table = lt.renderTable()
}

func (lt *ListTable) HandleEvent(ev tcell.Event) bool {
	return KeySwitch(ev, func(ev *tcell.EventKey) bool {
		switch ev.Key() {
		case tcell.KeyDown:
			lt.Next()
			return true
		case tcell.KeyUp:
			lt.Prev()
			return true
		case tcell.KeyPgDn:
			lt.NextPage()
			return true
		case tcell.KeyPgUp:
			lt.PrevPage()
			return true
		case tcell.KeyHome:
			lt.Home()
			return true
		case tcell.KeyEnd:
			lt.End()
			return true
		case tcell.KeyRight:
			lt.Right()
			return true
		case tcell.KeyLeft:
			lt.Left()
			return true
		}
		return lt.onKeyEvent(lt.selectedRow, lt.SelectedRow(), ev)
	})
}

func (lt *ListTable) Next() {
	lt.Select(lt.selectedRow + 1)
}

func (lt *ListTable) Prev() {
	lt.Select(lt.selectedRow - 1)
}

func (lt *ListTable) NextPage() {
	lt.Select(lt.selectedRow + lt.viewHeight())
}

func (lt *ListTable) PrevPage() {
	lt.Select(lt.selectedRow - lt.viewHeight())
}

func (lt *ListTable) Home() {
	lt.Select(0)
}

func (lt *ListTable) End() {
	lt.Select(len(lt.rows) - 1)
}

func (lt *ListTable) Right() {
	lt.SetLeft(lt.leftCell + 5)
}

func (lt *ListTable) Left() {
	lt.SetLeft(lt.leftCell - 5)
}

func (lt *ListTable) Select(index int) {
	if len(lt.rows) == 0 {
		return
	}

	if index > len(lt.rows)-1 {
		index = len(lt.rows) - 1
	}
	if index < 0 {
		index = 0
	}
	if lt.selectedRow == index {
		return
	}
	lt.selectedRow = index

	lt.onChange(lt.selectedRow, lt.SelectedRow())

	height := lt.viewHeight()
	scrollThreshold := lt.topRow + height - 1
	if index > scrollThreshold {
		lt.topRow = index - height + 1
	} else if index < lt.topRow {
		lt.topRow = index
	}
}

func (lt *ListTable) SetLeft(index int) {
	if index < 0 {
		index = 0
	}
	maxLeft := lt.table.dataWidth + lt.columnSeparatorsWidth() - lt.viewWidth()
	if maxLeft < 0 {
		index = 0
	} else if index > maxLeft {
		index = maxLeft
	}
	lt.leftCell = index
}

func (lt *ListTable) SetView(view views.View) {
	lt.view = view
	lt.Resize()
}

func (lt *ListTable) ShowHeader(showHeader bool) {
	lt.showHeader = showHeader
}

// This is the minimum required size of ListTable
func (lt *ListTable) Size() (int, int) {
	return 10, 3
}
