package themes

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
)

var Paraiso = &pb.Theme{
	Name: "paraiso",
	Colors: []*pb.Color{
		RGBColor("bg", "2f1e2e"),
		RGBColor("fg", "a39e9b"),
		RGBColor("title-bg", "5bc4bf"),
		RGBColor("title-fg", "2f1e2e"),
		RGBColor("loader-fg", "7587a6"),
		RGBColor("selection-fg", "2f1e2e"),
		RGBColor("selection-bg", "fec418"),
		RGBColor("unfocused-fg", "2f1e2e"),
		RGBColor("unfocused-bg", "815ba4"),
		RGBColor("disabled-fg", "776e71"),
		RGBColor("disabled-bg", "2f1e2e"),
		RGBColor("status-bar", "776e71"),
		RGBColor("error-fg", "2f1e2e"),
		RGBColor("error-bg", "ef6155"),
		RGBColor("warning-fg", "2f1e2e"),
		RGBColor("warning-bg", "fec418"),
		RGBColor("info-fg", "2f1e2e"),
		RGBColor("info-bg", "fec418"),
		RGBColor("confirm-fg", "2f1e2e"),
		RGBColor("confirm-bg", "fec418"),
	},
	Styles: BaseStyles(),
}

func init() {
	RegisterTheme(Paraiso)
}
