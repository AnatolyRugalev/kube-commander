package listTable

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

type loadFunc func() ([]string, []commander.Row, error)

type ReloadableListTable struct {
	*ListTable

	errHandler commander.ErrorHandler
	loaded     bool
	loadM      *sync.Mutex
	load       loadFunc
	err        error
	updater    commander.ScreenUpdater
	preloader  *preloader
}

func NewReloadableListTable(updater commander.ScreenUpdater, errHandler commander.ErrorHandler, showHeader bool, load loadFunc) *ReloadableListTable {
	lt := NewListTable(nil, []commander.Row{}, showHeader)
	rlt := &ReloadableListTable{
		ListTable:  lt,
		errHandler: errHandler,
		loadM:      &sync.Mutex{},
		load:       load,
		updater:    updater,
		preloader:  NewPreloader(updater),
	}
	lt.BindOnKeyPress(rlt.OnKeyPress)
	return rlt
}

func (r *ReloadableListTable) OnKeyPress(_ int, _ commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyF5 || event.Key() == tcell.KeyCtrlR {
		go r.Reload()
		return true
	}
	return false
}

func (r *ReloadableListTable) Draw() {
	if r.err != nil {
		r.drawError()
		return
	}
	r.ListTable.Draw()
	r.preloader.Draw()
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
	r.preloader.Start()
	defer r.preloader.Stop()
	time.Sleep(time.Second)
	r.columns, r.rows, r.err = r.load()
	if r.err != nil {
		r.errHandler.HandleError(r.err)
		return
	}
	r.ListTable.table = r.ListTable.renderTable()
	r.ListTable.Select(r.ListTable.selectedRow)
	r.loaded = true
}

func (r *ReloadableListTable) SetView(view views.View) {
	r.ListTable.SetView(view)
	r.preloader.SetView(view)
}
