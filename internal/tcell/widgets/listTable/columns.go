package listTable

import (
	"errors"
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
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
	return fmt.Sprintf("%s", value), nil
}

func NewStringColumn(header string) *StringColumn {
	return &StringColumn{
		column: &column{header: header},
	}
}

type AgeColumn struct {
	*column
}

func NewAgeColumn() *AgeColumn {
	return &AgeColumn{
		column: &column{header: "Age"},
	}
}

func (a AgeColumn) Render(value interface{}) (string, error) {
	var t *time.Time
	if v, ok := value.(v1.Time); ok {
		t = &v.Time
	}
	if v, ok := value.(time.Time); ok {
		t = &v
	}
	if t == nil {
		return "", errors.New("invalid time")
	}
	return a.format(*t), nil
}

func (a AgeColumn) format(t time.Time) string {
	dur := time.Since(t).Round(time.Second)
	if dur > time.Hour*24 {
		days := dur.Nanoseconds() / (time.Hour * 24).Nanoseconds()
		return fmt.Sprintf("%dd", days)
	}
	if dur > time.Hour {
		hours := dur.Nanoseconds() / time.Hour.Nanoseconds()
		return fmt.Sprintf("%dh", hours)
	}
	if dur > time.Minute {
		minutes := dur.Nanoseconds() / time.Minute.Nanoseconds()
		return fmt.Sprintf("%dm", minutes)
	}
	return dur.String()
}
