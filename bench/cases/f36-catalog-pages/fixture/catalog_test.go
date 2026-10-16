package catalog

import (
	"strings"
	"testing"
)

func TestCatalogWorkflow(t *testing.T) {
	s := NewStore()
	a, err := NewRecord("a", "Alpha", "team")
	if err != nil {
		t.Fatal(err)
	}
	a = a.WithTag("active").WithTag("active")
	if err := s.Put(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ID: "b", Title: "Beta"}); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 2 || !a.HasTag("active") || len(a.Tags) != 1 {
		t.Fatal("records")
	}
	copy, _ := s.Get("a")
	copy.Tags[0] = "changed"
	saved, _ := s.Get("a")
	if !saved.HasTag("active") {
		t.Fatal("copy")
	}
	x := BuildIndex(s)
	if x.First() != "a" || x.Count() != 2 || !x.Contains("b") || x.Contains("z") {
		t.Fatal("index")
	}
	if len(x.From("b")) != 1 || len(x.Prefix("a")) != 1 {
		t.Fatal("range")
	}
	p, err := List(s, "", 10)
	if err != nil || p.Empty() || p.Next != "" || strings.Join(p.IDs(), ",") != "a,b" {
		t.Fatal(p, err)
	}
	records := Search(s, Filter{Group: "team", Tag: "active", Query: "ALP"})
	if len(records) != 1 || len(Search(s, Filter{Tag: "none"})) != 0 || len(Search(s, Filter{Query: "zzz"})) != 0 {
		t.Fatal("search")
	}
	if Groups(Snapshot(s))["ungrouped"] != 1 {
		t.Fatal("groups")
	}
	if JoinLabels(records) != "a: Alpha" {
		t.Fatal("labels")
	}
	label, err := Label("  Alphabet  ", 5)
	if err != nil || label != "Alpha" {
		t.Fatal(label, err)
	}
	if label, _ := Label("short", 10); label != "short" {
		t.Fatal(label)
	}
	data, err := ExportJSON(Snapshot(s))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(data)
	if err != nil || loaded.Len() != 2 {
		t.Fatal(err)
	}
	csv, err := ExportCSV(Snapshot(s))
	if err != nil || !strings.Contains(string(csv), "a,Alpha,team") {
		t.Fatal(err)
	}
	if !s.Delete("a") || s.Delete("a") {
		t.Fatal("delete")
	}
}

func TestValidation(t *testing.T) {
	if _, err := NewRecord("", "title", ""); err == nil {
		t.Fatal("id")
	}
	if _, err := NewRecord("a", " ", ""); err == nil {
		t.Fatal("title")
	}
	if err := NewStore().Put(Record{}); err == nil {
		t.Fatal("put")
	}
	if _, err := List(NewStore(), "", 0); err == nil {
		t.Fatal("limit")
	}
	if _, err := Label("a", -1); err == nil {
		t.Fatal("budget")
	}
	if _, err := Label(string([]byte{255}), 1); err == nil {
		t.Fatal("encoding")
	}
	if _, err := Load([]byte("?")); err == nil {
		t.Fatal("json")
	}
	if _, err := Decode([]byte(`[{"ID":"a","Title":"A"},{"ID":"a","Title":"B"}]`)); err == nil {
		t.Fatal("duplicate")
	}
	if _, err := Decode([]byte(`[{"ID":"a"}]`)); err == nil {
		t.Fatal("record")
	}
	if data, _ := ExportJSON(nil); string(data) != "[]" {
		t.Fatal(string(data))
	}
	if BuildIndex(NewStore()).First() != "" {
		t.Fatal("empty")
	}
}
