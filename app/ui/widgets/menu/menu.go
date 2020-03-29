package menu

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
)

type SelectFunc func(itemId string, item commander.MenuItem) bool

var (
	DefaultSelectFunc = func(itemId string, item commander.MenuItem) bool { return false }
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
	items    map[string]commander.MenuItem
	onSelect SelectFunc
}

func (m *Menu) SelectedItem() commander.MenuItem {
	row := m.ListTable.SelectedRow()
	if row == nil {
		return nil
	}
	return m.items[row.Id()]
}

func (m *Menu) HandleEvent(ev tcell.Event) bool {
	return m.ListTable.HandleEvent(ev)
}

func (m *Menu) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		return m.onSelect(row.Id(), m.items[row.Id()])
	}
	return false
}

func (m *Menu) BindOnSelect(selectFunc SelectFunc) {
	oldFunc := m.onSelect
	m.onSelect = func(itemId string, item commander.MenuItem) bool {
		if selectFunc(itemId, item) {
			return true
		}
		return oldFunc(itemId, item)
	}
}

func (m *Menu) SelectItem(id string) {
	item, ok := m.items[id]
	if !ok {
		return
	}
	m.ListTable.SelectId(id)
	m.onSelect(id, item)
}

func (m *Menu) SelectNext() {
	m.ListTable.SelectIndex(m.ListTable.SelectedRowIndex() + 1)
	id := m.ListTable.SelectedRowId()
	item, ok := m.items[id]
	if !ok {
		return
	}
	m.onSelect(id, item)
}

func (m *Menu) SelectPrevious() {
	m.ListTable.SelectIndex(m.ListTable.SelectedRowIndex() - 1)
}

func NewMenu(items []commander.MenuItem) *Menu {
	var rows []commander.Row
	itemMap := make(map[string]commander.MenuItem)
	for _, item := range items {
		rows = append(rows, commander.NewSimpleRow(item.Title(), []string{item.Title()}))
		itemMap[item.Title()] = item
	}
	lt := listTable.NewStaticListTable([]string{"Title"}, rows, listTable.NoHorizontalScroll)
	m := Menu{
		ListTable: lt,
		items:     itemMap,
		onSelect:  DefaultSelectFunc,
	}
	lt.BindOnKeyPress(m.OnKeyPress)
	return &m
}
