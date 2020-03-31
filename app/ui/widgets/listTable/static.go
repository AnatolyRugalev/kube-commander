package listTable

import "github.com/AnatolyRugalev/kube-commander/commander"

func NewStaticListTable(columns []string, rows []commander.Row, format TableFormat) *ListTable {
	lt := NewListTable(NewStaticRowProvider(columns, rows), format, nil)
	lt.watch()
	return lt
}

func NewStaticRowProvider(columns []string, rows []commander.Row) commander.RowProvider {
	prov := make(commander.RowProvider)
	go func() {
		ops := []commander.Operation{
			&commander.OpClear{},
			&commander.OpSetColumns{Columns: columns},
		}
		for _, row := range rows {
			ops = append(ops, &commander.OpAdded{Row: row})
		}
		prov <- ops
		close(prov)
	}()
	return prov
}
