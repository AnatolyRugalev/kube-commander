package theme

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme/themes"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/AnatolyRugalev/kube-commander/pb"
	"github.com/gdamore/tcell"
	"google.golang.org/protobuf/proto"
	"sync"
)

type manager struct {
	focus  commander.FocusManager
	screen commander.ScreenHandler
	status commander.StatusReporter
	config commander.ConfigUpdater

	initialized bool

	themeMu        sync.RWMutex
	themes         []*pb.Theme
	currentThemeId int
	currentTheme   *pb.Theme
	colors         map[string]*pb.Color
	styles         map[string]*pb.Style
}

func ColorToProto(c *commander.Color) *pb.Color {
	pbColor := &pb.Color{
		Name: c.Name,
	}
	if c.Color&tcell.ColorIsRGB != 0 {
		pbColor.Value = &pb.Color_Rgb{
			Rgb: fmt.Sprintf("%06x", c.Color.Hex()),
		}
	} else {
		pbColor.Value = &pb.Color_Xterm{
			Xterm: int32(c.Color),
		}
	}
	return pbColor
}

func ProtoToColor(pbColor *pb.Color) *commander.Color {
	c := &commander.Color{
		Name: pbColor.Name,
	}
	switch t := pbColor.Value.(type) {
	case *pb.Color_Rgb:
		c.Color = tcell.GetColor("#" + t.Rgb)
	case *pb.Color_Xterm:
		c.Color = tcell.Color(t.Xterm)
	default:
		c.Color = tcell.ColorDefault
	}
	return c
}

func (m *manager) ConfigUpdated(config *pb.Config) {
	current := config.CurrentTheme
	if current == "" {
		current = "base16"
	}
	allThemes := themes.DefaultThemes
	themeIndex := make(map[string]int)
	for i, t := range allThemes {
		themeIndex[t.Name] = i
	}
	for _, t := range config.Themes {
		if i, ok := themeIndex[t.Name]; ok {
			allThemes[i] = t
		} else {
			allThemes = append(allThemes, t)
			themeIndex[t.Name] = len(allThemes) - 1
		}
	}
	i, ok := themeIndex[current]
	if !ok {
		i = 0
	}
	theme := allThemes[i]
	m.themeMu.Lock()
	m.themes = allThemes
	m.currentThemeId = i
	changed := m.currentTheme == nil || !proto.Equal(m.currentTheme, theme)
	m.currentTheme = theme
	m.themeMu.Unlock()
	if changed {
		m.ApplyTheme(theme)
	}
}

func (m *manager) ApplyTheme(theme *pb.Theme) {
	colors := make(map[string]*pb.Color)
	styles := make(map[string]*pb.Style)
	if len(theme.Styles) == 0 {
		theme.Styles = themes.BaseStyles()
	}
	for _, c := range theme.Colors {
		colors[c.Name] = c
	}
	for _, s := range theme.Styles {
		styles[s.Name] = s
	}
	m.themeMu.Lock()
	m.colors = colors
	m.styles = styles
	m.themeMu.Unlock()
	m.screen.UpdateScreen()
	m.status.Info(fmt.Sprintf("Applied theme: %s", theme.Name))
}

func (m *manager) NextTheme() {
	m.themeMu.Lock()
	themeId := m.currentThemeId + 1
	if themeId >= len(m.themes) {
		themeId = 0
	}
	err := m.config.UpdateConfig(func(config *pb.Config) {
		config.CurrentTheme = m.themes[themeId].Name
	})
	if err != nil {
		m.status.Error(err)
	}
	m.themeMu.Unlock()
}

func (m *manager) PrevTheme() {
	m.themeMu.Lock()
	themeId := m.currentThemeId - 1
	if themeId < 0 {
		themeId = len(m.themes) - 1
	}
	err := m.config.UpdateConfig(func(config *pb.Config) {
		config.CurrentTheme = m.themes[themeId].Name
	})
	if err != nil {
		m.status.Error(err)
	}
	m.themeMu.Unlock()
}

func NewManager(screen commander.ScreenHandler, status commander.StatusReporter, config commander.ConfigUpdater) *manager {
	return &manager{
		screen: screen,
		status: status,
		config: config,
	}
}

func (m *manager) GetStyle(name string) commander.Style {
	style := tcell.StyleDefault
	m.themeMu.RLock()
	pbStyle, ok := m.styles[name]
	m.themeMu.RUnlock()
	if !ok {
		return style
	}
	fg := tcell.ColorDefault
	bg := tcell.ColorDefault
	fgColor, ok := m.colors[pbStyle.Fg]
	if ok {
		fg = ProtoToColor(fgColor).Color
	}
	style = style.Foreground(fg)
	bgColor, ok := m.colors[pbStyle.Bg]
	if ok {
		bg = ProtoToColor(bgColor).Color
	}
	style = style.Background(bg)
	for _, attr := range pbStyle.Attrs {
		switch attr {
		case pb.StyleAttribute_BOLD:
			style = style.Bold(true)
		case pb.StyleAttribute_BLINK:
			style = style.Blink(true)
		case pb.StyleAttribute_REVERSE:
			style = style.Reverse(true)
		case pb.StyleAttribute_UNDERLINE:
			style = style.Underline(true)
		case pb.StyleAttribute_DIM:
			style = style.Dim(true)
		}
	}
	return style
}
