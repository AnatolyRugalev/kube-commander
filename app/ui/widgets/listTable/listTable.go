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
	RowFunc         func(row commander.Row) bool
	RowKeyEventFunc func(row commander.Row, event *tcell.EventKey) bool
)

var (
	DefaultRowFunc         = func(row commander.Row) bool { return false }
	DefaultRowKeyEventFunc = func(row commander.Row, event *tcell.EventKey) bool { return false }
)

const (
	columnSeparator    = '|'
	columnSeparatorLen = 1
)

type TableFormat uint8

const (
	WithHeaders TableFormat = 1 << iota
	Wide
	Short
	NameOnly
	NoHorizontalScroll
	NoVerticalScroll
	NoActions
)

func (tf TableFormat) Has(flag TableFormat) bool {
	return tf&flag != 0
}

var DefaultStyler commander.ListViewStyler = func(list commander.ListView, row commander.Row) commander.Style {
	if row == nil {
		return theme.Default.Underline(true)
	} else if row.Id() == list.SelectedRowId() {
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
	rowIndex   map[string]int
	selectedId string
	format     TableFormat
	// internal representation of table values
	table table
	// Currently selected row
	selectedRowIndex int
	// Row to start rendering from (vertical scrolling)
	topRow int
	// Left cell to start rendering from (horizontal scrolling)
	leftCell int

	onChange   RowFunc
	onKeyEvent RowKeyEventFunc

	styler      commander.ListViewStyler
	preloader   *preloader
	rowProvider commander.RowProvider
	updater     commander.ScreenUpdater
	stopCh      chan struct{}
}

func (lt *ListTable) Rows() []commander.Row {
	return lt.rows
}

func NewListTable(prov commander.RowProvider, format TableFormat, updater commander.ScreenUpdater) *ListTable {
	lt := &ListTable{
		Focusable: focus.NewFocusable(),
		format:    format,
		rowIndex:  make(map[string]int),

		onKeyEvent:  DefaultRowKeyEventFunc,
		onChange:    DefaultRowFunc,
		styler:      DefaultStyler,
		preloader:   NewPreloader(updater),
		rowProvider: prov,
		updater:     updater,
	}
	lt.table = lt.renderTable()
	return lt
}

func (lt *ListTable) OnShow() {
	lt.stopCh = make(chan struct{})
	go lt.watch()
	lt.Focusable.OnShow()
}

func (lt *ListTable) OnHide() {
	lt.Focusable.OnShow()
	close(lt.stopCh)
}

func (lt *ListTable) watch() {
	for {
		select {
		case <-lt.stopCh:
			return
		case ops, ok := <-lt.rowProvider:
			if !ok {
				return
			}
			changed := false
			for _, op := range ops {
				switch op.Type {
				case commander.OpClear:
					if len(lt.rows) > 0 {
						lt.rows = []commander.Row{}
						lt.rowIndex = make(map[string]int)
						changed = true
					}
					if len(lt.columns) > 0 {
						lt.columns = []string{}
						changed = true
					}
				case commander.OpColumns:
					if len(lt.columns) != len(op.Row.Cells()) {
						// TODO: compare contents?
						lt.columns = op.Row.Cells()
						changed = true
					}
				case commander.OpAdded:
					_, ok := lt.rowIndex[op.Row.Id()]
					if !ok {
						lt.rowIndex[op.Row.Id()] = len(lt.rows)
						lt.rows = append(lt.rows, op.Row)
						changed = true
					}
				case commander.OpDeleted:
					index, ok := lt.rowIndex[op.Row.Id()]
					if ok {
						lt.rows = append(lt.rows[:index], lt.rows[index+1:]...)
						delete(lt.rowIndex, op.Row.Id())
						for _, row := range lt.rows[index:] {
							lt.rowIndex[row.Id()]--
						}
						changed = true
					}
				case commander.OpModified:
					index, ok := lt.rowIndex[op.Row.Id()]
					if ok {
						// TODO: compare contents?
						lt.rows[index] = op.Row
						changed = true
					} else {
						lt.rows = append(lt.rows, op.Row)
						changed = true
					}
				case commander.OpLoading:
					lt.preloader.Start()
				case commander.OpLoadingFinished:
					lt.preloader.Stop()
				}
			}
			if changed {
				lt.reindexSelection()
				lt.table = lt.renderTable()
				if lt.updater != nil {
					lt.updater.Resize()
					lt.updater.UpdateScreen()
				}
			}
		}
	}
}

type table struct {
	headers []string
	values  [][]string

	columnDataWidths []int
	dataWidth        int
	dataHeight       int
}

func (lt *ListTable) SelectedRowIndex() int {
	return lt.selectedRowIndex
}

func (lt *ListTable) SelectedRowId() string {
	return lt.selectedId
}

func (lt *ListTable) SelectedRow() commander.Row {
	if len(lt.rows) == 0 {
		return nil
	}
	if lt.selectedRowIndex < len(lt.rows) {
		return lt.rows[lt.selectedRowIndex]
	}
	return nil
}

func (lt *ListTable) SetStyler(styler commander.ListViewStyler) {
	lt.styler = styler
}

func (lt *ListTable) BindOnChange(rowFunc RowFunc) {
	oldFunc := lt.onChange
	lt.onChange = func(row commander.Row) bool {
		if rowFunc(row) {
			return true
		}
		return oldFunc(row)
	}
}

func (lt *ListTable) RowById(id string) commander.Row {
	if index, ok := lt.rowIndex[id]; ok {
		return lt.rows[index]
	}
	return nil
}

func (lt *ListTable) BindOnKeyPress(rowKeyEventFunc RowKeyEventFunc) {
	oldFunc := lt.onKeyEvent
	lt.onKeyEvent = func(row commander.Row, event *tcell.EventKey) bool {
		if rowKeyEventFunc(row, event) {
			return true
		}
		return oldFunc(row, event)
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
	if lt.format.Has(WithHeaders) {
		height -= 1
	}
	return height
}

func (lt *ListTable) MaxSize() (w int, h int) {
	w = lt.table.dataWidth + len(lt.table.columnDataWidths) - 1

	h = lt.table.dataHeight
	if lt.format.Has(WithHeaders) {
		h++
	}
	return w, h
}

func (lt *ListTable) renderTable() table {
	t := table{}
	t.dataHeight = len(lt.rows)
	t.columnDataWidths = []int{}
	if lt.format.Has(WithHeaders) {
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
			cells := row.Cells()
			if colId > len(cells)-1 {
				err = errors.New("no val")
			} else {
				value = cells[colId]
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
	if lt.format.Has(WithHeaders) {
		lt.drawRow(index, lt.table.headers, sizes, lt.styler(lt, nil))
		index++
	}
	for rowId := lt.topRow; rowId < lt.topRow+lt.viewHeight() && rowId < len(lt.rows); rowId++ {
		lt.drawRow(index, lt.table.values[rowId], sizes, lt.styler(lt, lt.rows[rowId]))
		index++
	}
	lt.preloader.Draw()
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
		if ev.Modifiers() != tcell.ModNone {
			return false
		}
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
		return lt.onKeyEvent(lt.SelectedRow(), ev)
	})
}

func (lt *ListTable) Next() {
	lt.SelectIndex(lt.selectedRowIndex + 1)
}

func (lt *ListTable) Prev() {
	lt.SelectIndex(lt.selectedRowIndex - 1)
}

func (lt *ListTable) NextPage() {
	lt.SelectIndex(lt.selectedRowIndex + lt.viewHeight())
}

func (lt *ListTable) PrevPage() {
	lt.SelectIndex(lt.selectedRowIndex - lt.viewHeight())
}

func (lt *ListTable) Home() {
	lt.SelectIndex(0)
}

func (lt *ListTable) End() {
	lt.SelectIndex(len(lt.rows) - 1)
}

func (lt *ListTable) Right() {
	lt.SetLeft(lt.leftCell + 5)
}

func (lt *ListTable) Left() {
	lt.SetLeft(lt.leftCell - 5)
}

func (lt *ListTable) SelectIndex(index int) {
	if len(lt.rows) == 0 {
		return
	}

	if index > len(lt.rows)-1 {
		index = len(lt.rows) - 1
	}
	if index < 0 {
		index = 0
	}
	row := lt.rows[index]
	lt.selectedId = row.Id()
	if lt.selectedRowIndex == index {
		return
	}
	lt.selectedRowIndex = index
	lt.onChange(row)

	height := lt.viewHeight()
	scrollThreshold := lt.topRow + height - 1
	if height <= 0 {
		lt.topRow = 0
	} else if index > scrollThreshold {
		lt.topRow = index - height + 1
	} else if index < lt.topRow {
		lt.topRow = index
	}
}

func (lt *ListTable) SelectId(id string) {
	lt.selectedId = id
	lt.reindexSelection()
}

func (lt *ListTable) reindexSelection() {
	if lt.selectedId == "" {
		lt.SelectIndex(0)
	} else if index, ok := lt.rowIndex[lt.selectedId]; ok {
		lt.SelectIndex(index)
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
	lt.preloader.SetView(view)
	lt.Resize()
}

// This is the minimum required size of ListTable
func (lt *ListTable) Size() (int, int) {
	if lt.table.dataWidth == 0 {
		return 10, 3
	}
	w, h := lt.MaxSize()
	viewW, viewH := lt.view.Size()
	if !lt.format.Has(NoHorizontalScroll) && w > viewW {
		w = viewW
	}
	if !lt.format.Has(NoVerticalScroll) && h > viewH {
		h = viewH
	}
	return w, h
}
