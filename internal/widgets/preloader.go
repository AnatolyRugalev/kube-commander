package widgets

import (
	"image"
	"sync"
	"time"

	"github.com/AnatolyRugalev/kube-commander/internal/theme"

	"github.com/gizak/termui/v3"
)

type Preloader struct {
	*termui.Block
	mux      sync.Mutex
	phase    int
	isActive bool

	cancel chan struct{}
}

func (p *Preloader) Draw(buf *termui.Buffer) {
	p.mux.Lock()
	phase := p.phase
	isActive := p.isActive
	p.mux.Unlock()

	for i := range theme.PreloaderColors {
		var color termui.Color
		j := (i + phase) % len(theme.PreloaderColors)
		if isActive {
			color = theme.PreloaderColors[j]
		} else {
			color = theme.PreloaderIdleColor
		}
		buf.SetCell(termui.Cell{
			Rune:  termui.HORIZONTAL_LINE,
			Style: termui.NewStyle(color),
		}, image.Pt(p.Min.X+len(theme.PreloaderColors)-i-1, p.Min.Y))
	}
}

func (p *Preloader) incrementPhase() {
	p.mux.Lock()
	p.phase = (p.phase + 1) % len(theme.PreloaderColors)
	p.mux.Unlock()
}

func NewPreloader() *Preloader {
	block := termui.NewBlock()
	block.Border = false
	return &Preloader{
		Block: block,
		phase: 0,
	}
}

func (p *Preloader) startLoading(loadFunc func() error) <-chan error {
	loaded := make(chan error)
	go func(loadFunc func() error) {
		loaded <- loadFunc()
	}(loadFunc)
	return loaded
}

func (p *Preloader) init(screenRect image.Rectangle) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.isActive = true
	startX := screenRect.Max.X - len(theme.PreloaderColors) - 1
	startY := 0
	p.Block.Rectangle.Min = image.Point{
		X: startX,
		Y: startY,
	}
	p.Block.Rectangle.Max = image.Point{
		X: startX + len(theme.PreloaderColors),
		Y: startY + 1,
	}
}

func (p *Preloader) Run(screenRect image.Rectangle, loadFunc func() error, onError func(error)) {
	p.cancel = make(chan struct{})
	loaded := p.startLoading(loadFunc)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	p.init(screenRect)

	for {
		select {
		case <-ticker.C:
			p.incrementPhase()
			termui.Render(p)
		case err := <-loaded:
			p.mux.Lock()
			p.isActive = false
			p.mux.Unlock()
			if err != nil {
				onError(err)
			}
			return
		case <-p.cancel:
			p.mux.Lock()
			p.isActive = false
			p.mux.Unlock()
			return
		}
	}

}
