package listTable

import (
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

// TODO: error handling
type loadFunc func() []Row

type ReloadableListTable struct {
	*ListTable

	loaded bool
	loadM  *sync.Mutex
	load   loadFunc
}

const (
	LoadingStageStart  = 0
	LoadingStageFinish = 1
)

type LoadingEvent struct {
	rlt   *ReloadableListTable
	t     time.Time
	stage int
}

func (e *LoadingEvent) Widget() views.Widget {
	return e.rlt
}

func (e *LoadingEvent) When() time.Time {
	return e.t
}

func NewReloadableListTable(columns []Column, showHeader bool, load loadFunc) *ReloadableListTable {
	lt := NewListTable(columns, []Row{}, showHeader)
	return &ReloadableListTable{
		ListTable: lt,
		loadM:     &sync.Mutex{},
		load:      load,
	}
}

func (r *ReloadableListTable) Reload() {
	r.loadM.Lock()
	r.PostEvent(&LoadingEvent{
		t:     time.Now(),
		rlt:   r,
		stage: LoadingStageStart,
	})
	r.rows = r.load()
	r.PostEvent(&LoadingEvent{
		t:     time.Now(),
		rlt:   r,
		stage: LoadingStageFinish,
	})
	r.loadM.Unlock()
}

func (r *ReloadableListTable) OnDisplay() {
	if !r.loaded {
		r.Reload()
	}
}

func (r *ReloadableListTable) HandleEvent(ev tcell.Event) bool {
	if !r.IsFocused() {
		return false
	}
	switch ev := ev.(type) {
	case *tcell.EventKey:
		if ev.Key() == tcell.KeyF5 {
			r.Reload()
			return true
		}
	}
	return r.ListTable.HandleEvent(ev)
}
