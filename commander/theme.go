package commander

import "github.com/gdamore/tcell"

type Stylable interface {
	GetComponents() []StyleComponent
}

type StyleComponent interface {
	Name() string
	Style() Style
	SetStyle(style Style)
}

type ThemeManager interface {
	tcell.EventHandler
	Init() error
	DeInit()

	NextComponent()
	PrevComponent()

	NextBg()
	PrevBg()

	NextFg()
	PrevFg()

	SwitchAttr(attr tcell.AttrMask)
}
