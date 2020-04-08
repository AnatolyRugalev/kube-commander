package theme

import "github.com/AnatolyRugalev/kube-commander/commander"

type component struct {
	name  string
	style commander.Style
}

func (s *component) Name() string {
	return s.name
}

func (s *component) Style() commander.Style {
	return s.style
}

func (s *component) SetStyle(style commander.Style) {
	s.style = style
}

func NewComponent(name string, style commander.Style) *component {
	return &component{
		name:  name,
		style: style,
	}
}
