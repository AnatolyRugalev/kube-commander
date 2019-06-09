package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/batch/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CronJobsTable struct {
	namespace string
}

func (ct *CronJobsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().BatchV1beta1().CronJobs(ct.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (ct *CronJobsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Job " + ct.namespace + "/" + row[0]
}

func (ct *CronJobsTable) TypeName() string {
	return "jobs"
}

func (ct *CronJobsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (ct *CronJobsTable) Namespace() string {
	return ct.namespace
}

func (ct *CronJobsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(ct)
}

func NewCronJobsTable(namespace string) *widgets.DataTable {
	pt := &CronJobsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "CronJobs <" + namespace + ">"
	return lt
}

func (ct *CronJobsTable) LoadData() ([]widgets.ListRow, error) {
	jobs, err := kube.GetClient().BatchV1beta1().CronJobs(ct.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, set := range jobs.Items {
		rows = append(rows, ct.newRow(set))
	}
	return rows, nil
}

func (ct *CronJobsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "SCHEDULE", "ACTIVE", "LAST SCHEDULE", "AGE"}
}

func (ct *CronJobsTable) newRow(job v1.CronJob) []string {
	last := "Never"
	if job.Status.LastScheduleTime != nil {
		last = Age(job.Status.LastScheduleTime.Time)
	}
	return widgets.ListRow{
		job.Name,
		job.Spec.Schedule,
		fmt.Sprintf("%d", len(job.Status.Active)),
		last,
		Age(job.CreationTimestamp.Time),
	}
}
