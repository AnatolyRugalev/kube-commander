package listTable

import (
	"errors"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"strings"
)

type Row []interface{}

type Event struct {
	tcell.Event
	ListTable *ListTable
	RowId     int
	Row       *Row
}

type EventHandler interface {
	HandleListEvent(event *Event) bool
}

const (
	columnSeparator = "|"
)

type ListTable struct {
	view       views.View
	columns    []Column
	rows       []Row
	handler    tcell.EventHandler
	showHeader bool

	// internal representation of table values
	table table
	// Currently selected row
	selectedRow int
	// Row to start rendering from (vertical scrolling)
	topRow int
	// Left cell to start rendering from (horizontal scrolling)
	leftCell int

	eventHandler EventHandler

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
	return &ListTable{
		columns:    columns,
		rows:       rows,
		showHeader: showHeader,
		Focusable:  focus.NewFocusable(),
	}
}

type table struct {
	headers []string
	values  [][]string
	sizes   []int
	width   int
}

// dataBounds returns data size bounds for internal calculations
func (lt *ListTable) dataBounds(withoutSeparators bool) (int, int) {
	width, height := lt.view.Size()
	if withoutSeparators {
		width -= (len(lt.columns) - 1) * len(columnSeparator)
	}
	if lt.showHeader {
		height--
	}
	return width, height
}

func (lt *ListTable) renderTable() table {
	t := table{}
	dataWidth, _ := lt.dataBounds(true)
	if lt.showHeader {
		for _, col := range lt.columns {
			header := col.Header()
			t.headers = append(t.headers, header)
			t.sizes = append(t.sizes, len(header))
		}
	} else {
		t.sizes = make([]int, len(lt.columns))
	}
	for _, row := range lt.rows {
		var mRow []string
		for colId, col := range lt.columns {
			var (
				err   error
				value string
			)
			if len(row) < colId-1 {
				err = errors.New("no val")
			} else {
				value, err = col.Render(row[colId])
			}
			if err != nil {
				value = "err: " + err.Error()
			}
			mRow = append(mRow, value)
			if l := len(value); l > t.sizes[colId] {
				t.sizes[colId] = l
			}
		}
		t.values = append(t.values, mRow)
	}

	// Trying to balance column sizes
	usedWidth := 0
	for _, size := range t.sizes {
		usedWidth += size
	}
	// If there is some additional horizontal space available - spread it in a rational way
	addedWidth := 0
	if dataWidth > usedWidth {
		unusedWidth := dataWidth - usedWidth
		for i, size := range t.sizes {
			var add int
			if i == len(t.sizes)-1 {
				// expand last row to the maximum to avoid empty cells due to rounding error
				add = unusedWidth - addedWidth
			} else {
				// otherwise give column extra space based on current width ratio
				ratio := float64(size) / float64(usedWidth)
				add = int(ratio * float64(unusedWidth))
			}
			t.sizes[i] += add
			addedWidth += add
		}
	}
	t.width = usedWidth + addedWidth
	return t
}

func (lt *ListTable) Draw() {
	lt.view.Fill(' ', tcell.StyleDefault)
	lt.table = lt.renderTable()
	index := 0
	if lt.showHeader {
		lt.drawRow(index, lt.table.headers, lt.table.sizes, tcell.StyleDefault.Bold(true))
		index++
	}
	_, dataHeight := lt.dataBounds(false)
	for rowId := lt.topRow; rowId < lt.topRow+dataHeight && rowId < lt.topRow+len(lt.rows); rowId++ {
		row := lt.table.values[rowId]
		var style tcell.Style
		if rowId == lt.selectedRow {
			style = tcell.StyleDefault.Background(tcell.ColorNavajoWhite)
		} else {
			style = tcell.StyleDefault
		}
		lt.drawRow(index, row, lt.table.sizes, style)
		index++
	}
}

func (lt *ListTable) drawRow(y int, row []string, sizes []int, style tcell.Style) {
	rowString := ""
	for i, val := range row {
		rowString += val + strings.Repeat(" ", sizes[i]-len(val))
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

}

func (lt *ListTable) HandleEvent(ev tcell.Event) bool {
	if !lt.Focus() {
		return false
	}
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
		case tcell.KeyEsc:
			lt.PostEvent(focus.NewBlurEvent(lt))
			return true
		}
	}
	if lt.eventHandler != nil {
		return lt.eventHandler.HandleListEvent(&Event{
			Event:     ev,
			ListTable: lt,
			RowId:     lt.selectedRow,
			Row:       &lt.rows[lt.selectedRow],
		})
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
	if index < 0 {
		index = 0
	} else if index > len(lt.rows)-1 {
		index = len(lt.rows) - 1
	}
	lt.selectedRow = index

	_, height := lt.dataBounds(false)
	if index > lt.topRow+height-1 {
		lt.topRow = index - height + 1
	} else if index < lt.topRow {
		lt.topRow = index
	}
}

func (lt *ListTable) SetLeft(index int) {
	if index < 0 {
		index = 0
	}
	width, _ := lt.view.Size()
	maxLeft := (lt.table.width + (len(lt.columns)-1)*len(columnSeparator)) - width
	if index > maxLeft {
		index = maxLeft
	}
	lt.leftCell = index
}

func (lt *ListTable) SetView(view views.View) {
	lt.view = view
}

func (lt *ListTable) Size() (int, int) {
	return lt.view.Size()
}

func (lt *ListTable) SetEventHandler(handler EventHandler) {
	lt.eventHandler = handler
}
