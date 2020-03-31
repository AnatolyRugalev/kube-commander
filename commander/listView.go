package commander

type RowProvider chan []Operation

type Row interface {
	Id() string
	Cells() []string
}

type simpleRow struct {
	id    string
	cells []string
}

func (s simpleRow) Id() string {
	return s.id
}

func (s simpleRow) Cells() []string {
	return s.cells
}

func NewSimpleRow(id string, cells []string) *simpleRow {
	return &simpleRow{
		id:    id,
		cells: cells,
	}
}

type ListView interface {
	MaxSizeWidget
	Rows() []Row
	SelectedRow() Row
	SelectedRowId() string
	SetStyler(styler ListViewStyler)
	SelectId(id string)
}

type ListViewStyler func(list ListView, row Row) Style

type ResourceListView interface {
	ListView
	Resource() *Resource
}

type MenuListView interface {
	ListView
	SelectedItem() MenuItem
	SelectItem(id string)
	SelectNext()
	SelectPrevious()
}

type MenuItem interface {
	Title() string
	Widget() Widget
	Position() int
}
