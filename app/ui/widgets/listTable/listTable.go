package listTable

import (
	"errors"
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"github.com/mattn/go-runewidth"
	"strings"
	"time"
)

type (
	RowFunc         func(row commander.Row) bool
	RowKeyEventFunc func(row commander.Row, event *tcell.EventKey) bool
	InitFunc        func()
)

var (
	DefaultRowFunc         = func(row commander.Row) bool { return false }
	DefaultRowKeyEventFunc = func(row commander.Row, event *tcell.EventKey) bool { return false }
	DefaultInit            = func() {}
)

const (
	columnSeparator    = '|'
	columnSeparatorLen = 1
)

type TableFormat uint16

const (
	WithHeaders TableFormat = 1 << iota
	Wide
	Short
	NameOnly
	NoHorizontalScroll
	NoVerticalScroll
	NoActions
	NoWatch
	WithFilter
)

func (tf TableFormat) Has(flag TableFormat) bool {
	return tf&flag != 0
}

var DefaultStyler commander.ListViewStyler = func(list commander.ListView, row commander.Row) commander.Style {
	style := theme.Default
	if row == nil {
		style = style.Underline(true)
	} else if row.Id() == list.SelectedRowId() {
		if list.IsFocused() {
			style = style.Background(theme.ColorSelectedFocusedBackground)
		} else {
			style = style.Background(theme.ColorSelectedUnfocusedBackground)
		}
	}
	if row != nil && !row.Enabled() {
		style = style.Foreground(theme.ColorDisabledForeground)
	}
	return style
}

type ListTable struct {
	views.WidgetWatchers
	*focus.Focusable

	view       views.View
	columns    []string
	ageCol     int
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

	onChange     RowFunc
	onKeyEvent   RowKeyEventFunc
	onInitStart  InitFunc
	onInitFinish InitFunc

	styler      commander.ListViewStyler
	preloader   *preloader
	rowProvider commander.RowProvider
	updater     commander.ScreenUpdater
	stopCh      chan struct{}

	filter     string
	filterMode bool

	stRow               commander.StyleComponent
	stHeader            commander.StyleComponent
	stSelectedFocused   commander.StyleComponent
	stSelectedUnfocused commander.StyleComponent
	stDisabled          commander.StyleComponent
	stFilter            commander.StyleComponent
	stFilterActive      commander.StyleComponent
}

func (lt *ListTable) GetComponents() []commander.StyleComponent {
	return []commander.StyleComponent{
		lt.stRow,
		lt.stHeader,
		lt.stSelectedFocused,
		lt.stSelectedUnfocused,
		lt.stFilter,
		lt.stFilterActive,
	}
}

func (lt *ListTable) Rows() []commander.Row {
	return lt.rows
}

func NewListTable(prov commander.RowProvider, format TableFormat, updater commander.ScreenUpdater) *ListTable {
	lt := &ListTable{
		Focusable: focus.NewFocusable(),
		format:    format,
		rowIndex:  make(map[string]int),
		ageCol:    -1,

		onKeyEvent:   DefaultRowKeyEventFunc,
		onChange:     DefaultRowFunc,
		onInitStart:  DefaultInit,
		onInitFinish: DefaultInit,
		styler:       DefaultStyler,
		preloader:    NewPreloader(updater),
		rowProvider:  prov,
		updater:      updater,

		stRow:               theme.NewComponent("row", theme.Default),
		stHeader:            theme.NewComponent("header", theme.Default.Underline(true)),
		stSelectedFocused:   theme.NewComponent("selected-focused", theme.Default.Background(theme.ColorSelectedFocusedBackground)),
		stSelectedUnfocused: theme.NewComponent("selected-unfocused", theme.Default.Background(theme.ColorSelectedUnfocusedBackground)),
		stDisabled:          theme.NewComponent("disabled", theme.Default.Foreground(theme.ColorDisabledForeground)),
		stFilter:            theme.NewComponent("filter", theme.Default.Background(theme.ColorSelectedUnfocusedBackground)),
		stFilterActive:      theme.NewComponent("filter-active", theme.Default.Background(theme.ColorSelectedFocusedBackground)),
	}
	lt.Render()
	return lt
}

func (lt *ListTable) OnShow() {
	lt.stopCh = make(chan struct{})
	go lt.watch()
	lt.resetFilter()
	lt.Focusable.OnShow()
}

func (lt *ListTable) OnHide() {
	lt.Focusable.OnShow()
	close(lt.stopCh)
}

func (lt *ListTable) watch() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	for {
		select {
		case <-lt.stopCh:
			return
		case ops, ok := <-lt.rowProvider:
			if !ok {
				return
			}
			changed := false
			for _, operation := range ops {
				switch op := operation.(type) {
				case *commander.OpClear:
					lt.rows = []commander.Row{}
					lt.rowIndex = make(map[string]int)
					lt.columns = []string{}
					changed = true
				case *commander.OpSetColumns:
					// Compare columns content
					if strings.Join(lt.columns, "|") != strings.Join(op.Columns, "|") {
						lt.columns = op.Columns
						changed = true
					}
					lt.setAgeCol()
				case *commander.OpAdded:
					index, ok := lt.rowIndex[op.Row.Id()]
					if !ok {
						if op.Index == nil {
							if op.SortById {
								index = lt.getRowIndex(op.Row)
							} else {
								index = len(lt.rows)
							}
						} else {
							index = *op.Index
						}
						for key, val := range lt.rowIndex {
							if val >= index {
								lt.rowIndex[key] = val + 1
							}
						}
						lt.rowIndex[op.Row.Id()] = index
						lt.rows = append(lt.rows[:index], append([]commander.Row{op.Row}, lt.rows[index:]...)...)
						changed = true
					} else {
						// If row already exists
						lt.rows[index] = op.Row
						// TODO: move row if new index provided?
					}
				case *commander.OpDeleted:
					index, ok := lt.rowIndex[op.RowId]
					if ok {
						lt.rows = append(lt.rows[:index], lt.rows[index+1:]...)
						delete(lt.rowIndex, op.RowId)
						for _, row := range lt.rows[index:] {
							lt.rowIndex[row.Id()]--
						}
						changed = true
					}
				case *commander.OpModified:
					index, ok := lt.rowIndex[op.Row.Id()]
					if ok {
						// Compare cells content
						if strings.Join(lt.rows[index].Cells(), "|") != strings.Join(op.Row.Cells(), "|") {
							lt.rows[index] = op.Row
							changed = true
						}
						if lt.rows[index].Enabled() != op.Row.Enabled() {
							lt.rows[index] = op.Row
							changed = true
						}
					} else {
						lt.rows = append(lt.rows, op.Row)
						changed = true
					}
				case *commander.OpInitStart:
					lt.preloader.Start()
					lt.onInitStart()
				case *commander.OpInitFinished:
					lt.preloader.Stop()
					lt.onInitFinish()
				}
			}
			if changed {
				lt.Render()
				lt.reindexSelection()
				if lt.updater != nil {
					lt.updater.Resize()
					lt.updater.UpdateScreen()
				}
			}
		case <-ticker.C:
			// Periodically update list to ensure that age is somewhat relevant
			if lt.ageCol != -1 {
				lt.Render()
				if lt.updater != nil {
					lt.updater.Resize()
					lt.updater.UpdateScreen()
				}
			}
		}
	}
}

type table struct {
	rows    []commander.Row
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
	if len(lt.table.rows) == 0 {
		return nil
	}
	if lt.selectedRowIndex < len(lt.table.rows) {
		return lt.table.rows[lt.selectedRowIndex]
	}
	return nil
}

// deprecated
func (lt *ListTable) SetStyler(styler commander.ListViewStyler) {
	lt.styler = styler
}

func (lt *ListTable) rowStyle(row commander.Row) commander.Style {
	if row.Id() == lt.SelectedRowId() {
		if lt.IsFocused() {
			return lt.stSelectedFocused.Style()
		} else {
			return lt.stSelectedUnfocused.Style()
		}
	}
	if row != nil && !row.Enabled() {
		return lt.stDisabled.Style()
	}
	return lt.stRow.Style()

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

func (lt *ListTable) resetFilter() {
	lt.filterMode = false
	lt.filter = ""
	lt.Render()
	lt.reindexSelection()
}

func (lt *ListTable) BindOnInitFinish(initFunc InitFunc) {
	oldFunc := lt.onInitFinish
	lt.onInitFinish = func() {
		initFunc()
		oldFunc()
	}
}

func (lt *ListTable) BindOnInitStart(initFunc InitFunc) {
	oldFunc := lt.onInitStart
	lt.onInitStart = func() {
		initFunc()
		oldFunc()
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

func (lt *ListTable) tableHeight() int {
	_, height := lt.view.Size()
	if lt.format.Has(WithHeaders) {
		height -= 1
	}
	if lt.filterMode || lt.filter != "" {
		height -= 1
	}
	return height
}

func (lt *ListTable) MaxSize() (w int, h int) {
	w = lt.table.dataWidth + len(lt.table.columnDataWidths) - 1
	if lt.filterMode {
		filterLen := len(lt.filter) + 1
		if w < filterLen {
			w = filterLen
		}
	}

	h = lt.table.dataHeight
	if lt.format.Has(WithHeaders) {
		h++
	}
	return w, h
}

func (lt *ListTable) matchFilter(row commander.Row) bool {
	if lt.filter == "" {
		return true
	}
	for _, cell := range row.Cells() {
		if strings.Contains(cell, lt.filter) {
			return true
		}
	}
	return false
}

func (lt *ListTable) renderTable() table {
	t := table{}
	t.dataHeight = len(lt.rows)
	t.columnDataWidths = []int{}
	if lt.format.Has(WithHeaders) {
		for _, col := range lt.columns {
			t.headers = append(t.headers, col)
			t.columnDataWidths = append(t.columnDataWidths, runewidth.StringWidth(col))
		}
		t.dataHeight += 1
	} else {
		t.columnDataWidths = make([]int, len(lt.columns))
	}
	for _, row := range lt.rows {
		if !lt.matchFilter(row) {
			continue
		}
		cells := row.Cells()
		ageRow, _ := row.(commander.RowWithAge)
		var mRow []string
		for colId := range lt.columns {
			var (
				err   error
				value string
			)
			if colId == lt.ageCol && ageRow != nil {
				value = lt.renderAge(ageRow.Age())
			} else if colId > len(cells)-1 {
				err = errors.New("no val")
			} else {
				value = cells[colId]
			}
			if err != nil {
				value = "err: " + err.Error()
			}
			width := runewidth.StringWidth(value)
			if width > t.columnDataWidths[colId] {
				t.columnDataWidths[colId] = width
			}
			mRow = append(mRow, value)
		}
		t.values = append(t.values, mRow)
		t.rows = append(t.rows, row)
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
	if lt.filterMode || lt.filter != "" {
		lt.drawFilter(index)
		index++
	}
	sizes := lt.getColumnSizes()
	if lt.format.Has(WithHeaders) {
		lt.drawRow(index, lt.table.headers, sizes, lt.stHeader.Style())
		index++
	}
	for rowId := lt.topRow; rowId < lt.topRow+lt.tableHeight() && rowId < len(lt.table.rows); rowId++ {
		lt.drawRow(index, lt.table.values[rowId], sizes, lt.rowStyle(lt.table.rows[rowId]))
		index++
	}
	lt.preloader.Draw()
}

func (lt *ListTable) drawFilter(y int) {
	str := "/" + lt.filter
	x := 0
	var st commander.Style
	if lt.filterMode {
		st = lt.stFilterActive.Style()
	} else {
		st = lt.stFilter.Style()
	}
	for _, ch := range str {
		lt.view.SetContent(x, y, ch, nil, st)
		x++
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
	x := 0
	padding := 0
	for _, ch := range rowString {
		if runewidth.IsAmbiguousWidth(ch) {
			padding += 2
		}
		lt.view.SetContent(x, y, ch, nil, style)
		x++
	}
	for i := 0; i < padding; i++ {
		lt.view.SetContent(x+i, y, ' ', nil, style)
	}
}

func (lt *ListTable) Render() {
	lt.table = lt.renderTable()
}

func (lt *ListTable) Resize() {
	lt.Render()
	lt.reindexSelection()
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
		if lt.format.Has(WithFilter) {
			if (lt.filterMode || lt.filter != "") && ev.Key() == tcell.KeyEsc && lt.IsFocused() {
				lt.resetFilter()
				return true
			}
			if lt.filterMode {
				switch ev.Key() {
				case tcell.KeyBackspace2:
					if len(lt.filter) > 0 {
						lt.filter = lt.filter[:len(lt.filter)-1]
						lt.Render()
						lt.reindexSelection()
					}
					return true
				case tcell.KeyEnter:
					lt.filterMode = false
					lt.Render()
					lt.reindexSelection()
					return true
				}
				if ev.Rune() != 0 {
					lt.filter += string(ev.Rune())
					lt.Render()
					lt.reindexSelection()
					return true
				}
			} else {
				if ev.Rune() == '/' {
					lt.filterMode = true
					return true
				}
			}

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
	lt.SelectIndex(lt.selectedRowIndex + lt.tableHeight())
}

func (lt *ListTable) PrevPage() {
	lt.SelectIndex(lt.selectedRowIndex - lt.tableHeight())
}

func (lt *ListTable) Home() {
	lt.SelectIndex(0)
}

func (lt *ListTable) End() {
	lt.SelectIndex(len(lt.table.rows) - 1)
}

func (lt *ListTable) Right() {
	lt.SetLeft(lt.leftCell + 5)
}

func (lt *ListTable) Left() {
	lt.SetLeft(lt.leftCell - 5)
}

func (lt *ListTable) SelectIndex(index int) {
	if len(lt.table.rows) == 0 {
		return
	}

	if index > len(lt.table.rows)-1 {
		index = len(lt.table.rows) - 1
	}
	if index < 0 {
		index = 0
	}
	// Determine direction to skip disabled rows
	var delta int
	if lt.selectedRowIndex <= index {
		delta = 1
	} else {
		delta = -1
	}
	row := lt.table.rows[index]
	for !row.Enabled() {
		index += delta
		if index < 0 || index >= len(lt.table.rows) {
			return
		}
		row = lt.table.rows[index]
	}
	lt.selectedId = row.Id()
	changed := lt.selectedRowIndex != index
	lt.selectedRowIndex = index
	if changed {
		lt.onChange(row)
	}

	if lt.view == nil {
		return
	}

	height := lt.tableHeight()
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
	if index, ok := lt.rowIndex[lt.selectedId]; ok {
		lt.SelectIndex(index)
	} else {
		lt.SelectIndex(0)
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
		return 1, 1
	}
	w, h := lt.MaxSize()
	// Allows to extend the view from the outside
	if !lt.format.Has(NoHorizontalScroll) {
		w = 1
	}
	if !lt.format.Has(NoVerticalScroll) {
		h = 1
	}
	return w, h
}

func (lt *ListTable) getRowIndex(r commander.Row) int {
	for i, row := range lt.rows {
		if strings.Compare(r.Id(), row.Id()) != 1 {
			return i
		}
	}
	return 0
}

func (lt *ListTable) setAgeCol() {
	for i, n := range lt.columns {
		if n == "Age" {
			lt.ageCol = i
			return
		}
	}
	lt.ageCol = -1
}

func (lt *ListTable) renderAge(age time.Duration) string {
	if age > time.Hour*24 {
		days := age.Nanoseconds() / (time.Hour * 24).Nanoseconds()
		return fmt.Sprintf("%dd", days)
	}
	if age > time.Hour {
		hours := age.Nanoseconds() / time.Hour.Nanoseconds()
		return fmt.Sprintf("%dh", hours)
	}
	if age > time.Minute {
		minutes := age.Nanoseconds() / time.Minute.Nanoseconds()
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", int(age.Seconds()))
}
