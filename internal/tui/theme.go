package tui

import ui "github.com/gizak/termui/v3"

// Colors reference: http://www.lihaoyi.com/post/Ansi/RainbowBackground256.png

var theme = map[string]ui.Style{
	"default":            ui.NewStyle(ui.ColorClear, ui.ColorClear),
	"title":              ui.NewStyle(ui.ColorBlack, ui.ColorClear),
	"header":             ui.NewStyle(ui.ColorBlack, ui.ColorClear, ui.ModifierBold),
	"selectedInFocus":    ui.NewStyle(ui.ColorBlack, ui.ColorCyan),
	"selectedOutOfFocus": ui.NewStyle(ui.ColorBlack, ui.ColorWhite),
	"focus":              ui.NewStyle(ui.ColorCyan, ui.ColorClear),
}
