package focus

import (
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"time"
)

type FocusableWidget interface {
	views.Widget
	OnFocus()
	OnBlur()
}

type ChangeFocusEvent struct {
	when    time.Time
	widget  views.Widget
	focusTo FocusableWidget
}

func NewFocusEvent(widget views.Widget, to FocusableWidget) *ChangeFocusEvent {
	return &ChangeFocusEvent{
		when:    time.Now(),
		widget:  widget,
		focusTo: to,
	}
}

func (f *ChangeFocusEvent) FocusTo() FocusableWidget {
	return f.focusTo
}

func (f *ChangeFocusEvent) Widget() views.Widget {
	return f.widget
}

func (f *ChangeFocusEvent) When() time.Time {
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

func (f *Focusable) OnFocus() {
	f.focus = true
}

func (f *Focusable) OnBlur() {
	f.focus = false
}

func (f *Focusable) IsFocused() bool {
	return f.focus
}

func NewFocusable() *Focusable {
	return &Focusable{}
}

type Stack interface {
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

type Manager struct {
	stack []FocusableWidget
}

func (f *Manager) HandleEvent(ev tcell.Event) bool {
	switch t := ev.(type) {
	case *ChangeFocusEvent:
		f.Focus(t.FocusTo())
		return true
	case *BlurEvent:
		f.Blur()
		return true
	}
	return false
}

func (f *Manager) StackSize() int {
	return len(f.stack)
}

func NewFocusManager(root FocusableWidget) *Manager {
	root.OnFocus()
	manager := &Manager{
		stack: []FocusableWidget{root},
	}
	root.Watch(manager)
	return manager
}

func (f *Manager) Current() FocusableWidget {
	return f.stack[len(f.stack)-1]
}

func (f *Manager) Root() FocusableWidget {
	return f.stack[0]
}

func (f *Manager) Focus(widget FocusableWidget) {
	current := f.Current()
	if current == widget {
		return
	}
	current.OnBlur()
	widget.OnFocus()

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

func (f *Manager) Blur() {
	if len(f.stack) <= 1 {
		return
	}
	f.Current().OnBlur()
	f.stack[len(f.stack)-1].Unwatch(f)
	f.stack = f.stack[0 : len(f.stack)-1]
	f.Current().OnFocus()
}
