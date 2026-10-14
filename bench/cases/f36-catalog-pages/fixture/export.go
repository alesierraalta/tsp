package catalog

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
)

func ExportJSON(records []Record) ([]byte, error) {
	if records == nil {
		records = []Record{}
	}
	return json.Marshal(records)
}

func ExportCSV(records []Record) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"id", "title", "group"}); err != nil {
		return nil, err
	}
	for _, r := range records {
		if err := writer.Write([]string{r.ID, r.Title, r.Group}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func Snapshot(s *Store) []Record {
	records := make([]Record, 0, s.Len())
	for _, id := range s.IDs() {
		r, _ := s.Get(id)
		records = append(records, r)
	}
	return records
}
