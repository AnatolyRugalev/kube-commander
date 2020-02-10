package focus

import (
	"github.com/gdamore/tcell/views"
	"testing"
)

type focusableSpacer struct {
	views.Spacer
	Focusable
}

func TestFocusManager_Root(t *testing.T) {
	w := &focusableSpacer{}
	m := NewFocusManager(w, nil)
	if w != m.Root() {
		t.Fail()
	}
}

func TestFocusManager_Size(t *testing.T) {
	w1 := &focusableSpacer{}
	w2 := &focusableSpacer{}
	m := NewFocusManager(w1, nil)
	if m.StackSize() != 1 {
		t.Fail()
	}
	m.Focus(w2)
	if m.StackSize() != 2 {
		t.Fail()
	}
}

func TestFocusManager_Focus(t *testing.T) {
	w1 := &focusableSpacer{}
	w2 := &focusableSpacer{}
	w3 := &focusableSpacer{}
	m := NewFocusManager(w1, nil)
	m.Focus(w1)
	if m.Current() != m.Root() {
		t.Error("Current is not root")
		t.Fail()
	}
	m.Focus(w2)
	if m.Current() != w2 {
		t.Error("Current is not w2")
		t.Fail()
	}
	m.Focus(w3)
	if m.Current() != w3 {
		t.Error("Current is not w3")
		t.Fail()
	}
	m.Focus(w1)
	if m.StackSize() != 1 {
		t.Error("StackSize does not match")
		t.Fail()
	}
}

func TestFocusManager_Blur(t *testing.T) {
	w1 := &focusableSpacer{}
	w2 := &focusableSpacer{}
	w3 := &focusableSpacer{}
	m := NewFocusManager(w1, nil)
	m.Focus(w1)
	m.Focus(w2)
	m.Focus(w3)
	if m.Current() != w3 {
		t.Error("Current is not w3")
		t.Fail()
	}
	m.Blur()
	if m.Current() != w2 {
		t.Error("Current is not w2")
		t.Fail()
	}
	m.Blur()
	if m.Current() != w1 {
		t.Error("Current is not w1")
		t.Fail()
	}
	m.Blur()
	if m.Current() != w1 {
		t.Error("Current is not w1")
		t.Fail()
	}
}
