package theme

import "github.com/gdamore/tcell"

var (
	Default = tcell.
		StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorBlack)

	ColorActiveFocusedBackground   = tcell.ColorLightCyan
	ColorActiveUnfocusedBackground = tcell.ColorDarkGray
	ColorDisabledForeground        = tcell.ColorGray
)
