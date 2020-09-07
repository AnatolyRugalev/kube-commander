package themes

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
	"github.com/gdamore/tcell"
)

// Base16 uses base 16 colors of the terminal
var Base16 = &pb.Theme{
	Name: "base16",
	Colors: []*pb.Color{
		XTermColor("bg", int32(tcell.ColorDefault)),
		XTermColor("fg", int32(tcell.ColorWhite)),
		XTermColor("title-bg", int32(tcell.ColorBlue)),
		XTermColor("title-fg", int32(tcell.ColorWhite)),
		XTermColor("loader-fg", int32(tcell.ColorBlue)),
		XTermColor("selection-fg", int32(tcell.ColorBlack)),
		XTermColor("selection-bg", int32(tcell.ColorWhite)),
		XTermColor("unfocused-fg", int32(tcell.ColorWhite)),
		XTermColor("unfocused-bg", int32(tcell.ColorGray)),
		XTermColor("disabled-fg", int32(tcell.ColorGray)),
		XTermColor("disabled-bg", int32(tcell.ColorDefault)),
		XTermColor("status-bar", int32(tcell.ColorGray)),
		XTermColor("error-fg", int32(tcell.ColorWhite)),
		XTermColor("error-bg", int32(tcell.ColorRed)),
		XTermColor("warning-fg", int32(tcell.ColorBlack)),
		XTermColor("warning-bg", int32(tcell.ColorYellow)),
		XTermColor("info-fg", int32(tcell.ColorBlack)),
		XTermColor("info-bg", int32(tcell.ColorYellow)),
		XTermColor("confirm-fg", int32(tcell.ColorBlack)),
		XTermColor("confirm-bg", int32(tcell.ColorYellow)),
	},
	Styles: BaseStyles(),
}

func init() {
	RegisterTheme(Base16)
}
