package tui

import (
	"image"
	"sync"

	"github.com/AnatolyRugalev/kube-commander/internal/theme"
	"github.com/gizak/termui/v3"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type Action struct {
	Name      string
	HotKey    string
	Checked   bool
	Checkable bool
	OnExecute func(context []string) bool
}

type HotKeyPanel struct {
	hkey     []*widgets.Paragraph
	hkeyName []*widgets.Paragraph
	visibles []int
}

func (h *HotKeyPanel) adjustWidget(x1, y1, x2, y2 int) {
	var i, by1, bx2, by2, left int
	left = x1 + 1
	by1 = y2
	by2 = y2 + 1

	for vi := range h.visibles {
		i = h.visibles[vi]
		bx2 = left + len(h.hkey[i].Text)
		h.hkey[i].SetRect(left-1, by1-1, bx2+1, by2+1)

		left = bx2
		bx2 = left + len(h.hkeyName[i].Text)
		h.hkeyName[i].SetRect(left-1, by1-1, bx2+1, by2+1)
		left = bx2
	}
}

func (h *HotKeyPanel) AddHotKey(key, name string) {
	if len(key) > 0 {
		h.visibles = append(h.visibles, len(h.hkeyName))
	}

	bkey := widgets.NewParagraph()
	bkey.Text = " " + key + " "
	bkey.Border = false
	bkey.TextStyle = theme.Theme["hotKey"].Active
	h.hkey = append(h.hkey, bkey)

	bname := widgets.NewParagraph()
	bname.Text = name + " "
	bname.Border = false
	h.hkeyName = append(h.hkeyName, bname)
}

func NewHotKeyWidget() *HotKeyPanel {
	return &HotKeyPanel{}
}

type ActionList struct {
	dropDownPanel *widgets.Table
	hotKeyPanel   *HotKeyPanel

	Actions     []Action
	selectedRow int

	mux sync.Mutex

	itemStyle         ui.Style
	selectedItemStyle ui.Style
	checkedItemStyle  ui.Style
	hotKeyStyle       ui.Style
	hotKeyNameStyle   ui.Style

	dropDownVisible    bool
	bottomPanelVisible bool
}

func NewActionList(bottomVisible bool) *ActionList {
	actions := &ActionList{
		dropDownPanel:      widgets.NewTable(),
		selectedRow:        -1,
		dropDownVisible:    false,
		bottomPanelVisible: bottomVisible,

		itemStyle:         theme.Theme["listItem"].Active,
		selectedItemStyle: theme.Theme["listItemSelected"].Active,
		checkedItemStyle:  theme.Theme["checked"].Active,
		hotKeyStyle:       theme.Theme["hotKey"].Active,
		hotKeyNameStyle:   theme.Theme["hotKeyName"].Active,
	}
	actions.dropDownPanel.RowSeparator = false
	actions.dropDownPanel.FillRow = true
	actions.dropDownPanel.TextAlignment = ui.AlignLeft
	actions.dropDownPanel.Rows = make([][]string, 0, 5)

	if bottomVisible {
		actions.hotKeyPanel = NewHotKeyWidget()
	}

	return actions
}

func (a *ActionList) AddAction(name, hotKey string, checkable bool, onExec func([]string) bool) {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.Actions = append(a.Actions, Action{Name: name, HotKey: hotKey, Checkable: checkable, OnExecute: onExec})
	a.dropDownPanel.Rows = append(a.dropDownPanel.Rows, []string{name, hotKey})

	if a.hotKeyPanel != nil {
		a.hotKeyPanel.AddHotKey(hotKey, name)
	}
}

func (a *ActionList) setDwopDownRect(x, y int) {
	_, termHeight := ui.TerminalDimensions()
	y2 := y + len(a.dropDownPanel.Rows) + 2
	// TODO: calc width
	x2 := x + 30

	if y2 >= termHeight {
		y = termHeight - len(a.dropDownPanel.Rows) - 2
		y2 = termHeight
	}

	a.dropDownPanel.SetRect(x, y, x2, y2)
}

func (a *ActionList) SetHotKeyPanelRect(r image.Rectangle) {
	if a.hotKeyPanel != nil {
		a.hotKeyPanel.adjustWidget(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
	}
}

func (a *ActionList) Draw(buf *ui.Buffer) {
	if a.bottomPanelVisible {
		for i := range a.hotKeyPanel.hkeyName {
			if a.Actions[i].Checked {
				a.hotKeyPanel.hkeyName[i].TextStyle = a.checkedItemStyle
			} else {
				a.hotKeyPanel.hkeyName[i].TextStyle = a.hotKeyNameStyle
			}
			a.hotKeyPanel.hkey[i].Draw(buf)
			a.hotKeyPanel.hkeyName[i].Draw(buf)
		}
	}

	if a.dropDownVisible {
		for i := range a.dropDownPanel.Rows {
			if i == a.selectedRow {
				a.dropDownPanel.RowStyles[i] = a.selectedItemStyle
			} else if a.Actions[i].Checked {
				a.dropDownPanel.RowStyles[i] = a.checkedItemStyle
			} else {
				a.dropDownPanel.RowStyles[i] = a.itemStyle
			}
		}
		a.dropDownPanel.Draw(buf)
	}
}

func (a *ActionList) ShowDropDown(x, y int) {
	a.mux.Lock()
	defer a.mux.Unlock()
	if len(a.Actions) == 0 {
		return
	}
	a.dropDownVisible = true
	a.selectedRow = 0
	a.setDwopDownRect(x, y)
}

func (a *ActionList) HideDropDown() {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.dropDownVisible = false
}

func (a *ActionList) onExecute(context []string) {
	if a.selectedRow < 0 || len(context) == 0 {
		return
	}

	if a.Actions[a.selectedRow].Checkable {
		a.Actions[a.selectedRow].Checked = !a.Actions[a.selectedRow].Checked
	}

	if a.Actions[a.selectedRow].OnExecute != nil {
		a.Actions[a.selectedRow].OnExecute(context)
	}
}

func (a *ActionList) OnEvent(event *termui.Event, context []string) bool {
	switch event.ID {
	case "<Down>", "<MouseWheelDown>":
		a.Down()
		return true
	case "<Up>", "<MouseWheelUp>":
		a.Up()
		return true
	case "<MouseRelease>":
		m := event.Payload.(ui.Mouse)
		if a.locateAndFocus(m.X, m.Y) {
			return true
		}
		return false
	case "<MouseLeft>":
		a.HideDropDown()
		m := event.Payload.(ui.Mouse)
		if a.locateAndFocus(m.X, m.Y) {
			a.onExecute(context)
		}
		return true
	case "<Enter>":
		a.HideDropDown()
		a.onExecute(context)
		return true
	}

	return false
}

func (a *ActionList) OnHotKeys(event *termui.Event, context []string) bool {
	a.mux.Lock()
	defer a.mux.Unlock()
	for i := range a.Actions {
		if event.ID == a.Actions[i].HotKey {
			a.selectedRow = i
			a.onExecute(context)
			return true
		}
	}

	return false
}

func (a *ActionList) IsDropDownVisible() bool {
	a.mux.Lock()
	defer a.mux.Unlock()
	return a.dropDownVisible
}

func (a *ActionList) IsBottomPanelVisible() bool {
	a.mux.Lock()
	defer a.mux.Unlock()
	return a.bottomPanelVisible
}

func (a *ActionList) Up() {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.scroll(-1)
}

func (a *ActionList) Down() {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.scroll(1)
}

func (a *ActionList) scroll(amount int) {
	sel := a.selectedRow + amount
	a.setCursor(sel)
}

func (a *ActionList) setCursor(rowIdx int) {
	if rowIdx >= 0 && rowIdx < len(a.dropDownPanel.Rows) {
		a.selectedRow = rowIdx
	}
}

func (a *ActionList) locateAndFocus(x, y int) bool {
	rect := image.Rect(x, y, x+1, y+1)
	if rect.In(a.dropDownPanel.Bounds()) {
		a.setCursor(y - a.dropDownPanel.Min.Y - 1)
		return true
	}

	return false
}
