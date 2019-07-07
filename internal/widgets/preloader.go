package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/theme"
	"image"
	"sync"
	"time"

	"github.com/gizak/termui/v3"
)

type Preloader struct {
	*termui.Block
	phaseM *sync.Mutex
	phase  int

	ticker  *time.Ticker
	tickerM *sync.Mutex

	cancel chan struct{}
}

func (p *Preloader) Draw(buf *termui.Buffer) {
	p.tickerM.Lock()
	isActive := p.ticker != nil
	p.tickerM.Unlock()

	p.phaseM.Lock()
	phase := p.phase
	p.phaseM.Unlock()
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
	p.phaseM.Lock()
	p.phase = (p.phase + 1) % len(theme.PreloaderColors)
	p.phaseM.Unlock()
}

func NewPreloader() *Preloader {
	block := termui.NewBlock()
	block.Border = false
	return &Preloader{
		Block:   block,
		phase:   0,
		phaseM:  &sync.Mutex{},
		tickerM: &sync.Mutex{},
	}
}

func (p *Preloader) Run(screenRect image.Rectangle, loadFunc func() error, onError func(error)) {
	p.tickerM.Lock()
	if p.ticker == nil {
		p.ticker = time.NewTicker(100 * time.Millisecond)
	} else {
		close(p.cancel)
	}
	p.tickerM.Unlock()
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

	loaded := make(chan error)
	p.cancel = make(chan struct{})
	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.incrementPhase()
				termui.Render(p)
			case err := <-loaded:
				if err != nil {
					onError(err)
				}
				close(loaded)

				p.ticker.Stop()
				p.tickerM.Lock()
				p.ticker = nil
				p.tickerM.Unlock()

				termui.Render(p)

				return
			case <-p.cancel:
				return
			}
		}
	}()
	go func() {
		loaded <- loadFunc()
	}()
}
