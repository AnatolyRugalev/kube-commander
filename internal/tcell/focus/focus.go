package focus

import (
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"time"
)

type FocusableWidget interface {
	views.Widget
	SetFocus(focus bool)
	Focus() bool
}

type FocusChangeEvent interface {
	views.EventWidget
	FocusTo() FocusableWidget
}

type FocusBlurEvent interface {
	views.EventWidget
}

type FocusEvent struct {
	when    time.Time
	widget  views.Widget
	focusTo FocusableWidget
}

func NewFocusEvent(widget views.Widget, to FocusableWidget) *FocusEvent {
	return &FocusEvent{
		when:    time.Now(),
		widget:  widget,
		focusTo: to,
	}
}

func (f *FocusEvent) FocusTo() FocusableWidget {
	return f.focusTo
}

func (f *FocusEvent) Widget() views.Widget {
	return f.widget
}

func (f *FocusEvent) When() time.Time {
	return f.when
}

type BlurEvent struct {
	when   time.Time
	widget views.Widget
}

func (b *BlurEvent) Widget() views.Widget {
	return b.widget
}

func (b *BlurEvent) When() time.Time {
	return b.when
}

func NewBlurEvent(widget views.Widget) *BlurEvent {
	return &BlurEvent{
		when:   time.Now(),
		widget: widget,
	}
}

type Focusable struct {
	focus bool
}

func (f *Focusable) SetFocus(focus bool) {
	f.focus = focus
}

func (f *Focusable) Focus() bool {
	return f.focus
}

func NewFocusable() *Focusable {
	return &Focusable{}
}

type FocusStack interface {
	// Returns root widget which presents on top of focus stack
	Root() FocusableWidget
	// Put widget to focus stack
	Focus(widget FocusableWidget)
	// Remove focus from focused widget and reduce focus stack if not on root widget
	Blur()
	// Returns currently focused widget
	Current() FocusableWidget
	// Returns stack size
	StackSize() int
}

type FocusManager struct {
	stack []FocusableWidget
}

func (f *FocusManager) HandleEvent(ev tcell.Event) bool {
	switch t := ev.(type) {
	case FocusChangeEvent:
		f.Focus(t.FocusTo())
		return true
	case FocusBlurEvent:
		f.Blur()
		return true
	}
	return false
}

func (f *FocusManager) StackSize() int {
	return len(f.stack)
}

func NewFocusManager(root FocusableWidget) *FocusManager {
	root.SetFocus(true)
	manager := &FocusManager{
		stack: []FocusableWidget{root},
	}
	root.Watch(manager)
	return manager
}

func (f *FocusManager) Current() FocusableWidget {
	return f.stack[len(f.stack)-1]
}

func (f *FocusManager) Root() FocusableWidget {
	return f.stack[0]
}

func (f *FocusManager) Focus(widget FocusableWidget) {
	current := f.Current()
	if current == widget {
		return
	}
	current.SetFocus(false)
	widget.SetFocus(true)

	// Find if this widget persist in focus stack
	for i, w := range f.stack {
		if w == widget {
			// Reduce focus stack to focus on found widget
			for j := i + 1; j < len(f.stack); j++ {
				f.stack[j].Unwatch(f)
			}
			f.stack = f.stack[0 : i+1]
			return
		}
	}
	f.stack = append(f.stack, widget)
	widget.Watch(f)
}

func (f *FocusManager) Blur() {
	if len(f.stack) <= 1 {
		return
	}
	f.Current().SetFocus(false)
	f.stack[len(f.stack)-1].Unwatch(f)
	f.stack = f.stack[0 : len(f.stack)-1]
	f.Current().SetFocus(true)
}
