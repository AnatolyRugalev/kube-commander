package border

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell/views"
)

const (
	vertical          = '│'
	horizontal        = '─'
	cornerTopLeft     = '┌'
	cornerTopRight    = '┐'
	cornerBottomLeft  = '└'
	cornerBottomRight = '┘'
	titleLeft         = '┤'
	titleRight        = '├'
)

type Borders uint8

const (
	Top Borders = 1 << iota
	Bottom
	Left
	Right
	All = Top | Right | Bottom | Left
)

type BorderedWidget struct {
	commander.MaxSizeWidget
	title      string
	view       views.View
	theme      commander.ThemeManager
	style      string
	titleStyle string
	borders    Borders
}

func (b Borders) Has(flag Borders) bool {
	return b&flag == flag
}

func NewBorderedWidget(widget commander.MaxSizeWidget, title string, theme commander.ThemeManager, style string, titleStyle string, borders Borders) *BorderedWidget {
	if borders == 0 {
		borders = All
	}
	return &BorderedWidget{
		MaxSizeWidget: widget,
		title:         title,
		theme:         theme,
		style:         style,
		titleStyle:    titleStyle,
		borders:       borders,
	}
}

func (b *BorderedWidget) Draw() {
	w, h := b.view.Size()

	x0 := 0
	if b.borders.Has(Left) {
		x0++
	}
	y0 := 0
	if b.borders.Has(Top) {
		y0++
	}

	style := b.theme.GetStyle(b.style)
	titleStyle := b.theme.GetStyle(b.titleStyle)
	for y := 1; y < h-1; y++ {
		if b.borders.Has(Left) {
			b.view.SetContent(0, y, vertical, nil, style)
		}
		if b.borders.Has(Right) {
			b.view.SetContent(w-1, y, vertical, nil, style)
		}
	}

	for x := 1; x < w-1; x++ {
		if b.borders.Has(Top) {
			b.view.SetContent(x, 0, horizontal, nil, style)
		}
		if b.borders.Has(Bottom) {
			b.view.SetContent(x, h-1, horizontal, nil, style)
		}
	}
	if b.title != "" && b.borders.Has(Top) {
		b.view.SetContent(x0, 0, titleLeft, nil, style)
		for i, r := range b.title {
			b.view.SetContent(i+x0+1, 0, r, nil, titleStyle)
		}
		b.view.SetContent(x0+len(b.title)+1, 0, titleRight, nil, style)
	}

	if b.borders.Has(Top | Left) {
		b.view.SetContent(0, 0, cornerTopLeft, nil, style)
	}
	if b.borders.Has(Top | Right) {
		b.view.SetContent(w-1, 0, cornerTopRight, nil, style)
	}
	if b.borders.Has(Bottom | Left) {
		b.view.SetContent(0, h-1, cornerBottomLeft, nil, style)
	}
	if b.borders.Has(Bottom | Right) {
		b.view.SetContent(w-1, h-1, cornerBottomRight, nil, style)
	}
	b.MaxSizeWidget.Draw()
}

func (b *BorderedWidget) offsets() (int, int) {
	offsetH := 0
	offsetW := 0
	if b.borders.Has(Top) {
		offsetH++
	}
	if b.borders.Has(Bottom) {
		offsetH++
	}
	if b.borders.Has(Left) {
		offsetW++
	}
	if b.borders.Has(Right) {
		offsetW++
	}
	return offsetW, offsetH
}

func (b *BorderedWidget) SetView(view views.View) {
	b.view = view
	w, h := view.Size()
	x, y := 0, 0
	// Reduce internal view size
	if b.borders.Has(Top) {
		y++
		h--
	}
	if b.borders.Has(Bottom) {
		h--
	}
	if b.borders.Has(Left) {
		x++
		w--
	}
	if b.borders.Has(Right) {
		w--
	}
	viewport := views.NewViewPort(view, x, y, w, h)
	b.MaxSizeWidget.SetView(viewport)
}

func (b *BorderedWidget) MaxSize() (int, int) {
	w, h := b.MaxSizeWidget.MaxSize()
	offsetW, offsetH := b.offsets()
	if b.title != "" && len(b.title)+2 > w {
		w = len(b.title) + 2
	}
	return w + offsetW, h + offsetH
}
