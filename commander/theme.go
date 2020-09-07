package commander

import "github.com/gdamore/tcell"

type Style = tcell.Style

type Color struct {
	Name  string
	Color tcell.Color
}

type ThemeComponent interface {
	Name() string
	Style(name string) Style
	SetStyle(name string, style Style)
}

type ThemeManager interface {
	Configurable
	GetStyle(name string) Style
	NextTheme()
	PrevTheme()
}
