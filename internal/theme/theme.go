package theme

// Colors reference: http://www.lihaoyi.com/post/Ansi/RainbowBackground256.png

import "github.com/gizak/termui/v3"

type themeMap struct {
	Active   termui.Style
	Inactive termui.Style
}

var Theme = map[string]themeMap{
	"grid": {
		Inactive: termui.NewStyle(termui.Color(249), termui.ColorClear),
		Active:   termui.NewStyle(termui.Color(231), termui.ColorClear),
	},
	"title": {
		Inactive: termui.NewStyle(termui.Color(249), termui.ColorClear),
		Active:   termui.NewStyle(termui.Color(231), termui.ColorClear),
	},
	"listItem": {
		Inactive: termui.NewStyle(termui.Color(249), termui.ColorClear),
		Active:   termui.NewStyle(termui.Color(231), termui.ColorClear),
	},
	"listItemSelected": {
		Inactive: termui.NewStyle(termui.Color(249), termui.Color(240)),
		Active:   termui.NewStyle(termui.Color(237), termui.Color(51)),
	},
	"listHeader": {
		Inactive: termui.NewStyle(termui.Color(249), termui.ColorClear, termui.ModifierBold),
		Active:   termui.NewStyle(termui.Color(231), termui.ColorClear, termui.ModifierBold),
	},
	"dialog": {
		Inactive: termui.NewStyle(termui.Color(249), termui.ColorClear),
		Active:   termui.NewStyle(termui.Color(231), termui.ColorClear),
	},
	"button": {
		Inactive: termui.NewStyle(termui.Color(237), termui.ColorClear),
		Active:   termui.NewStyle(termui.Color(51), termui.ColorClear, termui.ModifierBold),
	},
	"checked": {
		Inactive: termui.NewStyle(termui.Color(249), termui.Color(43)),
		Active:   termui.NewStyle(termui.Color(237), termui.Color(43)),
	},
	"hotKey": {
		Inactive: termui.NewStyle(termui.Color(254), termui.Color(239)),
		Active:   termui.NewStyle(termui.Color(254), termui.Color(239)),
	},
	"hotKeyName": {
		Inactive: termui.NewStyle(termui.Color(237), termui.Color(39)),
		Active:   termui.NewStyle(termui.Color(237), termui.Color(39)),
	},
}

var PreloaderColors = []termui.Color{
	termui.Color(18),
	termui.Color(19),
	termui.Color(20),
	termui.Color(21),
}
var PreloaderIdleColor = termui.Color(39)
