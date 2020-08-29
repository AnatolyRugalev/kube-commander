package popup

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/border"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell/views"
)

type popup struct {
	*border.BorderedWidget

	onBlur func()
}

func (p *popup) OnBlur() {
	p.BorderedWidget.OnBlur()
	p.onBlur()
}

func (p *popup) Reposition(view commander.View) {
	viewWidth, viewHeight := view.Size()
	viewWidth -= 2
	viewHeight -= 2
	maxW, maxH := p.BorderedWidget.MaxSize()
	x := float64(viewWidth)/2 - float64(maxW)/2
	y := float64(viewHeight)/2 - float64(maxH)/2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if maxW > viewWidth {
		maxW = viewWidth
	}
	if maxH > viewHeight {
		maxH = viewHeight
	}

	popupView := views.NewViewPort(view, int(x), int(y), maxW, maxH)
	p.BorderedWidget.SetView(popupView)
}

func NewPopup(view commander.View, title string, widget commander.MaxSizeWidget, onBlur func()) *popup {
	popup := popup{
		BorderedWidget: border.NewBorderedWidget(widget, title, theme.Default, theme.Default.Underline(true), border.All),
		onBlur:         onBlur,
	}
	popup.Reposition(view)
	return &popup
}
