package commander

type RowProvider chan []Operation

type Row interface {
	Id() string
	Cells() []string
	Enabled() bool
}

type simpleRow struct {
	id      string
	cells   []string
	enabled bool
}

func (s simpleRow) Id() string {
	return s.id
}

func (s simpleRow) Cells() []string {
	return s.cells
}

func (s simpleRow) Enabled() bool {
	return s.enabled
}

func NewSimpleRow(id string, cells []string, enabled bool) *simpleRow {
	return &simpleRow{
		id:      id,
		cells:   cells,
		enabled: enabled,
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
