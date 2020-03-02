package help

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell/views"
)

type widget struct {
	*views.Text
	*focus.Focusable
}

func (w widget) MaxSize() (int, int) {
	return w.Text.Size()
}

var text = `kube-commander - browse your Kubernetes cluster in a casual way!

Global:                           Resources:
 ?:            Shows help dialog   D: Describe selected resource
 Q:            Quit                E: Edit selected resource
 Ctrl+N or F2: Change namespace

Pods:
 L: Show logs
 F: Forward port
 X: eXec /bin/sh or /bin/bash inside container
`

func NewHelpWidget() *widget {
	widget := widget{
		Text:      views.NewText(),
		Focusable: focus.NewFocusable(),
	}
	widget.Text.SetText(text)
	return &widget
}

func ShowHelpPopup(workspace commander.Workspace) {
	help := NewHelpWidget()
	workspace.ShowPopup(help)
}
