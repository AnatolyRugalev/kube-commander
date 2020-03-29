package theme

import "github.com/gdamore/tcell"

var (
	Default = tcell.
		StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorBlack)
	ActiveFocused   = Default.Background(tcell.ColorLightCyan)
	ActiveUnfocused = Default.Background(tcell.ColorDarkGray)

	Menu = tcell.
		StyleDefault.
		Background(tcell.ColorLightCoral).
		Foreground(tcell.ColorBlack)
)
