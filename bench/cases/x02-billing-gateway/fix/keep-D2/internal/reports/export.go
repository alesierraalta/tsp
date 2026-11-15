package reports

import (
	"fmt"

	"example.com/billing/internal/csvx"
	"example.com/billing/internal/query"
	"example.com/billing/internal/store"
)

// ExportColumns is the column list of the usage export, in the order the file carries them.
var ExportColumns = []string{
	"id", "tenant", "account", "subscription", "metric", "quantity", "status", "day",
}

// exportLimit bounds the rows one export carries.
const exportLimit = 500

// ExportCSV renders the usage events of one tenant as a CSV file. The status filter is the one the
// caller sends with the download; it is read through the usage report's own declaration, so a value
// the report does not accept is refused and the accepted value is bound as an argument of the
// statement, never written into it.
func ExportCSV(s *store.Store, tenant, status string) (string, error) {
	if tenant == "" {
		return "", ErrNoTenant
	}
	definition := Usage()
	b := query.Select(ExportColumns...).From(definition.Table).Where("tenant", tenant)
	if status != "" {
		if err := query.Read(b, definition.Filters, map[string]string{"status": status}); err != nil {
			return "", fmt.Errorf("reports: export: %w", err)
		}
	}
	if err := b.Limit(exportLimit); err != nil {
		return "", err
	}
	b.OrderBy("id", false)
	stmt, err := b.Stmt()
	if err != nil {
		return "", err
	}
	result, err := s.Query(stmt)
	if err != nil {
		return "", fmt.Errorf("reports: export: %w", err)
	}
	return csvx.Render(ExportColumns, result.Strings()), nil
}

// ExportName is the file name a client is offered for the export.
func ExportName(tenant string) string {
	return "usage-" + tenant + ".csv"
}
