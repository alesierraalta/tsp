package reports

import (
	"fmt"
	"strings"

	"example.com/insight/internal/sqlmem"
)

// ExportColumns is the column list of the export, in the order the file carries them.
var ExportColumns = []string{"id", "tenant", "account", "status", "amount", "created"}

// ExportCSV renders the events of one tenant as CSV. The status filter is the one a caller sends
// with the download, and a status the store does not carry is refused before anything runs.
func ExportCSV(db *sqlmem.DB, tenant, status string) (string, error) {
	if tenant == "" {
		return "", fmt.Errorf("reports: export needs a tenant")
	}
	where := "tenant = '" + tenant + "'"
	if status != "" {
		where += " AND status = '" + status + "'"
	}
	sql := "SELECT " + strings.Join(ExportColumns, ", ") + " FROM events WHERE " + where + " ORDER BY id LIMIT 500"
	result, err := db.QueryText(sql)
	if err != nil {
		return "", fmt.Errorf("reports: export: %w", err)
	}
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
