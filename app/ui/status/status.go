package status

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

type Status struct {
	views.WidgetWatchers
	*focus.Focusable
	mu sync.Mutex

	view    views.View
	loaderV *views.ViewPort
	loader  *loader
	textV   *views.ViewPort
	text    *views.Text

	screen commander.ScreenHandler
	events chan *tcell.EventKey

	once    sync.Once
	clearMu sync.Mutex
	clearIn time.Time
}

func (s *Status) LoadingStarted() {
	s.loader.Start()
}

func (s *Status) LoadingFinished() {
	s.loader.Finish()
}

func (s *Status) Resize() {
	w, h := s.view.Size()
	s.loaderV.Resize(0, 0, 2, h)
	s.textV.Resize(2, 0, w-2, h)
}

func (s *Status) SetView(view views.View) {
	s.mu.Lock()
	s.view = view
	w, h := view.Size()
	s.loaderV = views.NewViewPort(view, 0, 0, 2, h)
	s.loader.SetView(s.loaderV)
	s.textV = views.NewViewPort(view, 2, 0, w-2, h)
	s.text.SetView(s.textV)
	s.mu.Unlock()
}

func (s *Status) watch() {
	ticker := time.NewTicker(time.Millisecond * 200)
	for {
		t := <-ticker.C
		s.mu.Lock()
		clear := !s.clearIn.IsZero() && s.clearIn.Before(t)
		s.mu.Unlock()
		if clear {
			s.Clear()
		}
		s.loader.Tick()
	}
}

func (s *Status) setMessage(text string, style tcell.Style, clearIn time.Duration) {
	s.mu.Lock()
	if clearIn == 0 {
		s.clearIn = time.Time{}
	} else {
		s.clearIn = time.Now().Add(clearIn)
	}
	s.text.SetStyle(style)
	s.text.SetText(text)
	s.mu.Unlock()
	s.screen.UpdateScreen()
}

func (s *Status) Clear() {
	s.setMessage("", s.screen.Theme().GetStyle("status-bar"), 0)
}

func (s *Status) Warning(msg string) {
	s.setMessage(msg, s.screen.Theme().GetStyle("status-warning"), time.Second*5)
}

func (s *Status) Info(msg string) {
	s.setMessage(msg, s.screen.Theme().GetStyle("status-info"), time.Second*2)
}

func (s *Status) Error(err error) {
	s.setMessage(err.Error(), s.screen.Theme().GetStyle("status-error"), time.Second*10)
}

func (s *Status) HandleEvent(ev tcell.Event) bool {
	if e, ok := ev.(*tcell.EventKey); ok {
		select {
		case s.events <- e:
			return true
		default:
		}
	}
	return false
}

func (s *Status) Confirm(msg string) bool {
	s.setMessage(msg, s.screen.Theme().GetStyle("status-confirm"), 0)
	ev := <-s.events
	switch ev.Rune() {
	case 'y', 'Y':
		return true
	}
	return false
}

func (s *Status) Draw() {
	s.once.Do(func() {
		s.mu.Lock()
		s.text.SetStyle(s.screen.Theme().GetStyle("status-bar"))
		s.mu.Unlock()
	})
	s.mu.Lock()
	s.loader.Draw()
	s.text.Draw()
	s.mu.Unlock()
}

func (s *Status) Size() (int, int) {
	return 1, 1
}

func NewStatus(screen commander.ScreenHandler) *Status {
	s := &Status{
		Focusable: focus.NewFocusable(),
		loader:    NewLoader(screen),
		text:      views.NewText(),
		screen:    screen,
		events:    make(chan *tcell.EventKey),
	}
	go s.watch()
	return s
}
