package reports

import (
	"fmt"
	"strings"

	"example.com/billing/internal/csvx"
	"example.com/billing/internal/store"
)

// ExportColumns is the column list of the usage export, in the order the file carries them.
var ExportColumns = []string{
	"id", "tenant", "account", "subscription", "metric", "quantity", "status", "day",
}

// exportLimit bounds the rows one export carries. The bound is a constant rather than the report's
// declaration because the export is a download an operator runs, not a page a client asks for.
const exportLimit = 500

// ExportCSV renders the usage events of one tenant as a CSV file. The status filter is the one the
// caller sends with the download and it narrows the tenant's events; a tenant is always named, so an
// export can never read the events of every tenant.
func ExportCSV(s *store.Store, tenant, status string) (string, error) {
	if tenant == "" {
		return "", ErrNoTenant
	}
	where := "tenant = '" + tenant + "'"
	if status != "" {
		where += " AND status = '" + status + "'"
	}
	text := "SELECT " + strings.Join(ExportColumns, ", ") + " FROM usage_events WHERE " + where +
		" ORDER BY id LIMIT " + fmt.Sprint(exportLimit)
	result, err := s.QueryText(text)
	if err != nil {
		return "", fmt.Errorf("reports: export: %w", err)
	}
	return csvx.Render(ExportColumns, result.Strings()), nil
}

// ExportName is the file name a client is offered for the export.
func ExportName(tenant string) string {
	return "usage-" + tenant + ".csv"
}
