package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type PopupWidget struct {
	focus.FocusableWidget
	layout *ScreenLayout
}

func (p *PopupWidget) OnBlur() {
	p.FocusableWidget.OnBlur()
	p.layout.ClearPopup()
}

type ScreenLayout struct {
	*views.BoxLayout
	focus *focus.Manager
	view  views.View

	popup   focus.FocusableWidget
	popupWr float64
	popupHr float64
}

type DisplayableWidget interface {
	OnDisplay()
}

func (s *ScreenLayout) ClearPopup() {
	s.popup = nil
}

func (s *ScreenLayout) OnPopupEvent(widget focus.FocusableWidget, wr float64, hr float64) focus.FocusableWidget {
	s.popup = &PopupWidget{
		FocusableWidget: widget,
		layout:          s,
	}
	s.popupWr, s.popupHr = wr, hr
	if w, ok := widget.(DisplayableWidget); ok {
		w.OnDisplay()
	}
	return s.popup
}

func (s *ScreenLayout) Draw() {
	s.BoxLayout.Draw()
	if s.popup != nil {
		s.popup.SetView(s.newPopupView(s.popupWr, s.popupHr))
		s.popup.Draw()
	}
}

func (s *ScreenLayout) newPopupView(wr, hr float64) *views.ViewPort {
	w, h := s.view.Size()

	pw := float64(w) * wr
	ph := float64(h) * hr

	x := float64(w)/2 - pw/2
	y := float64(h)/2 - ph/2

	return views.NewViewPort(s.view, int(x), int(y), int(pw), int(ph))
}

func (s *ScreenLayout) SwitchWorkspace(widget views.Widget) {
	widgets := s.Widgets()
	if len(widgets) == 2 {
		s.RemoveWidget(widgets[len(widgets)-1])
	}
	s.AddWidget(widget, 0.9)
	if w, ok := widget.(DisplayableWidget); ok {
		w.OnDisplay()
	}
}

func (s *ScreenLayout) HandleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEsc:
			s.focus.Blur()
			return true
		}
	}
	if s.popup != nil {
		return s.popup.HandleEvent(ev)
	} else {
		return s.focus.Current().HandleEvent(ev)
	}
}

func (s *ScreenLayout) SetView(view views.View) {
	s.view = view
	s.BoxLayout.SetView(view)
}

func NewScreenLayout(root focus.FocusableWidget, fill float64) *ScreenLayout {
	box := views.NewBoxLayout(views.Horizontal)
	box.AddWidget(root, fill)
	layout := &ScreenLayout{
		BoxLayout: box,
	}
	layout.focus = focus.NewFocusManager(root, layout)
	layout.Watch(layout.focus)
	return layout
}
