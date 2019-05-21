package tui

import (
	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type Welcome struct {
	*widgets.Paragraph
}

func (Welcome) OnEvent(event *termui.Event) bool {
	return false
}

func (Welcome) OnFocusIn() {
}

func (Welcome) OnFocusOut() {
}

func NewWelcomeScreen() *Welcome {
	p := widgets.NewParagraph()
	p.Text = "Welcome!"
	return &Welcome{
		Paragraph: p,
	}
}
