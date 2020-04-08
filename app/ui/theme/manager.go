package theme

import (
	"errors"
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
)

type manager struct {
	focus    commander.FocusManager
	updater  commander.ScreenUpdater
	reporter commander.StatusReporter

	initialized    bool
	stylable       commander.Stylable
	component      commander.StyleComponent
	componentIndex int
}

func NewManager(focus commander.FocusManager, updater commander.ScreenUpdater, reporter commander.StatusReporter) *manager {
	return &manager{
		focus:    focus,
		updater:  updater,
		reporter: reporter,
	}
}

func (m *manager) Init() error {
	widget := m.focus.Current()
	stylable, ok := widget.(commander.Stylable)
	if !ok {
		return errors.New("this widget cannot be stylized")
	}
	components := stylable.GetComponents()
	if len(components) == 0 {
		return errors.New("this widget does not contain components to stylize")
	}
	m.stylable = stylable
	m.initialized = true
	m.switchComponent(0)
	return nil
}

func (m *manager) DeInit() {
	m.stylable = nil
	m.component = nil
	m.componentIndex = 0
	m.initialized = false
}

func (m *manager) HandleEvent(e tcell.Event) bool {
	if !m.initialized {
		return false
	}
	ev, ok := e.(*tcell.EventKey)
	if !ok {
		return false
	}
	switch ev.Key() {
	case tcell.KeyPgUp:
		m.PrevComponent()
		return true
	case tcell.KeyPgDn:
		m.NextComponent()
		return true
	case tcell.KeyUp:
		m.PrevBg()
		return true
	case tcell.KeyDown:
		m.NextBg()
		return true
	case tcell.KeyLeft:
		m.PrevFg()
		return true
	case tcell.KeyRight:
		m.NextFg()
		return true
	}

	switch ev.Rune() {
	case 'b':
		m.SwitchAttr(tcell.AttrBold)
		return true
	case 'l':
		m.SwitchAttr(tcell.AttrBlink)
		return true
	case 'r':
		m.SwitchAttr(tcell.AttrReverse)
		return true
	case 'u':
		m.SwitchAttr(tcell.AttrUnderline)
		return true
	case 'd':
		m.SwitchAttr(tcell.AttrDim)
		return true
	}

	return false
}

func (m *manager) NextComponent() {
	m.switchComponent(m.componentIndex + 1)
}

func (m *manager) switchComponent(index int) {
	components := m.stylable.GetComponents()
	if index >= len(components) {
		index = 0
	} else if index < 0 {
		index = len(components) - 1
	}
	m.componentIndex = index
	m.component = components[index]
	m.reporter.Info(fmt.Sprintf("Editing component: %s", m.component.Name()))
	m.updater.UpdateScreen()
}

func (m manager) PrevComponent() {
	m.switchComponent(m.componentIndex - 1)
}

func (m *manager) NextBg() {
	m.setBg(1)
}

func (m *manager) setBg(delta int32) {
	style := m.component.Style()
	_, bg, _ := style.Decompose()
	bg = m.shiftColor(bg, delta)
	m.component.SetStyle(style.Background(bg))
	m.reporter.Info(fmt.Sprintf("bg color: %d", bg))
	m.updater.UpdateScreen()
}

func (m *manager) setFg(delta int32) {
	style := m.component.Style()
	fg, _, _ := style.Decompose()
	fg = m.shiftColor(fg, delta)
	m.component.SetStyle(style.Foreground(fg))
	m.reporter.Info(fmt.Sprintf("fg color: %d", fg))
	m.updater.UpdateScreen()
}

func (m *manager) shiftColor(c tcell.Color, delta int32) tcell.Color {
	c = c + tcell.Color(delta)
	if c > tcell.ColorYellowGreen {
		c = tcell.ColorBlack
	} else if c < 0 {
		c = tcell.ColorYellowGreen
	}
	return c
}

func (m *manager) PrevBg() {
	m.setBg(-1)
}

func (m *manager) NextFg() {
	m.setFg(1)
}

func (m *manager) PrevFg() {
	m.setFg(-1)
}

func (m *manager) SwitchAttr(a tcell.AttrMask) {
	style := m.component.Style()
	_, _, attr := style.Decompose()
	on := attr&a == 0
	switch a {
	case tcell.AttrBold:
		style = style.Bold(on)
		m.reporter.Info(fmt.Sprintf("bold: %v", on))
	case tcell.AttrBlink:
		style = style.Blink(on)
		m.reporter.Info(fmt.Sprintf("blink: %v", on))
	case tcell.AttrReverse:
		style = style.Reverse(on)
		m.reporter.Info(fmt.Sprintf("reverse: %v", on))
	case tcell.AttrUnderline:
		style = style.Underline(on)
		m.reporter.Info(fmt.Sprintf("underline: %v", on))
	case tcell.AttrDim:
		style = style.Dim(on)
		m.reporter.Info(fmt.Sprintf("dim: %v", on))
	default:
		return
	}
	m.component.SetStyle(style)
	m.updater.UpdateScreen()
}
