package widgets

import (
	"image"
	"sync"
	"time"

	"github.com/gizak/termui/v3"
)

type Preloader struct {
	*termui.Block
	phaseM    *sync.Mutex
	phase     int
	loadFunc  func() error
	onSuccess func()
	onError   func(err error)
	onCancel  func()

	cancel chan struct{}
}

var preloaderColors = []termui.Color{
	termui.Color(17),
	termui.Color(18),
	termui.Color(19),
	termui.Color(20),
	termui.Color(21),
}

func (p *Preloader) Draw(buf *termui.Buffer) {
	p.phaseM.Lock()
	phase := p.phase
	p.phaseM.Unlock()
	for i := range preloaderColors {
		j := (i + phase) % len(preloaderColors)
		color := preloaderColors[j]
		buf.SetCell(termui.Cell{
			Rune:  ' ',
			Style: termui.NewStyle(0, color),
		}, image.Pt(p.Min.X+len(preloaderColors)-i, p.Min.Y))
	}
}

func (p *Preloader) incrementPhase() {
	p.phaseM.Lock()
	p.phase = (p.phase + 1) % len(preloaderColors)
	p.phaseM.Unlock()
}

func NewPreloader(screenRect image.Rectangle, loadFunc func() error, onSuccess func(), onError func(err error), onCancel func()) *Preloader {
	block := termui.NewBlock()
	block.Border = false
	startX := (screenRect.Max.X / 2) - len(preloaderColors)/2
	startY := (screenRect.Max.Y / 2) - len(preloaderColors)/2
	block.Rectangle.Min = image.Point{
		X: startX,
		Y: startY,
	}
	block.Rectangle.Max = image.Point{
		X: startX + len(preloaderColors),
		Y: startY + 1,
	}
	return &Preloader{
		Block:     block,
		phaseM:    &sync.Mutex{},
		loadFunc:  loadFunc,
		onSuccess: onSuccess,
		onError:   onError,
		onCancel:  onCancel,
	}
}

func (p *Preloader) OnEvent(event *termui.Event) bool {
	switch event.ID {
	case "<Escape>":
		close(p.cancel)
	}
	return false
}

func (p *Preloader) startLoading() <-chan error {
	loaded := make(chan error)
	go func() {
		loaded <- p.loadFunc()
	}()
	return loaded
}

func (p *Preloader) Run() {
	p.cancel = make(chan struct{})
	loaded := p.startLoading()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.incrementPhase()
			termui.Render(p)
		case err := <-loaded:
			if err != nil {
				p.onError(err)
			} else {
				p.onSuccess()
			}
			return
		case <-p.cancel:
			p.onCancel()
			return
		}
	}

}
