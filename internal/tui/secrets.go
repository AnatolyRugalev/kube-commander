package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SecretsTable struct {
	namespace string
}

func (ct *SecretsTable) Namespace() string {
	return ct.namespace
}

func (ct *SecretsTable) GetActions() []*widgets.ListAction {
	return GetDefaultActions(ct)
}

func (ct *SecretsTable) DeleteDescription(idx int, row widgets.ListRow) string {
	return "Secret " + row[0]
}

func (ct *SecretsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().Secrets(ct.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (ct *SecretsTable) TypeName() string {
	return "secrets"
}

func (ct *SecretsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func NewSecretsTable(namespace string) *widgets.DataTable {
	lt := widgets.NewDataTable(&SecretsTable{
		namespace: namespace,
	}, screen)
	lt.Title = "Secrets"
	return lt
}

func (ct *SecretsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "TYPE", "DATA", "AGE"}
}

func (ct *SecretsTable) LoadData() ([]widgets.ListRow, error) {
	secrets, err := kube.GetClient().CoreV1().Secrets(ct.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
	for _, secret := range secrets.Items {
		rows = append(rows, ct.newRow(secret))
	}
	return rows, nil
}

func (ct *SecretsTable) newRow(secret v1.Secret) widgets.ListRow {
	return widgets.ListRow{
		secret.Name,
		string(secret.Type),
		fmt.Sprintf("%d", len(secret.Data)),
		Age(secret.CreationTimestamp.Time),
	}
}
