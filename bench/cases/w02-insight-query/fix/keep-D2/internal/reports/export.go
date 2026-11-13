package reports

import (
	"fmt"
	"strings"

	"example.com/insight/internal/query"
	"example.com/insight/internal/sqlmem"
	"example.com/insight/internal/store"
)

// ExportColumns is the column list of the export, in the order the file carries them.
var ExportColumns = []string{"id", "tenant", "account", "status", "amount", "created"}

// ExportCSV renders the events of one tenant as CSV. The status filter is the one a caller sends
// with the download, and a status the store does not carry is refused before anything runs.
func ExportCSV(db *sqlmem.DB, tenant, status string) (string, error) {
	if tenant == "" {
		return "", fmt.Errorf("reports: export needs a tenant")
	}
	where := ""
	if status != "" {
		if !store.IsStatus(status) {
			return "", fmt.Errorf("reports: status %q is not accepted", status)
		}
	}
	builder := query.Select(ExportColumns...).From(store.TableEvents).Where("tenant", tenant)
	if status != "" {
		builder = builder.Where("status", status)
	}
	builder = builder.OrderBy("id", false)
	if err := builder.Limit(500); err != nil {
		return "", err
	}
	stmt, err := builder.Stmt()
	if err != nil {
		return "", err
	}
	result, err := db.Query(stmt.Text, stmt.Args...)
	if err != nil {
		return "", fmt.Errorf("reports: export: %w", err)
	}
	_ = where
	var b strings.Builder
	b.WriteString(strings.Join(ExportColumns, ","))
	b.WriteString("\n")
	for _, values := range result.Rows {
		cells := make([]string, 0, len(values))
		for _, value := range values {
			cells = append(cells, csvCell(value.String()))
		}
		b.WriteString(strings.Join(cells, ","))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// csvCell quotes a cell the way the export contract promises: text with a comma, a quote or a line
// break is quoted, and a quote inside is doubled.
func csvCell(value string) string {
	if !strings.ContainsAny(value, ",\"\n\r") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
