package popup

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell/views"
)

type popup struct {
	commander.MaxSizeWidget

	onBlur func()
}

func (p *popup) OnBlur() {
	p.MaxSizeWidget.OnBlur()
	p.onBlur()
}

func (p *popup) Reposition(view commander.View) {
	viewWidth, viewHeight := view.Size()
	maxW, maxH := p.MaxSizeWidget.MaxSize()
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
	p.MaxSizeWidget.SetView(popupView)
}

func NewPopup(view commander.View, widget commander.MaxSizeWidget, onBlur func()) *popup {
	popup := popup{
		MaxSizeWidget: widget,
		onBlur:        onBlur,
	}
	popup.Reposition(view)
	return &popup
}
