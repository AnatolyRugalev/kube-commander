package themes

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
)

var Monokai = &pb.Theme{
	Name: "monokai",
	Colors: []*pb.Color{
		RGBColor("bg", "272822"),
		RGBColor("fg", "f8f8f2"),
		RGBColor("title-bg", "66d9ef"),
		RGBColor("title-fg", "272822"),
		RGBColor("loader-fg", "66d9ef"),
		RGBColor("selection-fg", "272822"),
		RGBColor("selection-bg", "a6e22e"),
		RGBColor("unfocused-fg", "75715e"),
		RGBColor("unfocused-bg", "a1efe4"),
		RGBColor("disabled-fg", "75715e"),
		RGBColor("disabled-bg", "272822"),
		RGBColor("status-bar", "a1efe4"),
		RGBColor("error-fg", "f8f8f2"),
		RGBColor("error-bg", "fe1d6e"),
		RGBColor("warning-fg", "272822"),
		RGBColor("warning-bg", "f4bf75"),
		RGBColor("info-fg", "272822"),
		RGBColor("info-bg", "f4bf75"),
		RGBColor("confirm-fg", "272822"),
		RGBColor("confirm-bg", "f4bf75"),
	},
	Styles: BaseStyles(),
}

func init() {
	RegisterTheme(Monokai)
}
