package listTable

import (
	"errors"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"strings"
	"time"
)

type rowEvent struct {
	event views.EventWidget
	t     time.Time
	lt    *ListTable
	rowId int
	row   Row
}

func (r rowEvent) Event() views.EventWidget {
	return r.event
}

func (r rowEvent) When() time.Time {
	return r.t
}

func (r rowEvent) Widget() views.Widget {
	return r.lt
}

func (r rowEvent) ListTable() *ListTable {
	return r.lt
}

func (r rowEvent) RowId() int {
	return r.rowId
}

func (r rowEvent) Row() Row {
	return r.row
}

type RowEvent interface {
	Event() views.EventWidget
	ListTable() *ListTable
	RowId() int
	Row() Row
}

type RowEventChange struct {
	rowEvent
}

type RowTcellEvent struct {
	rowEvent
	ev tcell.Event
}

func (e RowTcellEvent) TcellEvent() tcell.Event {
	return e.ev
}

type Row []interface{}

type RowEventHandler interface {
	HandleRowEvent(event RowEvent) bool
}

const (
	columnSeparator = "|"
)

type ListTable struct {
	view       views.View
	columns    []Column
	rows       []Row
	showHeader bool

	// internal representation of table values
	table table
	// Currently selected row
	selectedRow int
	// Row to start rendering from (vertical scrolling)
	topRow int
	// Left cell to start rendering from (horizontal scrolling)
	leftCell int

	rowEventHandler RowEventHandler

	views.WidgetWatchers
	*focus.Focusable
}

func NewList(lines []string) *ListTable {
	var rows []Row
	for _, line := range lines {
		rows = append(rows, Row{line})
	}
	return &ListTable{
		columns: []Column{
			NewStringColumn(""),
		},
		rows:       rows,
		showHeader: false,
		Focusable:  focus.NewFocusable(),
	}
}

func NewListTable(columns []Column, rows []Row, showHeader bool) *ListTable {
	lt := &ListTable{
		columns:    columns,
		rows:       rows,
		showHeader: showHeader,
		Focusable:  focus.NewFocusable(),
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

func (lt *ListTable) columnSeparatorsWidth() int {
	return (len(lt.columns) - 1) * len(columnSeparator)
}

func (lt *ListTable) viewWidth() int {
	width, _ := lt.view.Size()
	return width
}

func (lt *ListTable) viewHeight() int {
	_, height := lt.view.Size()
	return height
}

func (lt *ListTable) renderTable() table {
	t := table{}
	t.dataHeight = len(lt.rows)
	t.columnDataWidths = []int{}
	if lt.showHeader {
		for _, col := range lt.columns {
			header := col.Header()
			t.headers = append(t.headers, header)
			t.columnDataWidths = append(t.columnDataWidths, len(header))
		}
		t.dataHeight += 1
	} else {
		t.columnDataWidths = make([]int, len(lt.columns))
	}
	for _, row := range lt.rows {
		var mRow []string
		for colId, col := range lt.columns {
			var (
				err   error
				value string
			)
			if colId > len(row)-1 {
				err = errors.New("no val")
			} else {
				value, err = col.Render(row[colId])
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
	sizes := t.columnDataWidths

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
		lt.drawRow(index, lt.table.headers, sizes, lt.headerStyle())
		index++
	}
	for rowId := lt.topRow; rowId < lt.topRow+lt.viewHeight() && rowId < lt.topRow+len(lt.rows); rowId++ {
		row := lt.table.values[rowId]
		var rowStyle tcell.Style
		if rowId == lt.selectedRow {
			rowStyle = lt.selectedRowStyle()
		} else {
			rowStyle = lt.rowStyle()
		}
		lt.drawRow(index, row, sizes, rowStyle)
		index++
	}
}

func (lt *ListTable) defaultStyle() tcell.Style {
	return tcell.StyleDefault.Background(tcell.ColorTeal)
}

func (lt *ListTable) headerStyle() tcell.Style {
	return lt.rowStyle().Foreground(tcell.ColorWhite).Underline(true)
}

func (lt *ListTable) selectedRowStyle() tcell.Style {
	if lt.IsFocused() {
		return lt.rowStyle().Background(tcell.ColorLightCyan)
	} else {
		return lt.rowStyle().Background(tcell.ColorDarkGray)
	}
}

func (lt *ListTable) rowStyle() tcell.Style {
	return lt.defaultStyle().Foreground(tcell.ColorBlack)
}

func (lt *ListTable) drawRow(y int, row []string, sizes []int, style tcell.Style) {
	rowString := ""
	for i, val := range row {
		rowString += val
		if len(val) < sizes[i] {
			rowString += strings.Repeat(" ", sizes[i]-len(val))
		}
		if i < len(row)-1 {
			rowString += columnSeparator
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
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyDown:
			lt.Next()
			return true
		case tcell.KeyUp:
			lt.Prev()
			return true
		case tcell.KeyRight:
			lt.Right()
			return true
		case tcell.KeyLeft:
			lt.Left()
			return true
		}
	}
	return lt.raiseEvent(&RowTcellEvent{
		ev:       ev,
		rowEvent: lt.newRowEvent(),
	})
}

func (lt *ListTable) raiseEvent(event RowEvent) bool {
	if lt.rowEventHandler != nil {
		return lt.rowEventHandler.HandleRowEvent(event)
	} else {
		return false
	}
}

func (lt *ListTable) Next() {
	lt.Select(lt.selectedRow + 1)
}

func (lt *ListTable) Prev() {
	lt.Select(lt.selectedRow - 1)
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

	lt.raiseEvent(&RowEventChange{
		rowEvent: lt.newRowEvent(),
	})

	height := lt.table.dataHeight
	if index > lt.topRow+height-1 {
		lt.topRow = index - height + 1
	} else if index < lt.topRow {
		lt.topRow = index
	}
}

func (lt *ListTable) newRowEvent() rowEvent {
	var row Row
	if lt.selectedRow < len(lt.rows) {
		row = lt.rows[lt.selectedRow]
	}
	return rowEvent{
		t:     time.Now(),
		lt:    lt,
		rowId: lt.selectedRow,
		row:   row,
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

func (lt *ListTable) SetEventHandler(handler RowEventHandler) {
	lt.rowEventHandler = handler
}
