package query

import (
	"fmt"
	"strings"
)

// Filter is one accepted filter of a report: the parameter name a caller may send, the column it
// compares against, and the operator. A parameter no Filter names is refused rather than ignored,
// so a report cannot silently answer a question nobody asked.
type Filter struct {
	Param    string
	Column   string
	Operator string
}

// Apply adds every filter whose parameter the caller supplied, in the order the report declares
// them. The values travel as arguments.
func Apply(b *Builder, filters []Filter, params map[string]string) (*Builder, error) {
	for _, f := range filters {
		raw, ok := params[f.Param]
		if !ok || raw == "" {
			continue
		}
		switch f.Operator {
		case "=", "!=", "<", "<=", ">", ">=":
			b = b.WhereOp(f.Column, f.Operator, raw)
		default:
			return nil, fmt.Errorf("query: filter %q has operator %q", f.Param, f.Operator)
		}
	}
	return b, nil
}

// Known reports whether every parameter the caller supplied is one of the filters given.
func Known(filters []Filter, params map[string]string) error {
	accepted := make(map[string]bool, len(filters))
	for _, f := range filters {
		accepted[f.Param] = true
	}
	var unknown []string
	for param := range params {
		if !accepted[param] {
			unknown = append(unknown, param)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("query: unknown filter %s", strings.Join(sorted(unknown), ", "))
	}
	return nil
}

func sorted(values []string) []string {
	out := append([]string(nil), values...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
