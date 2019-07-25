package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/mitchellh/go-wordwrap"
	"image"
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/theme"

	"github.com/gizak/termui/v3"
	ui "github.com/gizak/termui/v3"
	uiwidgets "github.com/gizak/termui/v3/widgets"
)

// Buttons text
const (
	ButtonOk    = "OK"
	ButtonRetry = "Retry"
	ButtonExit  = "Exit"
	ButtonYes   = "Yes"
	ButtonNo    = "No"
)

const (
	buttonWidth       = 10
	buttonHeight      = 4
	buttonMarginRight = 3
)

type btnFunc func() error

// Button represents text button
type Button struct {
	*uiwidgets.Paragraph
	onClick btnFunc
}

// NewButton returns button with specified text
func NewButton(text string, onClick btnFunc) *Button {
	b := &Button{
		Paragraph: uiwidgets.NewParagraph(),
	}
	b.Text = text
	b.onClick = onClick
	return b
}

// Dialog represents modal dialog window
type Dialog struct {
	*uiwidgets.Paragraph
	Buttons             []*Button
	buttonStyle         ui.Style
	selectedButtonStyle ui.Style
	selectedButton      int
}

type Resizable interface {
	Resize(screenRect image.Rectangle)
}

// Draw draws dialog and buttons
func (dlg *Dialog) Draw(buf *ui.Buffer) {
	dlg.Paragraph.Draw(buf)
	for idx, btn := range dlg.Buttons {
		if idx == dlg.selectedButton {
			btn.Paragraph.BorderStyle = dlg.selectedButtonStyle
		} else {
			btn.Paragraph.BorderStyle = dlg.buttonStyle
		}
		btn.Paragraph.Draw(buf)
	}
}

func (dlg *Dialog) addButton(b *Button) {
	dlg.Buttons = append(dlg.Buttons, b)
}

func (dlg *Dialog) setButtonsRect(x1, y1, x2, y2 int) {
	var bx1, by1, bx2, by2, left, buttonsWidth int
	buttonsWidth = (buttonWidth + buttonMarginRight) * len(dlg.Buttons)
	left = x1 + (x2-x1)/2 - buttonsWidth/2 - 1

	for i := 0; i < len(dlg.Buttons); i++ {
		bx1 = left
		by1 = y2 - buttonHeight
		bx2 = bx1 + buttonWidth
		by2 = y2 - 1
		dlg.Buttons[i].SetRect(bx1, by1, bx2, by2)
		left += buttonWidth + buttonMarginRight
	}
}

func (dlg *Dialog) Resize(screenRect image.Rectangle) {
	// Wrap text inside dialog to fit into 3/4 of the screen
	wrapTextSize := screenRect.Max.X * 3 / 4
	text := wordwrap.WrapString(dlg.Paragraph.Text, uint(wrapTextSize))
	lines := strings.Split(text, "\n")

	textWidth := maxLineWidth(lines) + 2
	minWidth := len(dlg.Paragraph.Title) + 5

	if textWidth < minWidth {
		textWidth = minWidth
	}

	x1 := screenRect.Max.X/2 - textWidth/2 - 1
	y1 := screenRect.Max.Y/2 - 5
	x2 := x1 + textWidth
	y2 := y1 + len(lines) + buttonHeight + 2

	dlg.SetRect(x1, y1, x2, y2)
	dlg.setButtonsRect(x1, y1, x2, y2)
}

func newDialog(title, text string, buttons ...*Button) *Dialog {
	p := uiwidgets.NewParagraph()
	p.Title = title
	p.Text = text
	p.BorderStyle = theme.Theme["dialog"].Active
	p.TitleStyle = theme.Theme["dialog"].Active
	p.TextStyle = theme.Theme["dialog"].Active
	p.PaddingLeft = 1
	p.PaddingRight = 1

	newDlg := &Dialog{Paragraph: p}
	newDlg.buttonStyle = theme.Theme["button"].Inactive
	newDlg.selectedButtonStyle = theme.Theme["button"].Active

	if len(buttons) == 0 {
		newDlg.addButton(NewButton(ButtonOk, nil))
	} else {
		for _, button := range buttons {
			newDlg.addButton(button)
		}
	}

	return newDlg
}

func (dlg Dialog) currentButton() *Button {
	return dlg.Buttons[dlg.selectedButton]
}

func (dlg *Dialog) nextButton() {
	if dlg.selectedButton < len(dlg.Buttons)-1 {
		dlg.selectedButton++
	}
}

func (dlg *Dialog) prevButton() {
	if dlg.selectedButton > 0 {
		dlg.selectedButton--
	}
}

func (dlg *Dialog) onResult() {
	btn := dlg.currentButton()
	screen.popFocus()
	screen.removePopup()
	if btn.onClick != nil {
		err := btn.onClick()
		if err != nil {
			screen.ShowDialog(NewErrorDialog(err, nil))
		} else {
			screen.reloadCurrentRightPane()
		}
	}
}

func (dlg *Dialog) OnEvent(event *termui.Event) bool {
	switch event.ID {
	case "<Right>":
		dlg.nextButton()
		return true
	case "<Left>":
		dlg.prevButton()
		return true
	case "<Enter>":
		dlg.onResult()
		return true
	case "<Escape>":
		screen.removePopup()
		screen.popFocus()
		return true
	case "<MouseLeft>":
		m := event.Payload.(ui.Mouse)
		if dlg.locateAndFocus(m.X, m.Y) {
			dlg.onResult()
			return true
		}
		return false
	}

	return false
}

func (dlg *Dialog) OnFocusIn() {
}

func (Dialog) OnFocusOut() {
}

func (dlg *Dialog) locateAndFocus(x, y int) bool {
	rect := image.Rect(x, y, x+1, y+1)
	for i, btn := range dlg.Buttons {
		if rect.In(btn.Bounds()) {
			dlg.selectedButton = i
			return true
		}
	}
	return false
}

func NewConfirmDialog(text string, onYes btnFunc) *Dialog {
	return newDialog("Are you sure?", text, NewButton(ButtonNo, nil), NewButton(ButtonYes, onYes))
}

func NewErrorDialog(err error, onClick btnFunc) *Dialog {
	return newDialog("Error", err.Error(), NewButton(ButtonOk, onClick))
}

func NewLoadingErrorDialog(err error, onRetry btnFunc, onExit btnFunc) *Dialog {
	return newDialog("Error", err.Error(), NewButton(ButtonRetry, onRetry), NewButton(ButtonExit, onExit))
}

func NewListTableDialog(title string, rows []widgets.ListRow, handler widgets.ListTableHandler) *widgets.ListTable {
	lt := widgets.NewListTable(rows, handler, nil)
	lt.Title = title
	lt.IsContext = true
	width := 30
	for _, row := range rows {
		rowWidth := 0
		for _, col := range row {
			rowWidth += len(col) + 2
		}
		if rowWidth > width {
			width = rowWidth
		}
	}
	height := len(rows) + 2
	lt.SetRect(0, 0, width, height)
	return lt
}
