package widgets

import (
	"sync"
)

type DataTable struct {
	*ListTable
	dataHandler DataTableHandler
	reloadMx    *sync.Mutex
}

type DataTableHandler interface {
	ListTableHandler
	LoadData() ([]ListRow, error)
}

type DataTableResource interface {
	DataTableHandler
	TypeName() string
	Name(row ListRow) string
}

type DataTableDeletable interface {
	DataTableHandler
	DeleteDescription(row ListRow) string
	Delete(row ListRow) error
}

type DataTableResourceNamespace interface {
	DataTableResource
	Namespace() string
}

func NewDataTable(handler DataTableHandler, screenHandler ScreenHandler) *DataTable {
	lt := &DataTable{
		ListTable:   NewListTable([]ListRow{}, handler, screenHandler),
		dataHandler: handler,
		reloadMx:    &sync.Mutex{},
	}
	return lt
}

func (lt *DataTable) Reload() error {
	lt.reloadMx.Lock()
	defer lt.reloadMx.Unlock()
	lt.rows = []ListRow{}
	data, err := lt.dataHandler.LoadData()
	if err != nil {
		return err
	}
	for _, row := range data {
		lt.rows = append(lt.rows, row)
	}
	if len(lt.rows) == 0 {
		lt.selectedRow = 0
	} else if lt.selectedRow >= len(lt.rows) {
		lt.selectedRow = len(lt.rows) - 1
	}
	return nil
}
