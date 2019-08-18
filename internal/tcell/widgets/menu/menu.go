package menu

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"time"
)

type SelectEvent struct {
	t      time.Time
	widget focus.FocusableWidget
}

func (s *SelectEvent) Widget() views.Widget {
	return s.widget
}

func (s *SelectEvent) When() time.Time {
	return s.t
}

type EventHandler struct {
	items []Item
}

func (m EventHandler) HandleRowEvent(event listTable.RowEvent) bool {
	switch ev := event.(type) {
	case *listTable.RowEventChange:
		ev.ListTable().PostEvent(
			&SelectEvent{
				t:      time.Now(),
				widget: m.items[ev.RowId()].widget,
			},
		)
		return true
	case *listTable.RowTcellEvent:
		switch te := ev.TcellEvent().(type) {
		case *tcell.EventKey:
			if te.Key() == tcell.KeyEnter {
				ev.ListTable().PostEvent(focus.NewFocusEvent(ev.ListTable(), m.items[ev.RowId()].widget))
				return true
			}
		}
	}
	return false
}

type Item struct {
	title  string
	widget focus.FocusableWidget
}

func NewItem(title string, widget focus.FocusableWidget) Item {
	return Item{
		title:  title,
		widget: widget,
	}
}

type Menu struct {
	*listTable.ListTable
}

func NewMenu(items []Item) *Menu {
	var rows []string
	for _, item := range items {
		rows = append(rows, item.title)
	}
	lt := listTable.NewList(rows)
	lt.SetEventHandler(&EventHandler{items: items})
	return &Menu{
		ListTable: lt,
	}
}
