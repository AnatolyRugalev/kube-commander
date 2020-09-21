package status

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"go.uber.org/atomic"
	"sync"
)

const (
	lastPhase   = 3
	loadingIcon = rune('☸')
)

type loader struct {
	views.WidgetWatchers
	sync.Mutex
	view   views.View
	screen commander.ScreenHandler

	stack *atomic.Int32
	phase *atomic.Int32
}

func NewLoader(screen commander.ScreenHandler) *loader {
	return &loader{
		screen: screen,
		stack:  atomic.NewInt32(0),
		phase:  atomic.NewInt32(0),
	}
}

func (l *loader) Start() {
	l.stack.Inc()
}

func (l *loader) Finish() {
	l.stack.Dec()
}

func (l *loader) Tick() {
	if !l.phase.CAS(lastPhase, 0) {
		l.phase.Inc()
	}
	if l.stack.Load() != 0 {
		l.screen.UpdateScreen()
	}
}

func (l *loader) Draw() {
	l.Lock()
	defer l.Unlock()
	if l.view == nil {
		return
	}
	if l.stack.Load() == 0 {
		style := l.screen.Theme().GetStyle("status-loader-idle")
		l.view.SetContent(0, 0, ' ', nil, style)
		l.view.SetContent(1, 0, ' ', nil, style)
	} else {
		phase := l.phase.Load()
		style := l.screen.Theme().GetStyle(fmt.Sprintf("status-loader-phase-%d", phase))
		l.view.SetContent(0, 0, loadingIcon, nil, style)
		l.view.SetContent(1, 0, ' ', nil, style)
	}
}

func (l *loader) Resize() {
}

func (l *loader) HandleEvent(_ tcell.Event) bool {
	return false
}

func (l *loader) SetView(view views.View) {
	l.Lock()
	l.view = view
	l.Unlock()
}

func (l *loader) Size() (int, int) {
	return 2, 1
}
