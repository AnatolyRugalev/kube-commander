package listTable

import (
	"fmt"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

type loadFunc func() ([]Column, []Row, error)

type ReloadableListTable struct {
	*ListTable

	loaded bool
	loadM  *sync.Mutex
	load   loadFunc
	err    error
}

const (
	LoadingStarted = iota
	LoadingFinished
	LoadingError
)

type LoadingEvent struct {
	rlt  *ReloadableListTable
	t    time.Time
	kind int
	err  error
}

func (e *LoadingEvent) Widget() views.Widget {
	return e.rlt
}

func (e *LoadingEvent) When() time.Time {
	return e.t
}

func NewReloadableListTable(showHeader bool, load loadFunc) *ReloadableListTable {
	lt := NewListTable(nil, []Row{}, showHeader)
	return &ReloadableListTable{
		ListTable: lt,
		loadM:     &sync.Mutex{},
		load:      load,
	}
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
	r.PostEvent(&LoadingEvent{
		t:    time.Now(),
		rlt:  r,
		kind: LoadingStarted,
	})
	r.columns, r.rows, r.err = r.load()
	if r.err != nil {
		r.PostEvent(&LoadingEvent{
			t:    time.Now(),
			rlt:  r,
			kind: LoadingError,
			err:  r.err,
		})
		return
	}
	r.PostEvent(&LoadingEvent{
		t:    time.Now(),
		rlt:  r,
		kind: LoadingFinished,
	})
	r.ListTable.table = r.ListTable.renderTable()
	r.ListTable.Select(r.ListTable.selectedRow)
}

func (r *ReloadableListTable) OnDisplay() {
	if !r.loaded {
		r.Reload()
	}
}

func (r *ReloadableListTable) HandleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		if ev.Key() == tcell.KeyF5 {
			r.Reload()
			return true
		}
	}
	return r.ListTable.HandleEvent(ev)
}
