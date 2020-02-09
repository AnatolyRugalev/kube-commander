package events

import (
	"github.com/gdamore/tcell/views"
	"time"
)

type NamespaceChanged struct {
	widget    views.Widget
	namespace string
	t         time.Time
}

func (n NamespaceChanged) Namespace() string {
	return n.namespace
}

func (n NamespaceChanged) Widget() views.Widget {
	return n.widget
}

func (n NamespaceChanged) When() time.Time {
	return n.t
}

func NewNamespaceChanged(widget views.Widget, namespace string) *NamespaceChanged {
	return &NamespaceChanged{
		widget:    widget,
		namespace: namespace,
		t:         time.Now(),
	}
}
