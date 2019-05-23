package tui

import ui "github.com/gizak/termui/v3"

// Colors reference: http://www.lihaoyi.com/post/Ansi/RainbowBackground256.png

type themeMap struct {
	active   ui.Style
	inactive ui.Style
}

var theme = map[string]themeMap{
	"grid": {
		inactive: ui.NewStyle(ui.ColorClear, ui.ColorClear),
		active:   ui.NewStyle(ui.ColorBlack, ui.ColorClear),
	},
	"title": {
		inactive: ui.NewStyle(ui.ColorClear, ui.ColorClear),
		active:   ui.NewStyle(ui.ColorBlack, ui.ColorClear),
	},
	"listItem": {
		inactive: ui.NewStyle(ui.ColorClear, ui.ColorClear),
		active:   ui.NewStyle(ui.ColorBlack, ui.ColorClear),
	},
	"listItemSelected": {
		inactive: ui.NewStyle(ui.ColorClear, ui.Color(240)),
		active:   ui.NewStyle(ui.ColorBlack, ui.ColorCyan),
	},
	"listHeader": {
		inactive: ui.NewStyle(ui.Color(240), ui.ColorClear, ui.ModifierBold),
		active:   ui.NewStyle(ui.ColorBlack, ui.ColorClear, ui.ModifierBold),
	},
}
