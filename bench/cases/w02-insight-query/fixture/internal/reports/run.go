package reports

import (
	"fmt"

	"example.com/insight/internal/query"
	"example.com/insight/internal/sqlmem"
)

// Run executes one report for one principal. The tenant of the caller is always applied: a report
// answers for the tenant the caller acts in, whatever filters were passed.
//
// Every statement the service runs is built here or in the other functions of this package through
// query.Builder, so a filter value travels as a bound argument and never as statement text.
func Run(db *sqlmem.DB, def Definition, tenant string, params map[string]string) (sqlmem.Result, error) {
	if tenant == "" {
		return sqlmem.Result{}, fmt.Errorf("reports: %s needs a tenant", def.Name)
	}
	if err := query.Known(def.Filters, params); err != nil {
		return sqlmem.Result{}, err
	}
	builder := query.Select(def.Columns...).From(def.Table).Where("tenant", tenant)
	builder, err := query.Apply(builder, def.Filters, params)
	if err != nil {
		return sqlmem.Result{}, err
	}
	if def.OrderBy != "" {
		builder = builder.OrderBy(def.OrderBy, def.Desc)
	}
	if err := builder.Limit(def.Limit); err != nil {
		return sqlmem.Result{}, err
	}
	stmt, err := builder.Stmt()
	if err != nil {
		return sqlmem.Result{}, err
	}
	return db.Query(stmt.Text, stmt.Args...)
}

// Row is one report row as the API encodes it: column name to value.
type Row map[string]any

// Rows converts a statement result into the rows the API serves.
func Rows(result sqlmem.Result) []Row {
	out := make([]Row, 0, len(result.Rows))
	for _, values := range result.Rows {
		row := make(Row, len(result.Columns))
		for i, col := range result.Columns {
			if i < len(values) {
				row[col] = values[i].String()
			}
		}
		out = append(out, row)
	}
	return out
}

// Describe renders the definition as the catalogue the API serves.
func Describe(def Definition) map[string]any {
	filters := make([]map[string]string, 0, len(def.Filters))
	for _, f := range def.Filters {
		filters = append(filters, map[string]string{"param": f.Param, "operator": f.Operator})
	}
	return map[string]any{
		"name":    def.Name,
		"title":   def.Title,
		"columns": def.Columns,
		"filters": filters,
		"limit":   def.Limit,
	}
}
