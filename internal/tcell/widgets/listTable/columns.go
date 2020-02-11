package listTable

import (
	"github.com/spf13/cast"
)

type Column interface {
	Header() string
	Render(value interface{}) (string, error)
}

type column struct {
	header string
}

func (c column) Header() string {
	return c.header
}

type StringColumn struct {
	*column
}

func (s StringColumn) Render(value interface{}) (string, error) {
	return cast.ToString(value), nil
}

func NewStringColumn(header string) *StringColumn {
	return &StringColumn{
		column: &column{header: header},
	}
}
