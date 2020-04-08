package theme

import "github.com/gdamore/tcell"

var (
	Default = tcell.
		StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorBlack)

	ColorSelectedFocusedBackground   = tcell.ColorLightCyan
	ColorSelectedUnfocusedBackground = tcell.ColorDarkGray
	ColorDisabledForeground          = tcell.ColorGray
)
