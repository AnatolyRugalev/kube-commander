package widgets

import "sync/atomic"

type DataTable struct {
	*ListTable
	dataHandler DataTableHandler
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
	DeleteDescription(idx int, row ListRow) string
	Delete(idx int, row ListRow) error
}

type DataTableResourceNamespace interface {
	DataTableResource
	Namespace() string
}

func NewDataTable(handler DataTableHandler, screenHandler ScreenHandler) *DataTable {
	lt := &DataTable{
		ListTable:   NewListTable([]ListRow{}, handler, screenHandler),
		dataHandler: handler,
	}
	return lt
}

func (lt *DataTable) Reload() error {
	atomic.StoreInt32(&lt.loadingFlag, 1)
	defer atomic.StoreInt32(&lt.loadingFlag, 0)

	lt.rows = []ListRow{}
	data, err := lt.dataHandler.LoadData()
	if err != nil {
		return err
	}
	lt.SetRows(data)

	return nil
}
