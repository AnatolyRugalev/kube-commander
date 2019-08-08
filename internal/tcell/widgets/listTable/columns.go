package listTable

import "errors"

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
	if str, ok := value.(string); ok {
		return str, nil
	}
	return "", errors.New("not a string")
}

func NewStringColumn(header string) *StringColumn {
	return &StringColumn{
		column: &column{header: header},
	}
}

type IntColumn struct {
	*column
}

func (s IntColumn) Render(value interface{}) (int, error) {
	if str, ok := value.(int); ok {
		return str, nil
	}
	return 0, errors.New("not a int")
}

func NewIntColumn(header string) *IntColumn {
	return &IntColumn{
		column: &column{header: header},
	}
}
