package tui

type Loadable interface {
	Reload(chan<- error)
}
