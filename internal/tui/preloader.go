package tui

import (
	ui "github.com/gizak/termui/v3"
	"image"
	"sync"
	"time"
)

type Preloader struct {
	*ui.Block
	phaseM *sync.Mutex
	phase  int

	done <-chan struct{}
}

var preloaderColors = []ui.Color{
	ui.Color(17),
	ui.Color(18),
	ui.Color(19),
	ui.Color(20),
	ui.Color(21),
}

func (p *Preloader) Draw(buf *ui.Buffer) {
	p.phaseM.Lock()
	phase := p.phase
	p.phaseM.Unlock()
	for i := range preloaderColors {
		j := (i + phase) % len(preloaderColors)
		color := preloaderColors[j]
		buf.SetCell(ui.Cell{
			Rune:  ' ',
			Style: ui.NewStyle(0, color),
		}, image.Pt(p.Min.X+len(preloaderColors)-i, p.Min.Y))
	}
}

func (Preloader) OnEvent(event *ui.Event) bool {
	return false
}

func (p *Preloader) incrementPhase() {
	p.phaseM.Lock()
	p.phase = (p.phase + 1) % len(preloaderColors)
	p.phaseM.Unlock()
}

func NewPreloader(screenRect image.Rectangle, done <-chan struct{}) *Preloader {
	block := ui.NewBlock()
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
	p := &Preloader{
		Block:  block,
		phaseM: &sync.Mutex{},
		done:   done,
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ticker.C:
				p.incrementPhase()
				ui.Render(p)
			case <-p.done:
				ticker.Stop()
				return
			}
		}
	}()
	return p
}
