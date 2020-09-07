package themes

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
)

var DefaultThemes []*pb.Theme

func RegisterTheme(t *pb.Theme) {
	DefaultThemes = append(DefaultThemes, t)
}

func RGBColor(name string, color string) *pb.Color {
	return &pb.Color{
		Name: name,
		Value: &pb.Color_Rgb{
			Rgb: color,
		},
	}
}

func XTermColor(name string, color int32) *pb.Color {
	return &pb.Color{
		Name: name,
		Value: &pb.Color_Xterm{
			Xterm: color,
		},
	}
}

func Attributes(attrs ...pb.StyleAttribute) []pb.StyleAttribute {
	return append([]pb.StyleAttribute{}, attrs...)
}

func BaseStyles() []*pb.Style {
	return []*pb.Style{
		{
			Name: "screen",
			Fg:   "fg",
			Bg:   "bg",
		},
		{
			Name: "title-bar",
			Fg:   "title-fg",
			Bg:   "title-bg",
		},
		{
			Name:  "logo-icon",
			Fg:    "title-fg",
			Bg:    "title-bg",
			Attrs: Attributes(pb.StyleAttribute_REVERSE),
		},
		{
			Name: "logo-text",
			Fg:   "title-fg",
			Bg:   "title-bg",
		},
		{
			Name: "popup",
			Bg:   "bg",
		},
		{
			Name: "loader",
			Fg:   "loader-fg",
			Bg:   "bg",
		},
		{
			Name:  "popup-title",
			Fg:    "fg",
			Bg:    "bg",
			Attrs: Attributes(pb.StyleAttribute_UNDERLINE),
		},
		{
			Name: "row",
			Fg:   "fg",
			Bg:   "bg",
		},
		{
			Name:  "row-header",
			Fg:    "fg",
			Bg:    "bg",
			Attrs: Attributes(pb.StyleAttribute_UNDERLINE),
		},
		{
			Name: "row-selected-focused",
			Fg:   "selection-fg",
			Bg:   "selection-bg",
		},
		{
			Name: "row-selected-unfocused",
			Fg:   "unfocused-fg",
			Bg:   "unfocused-bg",
		},
		{
			Name: "row-disabled",
			Fg:   "disabled-fg",
			Bg:   "disabled-bg",
		},
		{
			Name: "filter-active",
			Fg:   "selection-fg",
			Bg:   "selection-bg",
		},
		{
			Name: "filter-inactive",
			Fg:   "unfocused-fg",
			Bg:   "unfocused-bg",
		},
		{
			Name: "status-bar",
			Bg:   "status-bar",
		},
		{
			Name: "status-error",
			Fg:   "error-fg",
			Bg:   "error-bg",
		},
		{
			Name: "status-warning",
			Fg:   "warning-fg",
			Bg:   "warning-bg",
		},
		{
			Name: "status-info",
			Fg:   "info-fg",
			Bg:   "info-bg",
		},
		{
			Name: "status-confirm",
			Fg:   "confirm-fg",
			Bg:   "confirm-bg",
		},
	}
}
