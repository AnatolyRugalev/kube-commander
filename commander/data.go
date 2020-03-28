package commander

import (
	"github.com/spf13/cast"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type OpType uint8

const (
	OpClear OpType = iota
	OpAdded
	OpModified
	OpDeleted

	OpColumns
	OpLoading
	OpLoadingFinished
)

type Operation struct {
	Type OpType
	Row  Row
}

type KubernetesRow struct {
	md    *metav1.PartialObjectMetadata
	cells []string
}

func NewKubernetesRow(row metav1.TableRow) (*KubernetesRow, error) {
	obj := row.Object
	md := metav1.PartialObjectMetadata{}
	err := runtime.DecodeInto(unstructured.UnstructuredJSONScheme, obj.Raw, &md)
	if err != nil {
		return nil, err
	}
	var cells []string
	for _, cell := range row.Cells {
		cells = append(cells, cast.ToString(cell))
	}
	return &KubernetesRow{md: &md, cells: cells}, nil
}

func (k KubernetesRow) Id() string {
	if k.md.Namespace == "" {
		return k.md.Name
	}
	return k.md.Namespace + ":" + k.md.Name
}

func (k KubernetesRow) Cells() []string {
	return k.cells
}

func (k KubernetesRow) Metadata() *metav1.PartialObjectMetadata {
	return k.md
}
