package themes

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
)

var Twilight = &pb.Theme{
	Name: "twilight",
	Colors: []*pb.Color{
		RGBColor("bg", "1e1e1e"),
		RGBColor("fg", "a7a7a7"),
		RGBColor("title-bg", "7587a6"),
		RGBColor("title-fg", "ffffff"),
		RGBColor("loader-fg", "7587a6"),
		RGBColor("selection-fg", "1e1e1e"),
		RGBColor("selection-bg", "f9ee98"),
		RGBColor("unfocused-fg", "5f5a60"),
		RGBColor("unfocused-bg", "8f9d6a"),
		RGBColor("disabled-fg", "5f5a60"),
		RGBColor("disabled-bg", "1e1e1e"),
		RGBColor("status-bar", "5f5a60"),
		RGBColor("error-fg", "1e1e1e"),
		RGBColor("error-bg", "cf6a4c"),
		RGBColor("warning-fg", "1e1e1e"),
		RGBColor("warning-bg", "9b859d"),
		RGBColor("info-fg", "1e1e1e"),
		RGBColor("info-bg", "9b859d"),
		RGBColor("confirm-fg", "1e1e1e"),
		RGBColor("confirm-bg", "9b859d"),
	},
	Styles: BaseStyles(),
}

func init() {
	RegisterTheme(Twilight)
}
