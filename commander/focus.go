package commander

import "github.com/gdamore/tcell"

type FocusManager interface {
	HandleEvent(e tcell.Event, useStack bool) bool
	// Returns root widget which presents on top of focus stack
	Root() Widget
	// Put widget to focus stack
	Focus(widget Widget)
	// Remove focus from focused widget and reduce focus stack if not on root widget
	Blur()
	// Returns currently focused widget
	Current() Widget
	// Returns stack size
	StackSize() int
}
