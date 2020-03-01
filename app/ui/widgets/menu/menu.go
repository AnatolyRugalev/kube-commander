package menu

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
)

type SelectFunc func(itemId int, item commander.MenuItem) bool

var (
	DefaultSelectFunc = func(itemId int, item commander.MenuItem) bool { return false }
)

type item struct {
	title  string
	widget commander.Widget
}

func NewItem(title string, widget commander.Widget) commander.MenuItem {
	return item{
		title:  title,
		widget: widget,
	}
}

func (i item) Title() string {
	return i.title
}

func (i item) Widget() commander.Widget {
	return i.widget
}

type Menu struct {
	*listTable.ListTable
	items    []commander.MenuItem
	onSelect SelectFunc
}

func (m *Menu) SelectedItem() commander.MenuItem {
	if len(m.items) == 0 {
		return nil
	}
	rowId := m.ListTable.SelectedRowId()
	if len(m.items) < rowId {
		return nil
	}
	return m.items[rowId]
}

func (m *Menu) Items() []commander.MenuItem {
	return m.items
}

func (m *Menu) HandleEvent(ev tcell.Event) bool {
	return m.ListTable.HandleEvent(ev)
}

func (m *Menu) OnKeyPress(rowId int, _ commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		return m.onSelect(rowId, m.items[rowId])
	}
	return false
}

func (m *Menu) BindOnSelect(selectFunc SelectFunc) {
	oldFunc := m.onSelect
	m.onSelect = func(itemId int, item commander.MenuItem) bool {
		if selectFunc(itemId, item) {
			return true
		}
		return oldFunc(itemId, item)
	}
}

func NewMenu(items []commander.MenuItem) *Menu {
	var rows []string
	for _, item := range items {
		rows = append(rows, item.Title())
	}
	lt := listTable.NewList(rows)
	m := Menu{
		ListTable: lt,
		items:     items,
		onSelect:  DefaultSelectFunc,
	}
	lt.BindOnKeyPress(m.OnKeyPress)
	return &m
}
