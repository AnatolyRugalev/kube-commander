package tui

import ui "github.com/gizak/termui/v3"

// Colors reference: http://www.lihaoyi.com/post/Ansi/RainbowBackground256.png

type themeMap struct {
	active   ui.Style
	inactive ui.Style
}

var theme = map[string]themeMap{
	"grid": {
		inactive: ui.NewStyle(ui.Color(249), ui.ColorClear),
		active:   ui.NewStyle(ui.Color(231), ui.ColorClear),
	},
	"title": {
		inactive: ui.NewStyle(ui.Color(249), ui.ColorClear),
		active:   ui.NewStyle(ui.Color(231), ui.ColorClear),
	},
	"listItem": {
		inactive: ui.NewStyle(ui.Color(249), ui.ColorClear),
		active:   ui.NewStyle(ui.Color(231), ui.ColorClear),
	},
	"listItemSelected": {
		inactive: ui.NewStyle(ui.Color(249), ui.Color(240)),
		active:   ui.NewStyle(ui.Color(237), ui.Color(51)),
	},
	"listHeader": {
		inactive: ui.NewStyle(ui.Color(249), ui.ColorClear, ui.ModifierBold),
		active:   ui.NewStyle(ui.Color(231), ui.ColorClear, ui.ModifierBold),
	},
	"dialog": {
		inactive: ui.NewStyle(ui.Color(249), ui.ColorClear),
		active:   ui.NewStyle(ui.Color(231), ui.ColorClear),
	},
	"button": {
		inactive: ui.NewStyle(ui.Color(237), ui.ColorClear),
		active:   ui.NewStyle(ui.Color(51), ui.ColorClear, ui.ModifierBold),
	},
}
