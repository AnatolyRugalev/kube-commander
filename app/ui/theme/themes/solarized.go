package themes

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
)

var Solarized = &pb.Theme{
	Name: "solarized",
	Colors: []*pb.Color{
		RGBColor("bg", "002b36"),
		RGBColor("fg", "93a1a1"),
		RGBColor("title-bg", "2aa198"),
		RGBColor("title-fg", "fdf6e3"),
		RGBColor("loader-fg", "2aa198"),
		RGBColor("selection-fg", "002b36"),
		RGBColor("selection-bg", "fdf6e3"),
		RGBColor("unfocused-fg", "002b36"),
		RGBColor("unfocused-bg", "93a1a1"),
		RGBColor("disabled-fg", "657b83"),
		RGBColor("disabled-bg", "002b36"),
		RGBColor("status-bar", "657b83"),
		RGBColor("error-fg", "002b36"),
		RGBColor("error-bg", "dc322f"),
		RGBColor("warning-fg", "002b36"),
		RGBColor("warning-bg", "b58900"),
		RGBColor("info-fg", "002b36"),
		RGBColor("info-bg", "b58900"),
		RGBColor("confirm-fg", "002b36"),
		RGBColor("confirm-bg", "b58900"),
	},
	Styles: BaseStyles(),
}

func init() {
	RegisterTheme(Solarized)
}
