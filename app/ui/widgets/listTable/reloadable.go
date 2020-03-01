package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"sync"
)

type loadFunc func() ([]string, []commander.Row, error)

type ReloadableListTable struct {
	*ListTable

	errHandler commander.ErrorHandler
	loaded     bool
	loadM      *sync.Mutex
	load       loadFunc
	err        error
}

func NewReloadableListTable(errHandler commander.ErrorHandler, showHeader bool, load loadFunc) *ReloadableListTable {
	lt := NewListTable(nil, []commander.Row{}, showHeader)
	rlt := &ReloadableListTable{
		ListTable:  lt,
		errHandler: errHandler,
		loadM:      &sync.Mutex{},
		load:       load,
	}
	lt.BindOnKeyPress(rlt.OnKeyPress)
	return rlt
}

func (r *ReloadableListTable) OnKeyPress(_ int, _ commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyF5 || event.Key() == tcell.KeyCtrlR {
		r.Reload()
		return true
	}
	return false
}

func (r *ReloadableListTable) Draw() {
	if r.err != nil {
		r.drawError()
	} else {
		r.ListTable.Draw()
	}
}

func (r *ReloadableListTable) drawError() {
	r.view.Fill(' ', tcell.StyleDefault)
	str := fmt.Sprintf("err: %s", r.err.Error())
	for i, ch := range str {
		r.view.SetContent(i, 0, ch, nil, tcell.StyleDefault)
	}
}

func (r *ReloadableListTable) Reload() {
	r.loadM.Lock()
	defer r.loadM.Unlock()
	r.columns, r.rows, r.err = r.load()
	if r.err != nil {
		r.errHandler.HandleError(r.err)
		return
	}
	r.ListTable.table = r.ListTable.renderTable()
	r.ListTable.Select(r.ListTable.selectedRow)
	r.loaded = true
}

func (r *ReloadableListTable) OnDisplay() {
	if !r.loaded {
		r.Reload()
	}
}
