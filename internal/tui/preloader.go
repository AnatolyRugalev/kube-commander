package tui

import (
	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type Preloader struct {
	*widgets.Paragraph
}

func (Preloader) OnEvent(event *termui.Event) bool {
	return false
}

func (Preloader) OnFocusIn() {

}

func (Preloader) OnFocusOut() {

}

func NewPreloader() *Preloader {
	p := widgets.NewParagraph()
	p.Text = "Loading..."
	return &Preloader{
		Paragraph: p,
	}
}
