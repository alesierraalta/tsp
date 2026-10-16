package catalog

import (
	"encoding/json"
	"errors"
)

func Decode(data []byte) ([]Record, error) {
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, r := range records {
		if _, err := NewRecord(r.ID, r.Title, r.Group); err != nil {
			return nil, err
		}
		if seen[r.ID] {
			return nil, errors.New("duplicate id")
		}
		seen[r.ID] = true
	}
	return records, nil
}

func Load(data []byte) (*Store, error) {
	records, err := Decode(data)
	if err != nil {
		return nil, err
	}
	s := NewStore()
	for _, r := range records {
		if err := s.Put(r); err != nil {
			return nil, err
		}
	}
	return s, nil
}
