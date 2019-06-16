package tui

import (
	"image"
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/theme"

	"github.com/gizak/termui/v3"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// Buttons text
const (
	ButtonOk     = "OK"
	ButtonCancel = "Cancel"
	ButtonYes    = "Yes"
	ButtonNo     = "No"
)

const (
	buttonWidth       = 10
	buttonHeight      = 4
	buttonMarginRight = 3
)

type btnFunc func() error

// Button represents text button
type Button struct {
	*widgets.Paragraph
	onClick btnFunc
}

// NewButton returns button with specified text
func NewButton(text string, onClick btnFunc) *Button {
	b := &Button{
		Paragraph: widgets.NewParagraph(),
	}
	b.Text = text
	b.onClick = onClick
	return b
}

func (b *Button) setRect(x1, y1, x2, y2 int) {
	b.Paragraph.SetRect(x1, y1, x2, y2)
}

// Dialog represents modal dialog window
type Dialog struct {
	*widgets.Paragraph
	Buttons             []*Button
	buttonStyle         ui.Style
	selectedButtonStyle ui.Style
	selectedButton      int
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
		dlg.Buttons[i].setRect(bx1, by1, bx2, by2)
		left += buttonWidth + buttonMarginRight
	}
}

func (dlg *Dialog) setRect() {
	termWidth, termHeight := ui.TerminalDimensions()
	lines := strings.Split(dlg.Paragraph.Text, "\n")
	lineWidth := maxLinesWidth(lines) + 2
	if len(dlg.Paragraph.Title) > lineWidth {
		lineWidth = len(dlg.Paragraph.Title) + 5
	}
	if lineWidth > termWidth {
		lineWidth = termWidth - 10
	}

	x1 := termWidth/2 - lineWidth/2 - 1
	y1 := termHeight/2 - 5
	x2 := x1 + lineWidth + dlg.Paragraph.PaddingLeft + dlg.Paragraph.PaddingRight
	y2 := y1 + len(lines) + buttonHeight + 2

	dlg.Paragraph.SetRect(x1, y1, x2, y2)
	dlg.setButtonsRect(x1, y1, x2, y2)
}

func newDialog(title, text string, buttons ...*Button) *Dialog {
	p := widgets.NewParagraph()
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

	newDlg.setRect()

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
			ShowErrorDialog(err, nil)
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

func ShowConfirmDialog(text string, onOk btnFunc) {
	dlg := newDialog("Are you sure?", text, NewButton(ButtonCancel, nil), NewButton(ButtonOk, onOk))
	screen.Focus(dlg)
	screen.setPopup(dlg)
}

func ShowErrorDialog(err error, onClick btnFunc) {
	dlg := newDialog("Error", err.Error(), NewButton(ButtonOk, onClick))
	screen.Focus(dlg)
	screen.setPopup(dlg)
}

func ShowMessageDialog(text string) {
	dlg := newDialog("Message", text)
	screen.Focus(dlg)
	screen.setPopup(dlg)
}
