package workqueue

import (
	"context"
	"sort"
)

type BatchResult struct {
	Values map[string]string
	Errors map[string]error
}

func (s *Service) FetchBatch(ctx context.Context, keys []string) BatchResult {
	pending := make(map[string]*Future)
	for _, key := range keys {
		if _, ok := pending[key]; !ok {
			pending[key] = s.Fetch(key)
		}
	}
	result := BatchResult{Values: make(map[string]string), Errors: make(map[string]error)}
	ordered := make([]string, 0, len(pending))
	for key := range pending {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		value, err := pending[key].Await(ctx)
		if err != nil {
			result.Errors[key] = err
		} else {
			result.Values[key] = value
		}
	}
	return result
}

func (r BatchResult) Keys() []string {
	keys := make([]string, 0, len(r.Values))
	for key := range r.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r BatchResult) OK() bool {
	return len(r.Errors) == 0
}
