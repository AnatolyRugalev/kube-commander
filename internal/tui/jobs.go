package tui

import (
	"fmt"
	"time"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobsTable struct {
	namespace string
}

func (jt *JobsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().BatchV1().Jobs(jt.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (jt *JobsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Job " + jt.namespace + "/" + row[0]
}

func (jt *JobsTable) TypeName() string {
	return "jobs"
}

func (jt *JobsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (jt *JobsTable) Namespace() string {
	return jt.namespace
}

func (jt *JobsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(jt)
}

func NewJobsTable(namespace string) *widgets.DataTable {
	pt := &JobsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "Jobs <" + namespace + ">"
	return lt
}

func (jt *JobsTable) LoadData() ([]widgets.ListRow, error) {
	jobs, err := kube.GetClient().BatchV1().Jobs(jt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, set := range jobs.Items {
		rows = append(rows, jt.newRow(set))
	}
	return rows, nil
}

func (jt *JobsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "COMPLETIONS", "DURATION", "AGE"}
}

func (jt *JobsTable) newRow(job v1.Job) []string {
	var duration string
	if job.Status.CompletionTime != nil {
		duration = job.Status.CompletionTime.Time.Sub(job.Status.StartTime.Time).Round(time.Second).String()
	} else if job.Status.StartTime != nil {
		duration = time.Since(job.Status.StartTime.Time).Round(time.Second).String()
	}
	return widgets.ListRow{
		job.Name,
		fmt.Sprintf("%d/%d", job.Status.Succeeded, *job.Spec.Completions),
		duration,
		Age(job.CreationTimestamp.Time),
	}
}
