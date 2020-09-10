package listTable

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

var phases = []rune{
	'|',
	'\\',
	'-',
	'/',
}

type preloader struct {
	views.WidgetWatchers
	focus.Focusable
	sync.Mutex
	phase     int
	ticker    *time.Ticker
	view      views.View
	style     tcell.Style
	updater   commander.ScreenUpdater
	preloader *preloader
}

func NewPreloader(updater commander.ScreenUpdater) *preloader {
	return &preloader{
		updater: updater,
		phase:   -1,
	}
}

func (p *preloader) Start() {
	p.phase = 0
	p.Lock()
	p.ticker = time.NewTicker(time.Millisecond * 200)
	p.Unlock()
	go func() {
		for range p.ticker.C {
			p.phase++
			if p.phase >= len(phases) {
				p.phase = 0
			}
			p.updater.UpdateScreen()
		}
	}()
}

func (p *preloader) Stop() {
	p.Lock()
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.Unlock()
	p.phase = -1
	p.updater.UpdateScreen()
}

func (p *preloader) Draw() {
	if p.phase == -1 {
		return
	}
	p.view.SetContent(0, 0, phases[p.phase], nil, tcell.StyleDefault.Background(tcell.ColorTeal).Foreground(tcell.ColorBlack))
}

func (p *preloader) Resize() {
}

func (p *preloader) HandleEvent(_ tcell.Event) bool {
	return false
}

func (p *preloader) SetView(view views.View) {
	p.view = view
}

func (p *preloader) Size() (int, int) {
	return 1, 1
}
