package batchdec

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseHeaderReadsTheCounts(t *testing.T) {
	meta, err := ParseHeader("v1 cols=3 rows=2")
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if meta.Version != 1 || meta.Columns != 3 || meta.Declared != 2 {
		t.Fatalf("ParseHeader = %+v", meta)
	}
}

func TestParseHeaderRefusesTooManyColumns(t *testing.T) {
	if _, err := ParseHeader("v1 cols=65 rows=1"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("ParseHeader of cols=65 = %v, want ErrTooMany", err)
	}
}

func TestParseHeaderRefusesTooManyDeclaredRows(t *testing.T) {
	if _, err := ParseHeader("v1 cols=2 rows=100001"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("ParseHeader of rows=100001 = %v, want ErrTooMany", err)
	}
}

func TestParseHeaderAtTheDeclaredLimit(t *testing.T) {
	if _, err := ParseHeader("v1 cols=2 rows=100000"); err != nil {
		t.Fatalf("ParseHeader at the limit: %v", err)
	}
}

func TestParseHeaderRefusesAFormatItCannotRead(t *testing.T) {
	for _, line := range []string{"", "cols=2 rows=1", "v2 cols=2 rows=1", "v1 cols=2 rows=-1", "v1 cols", "v1 cols=two"} {
		if _, err := ParseHeader(line); !errors.Is(err, ErrFormat) {
			t.Fatalf("ParseHeader(%q) = %v, want ErrFormat", line, err)
		}
	}
}

func TestParseRowReadsALabelAndAValue(t *testing.T) {
	row, repeat, err := ParseRow("shirt|7")
	if err != nil {
		t.Fatalf("ParseRow: %v", err)
	}
	if row != (Row{Label: "shirt", Value: 7}) || repeat != 1 {
		t.Fatalf("ParseRow = %+v repeat %d", row, repeat)
	}
}

func TestParseRowReadsARepeatFactor(t *testing.T) {
	_, repeat, err := ParseRow("socks|3 x4")
	if err != nil {
		t.Fatalf("ParseRow: %v", err)
	}
	if repeat != 4 {
		t.Fatalf("repeat = %d, want 4", repeat)
	}
}

func TestParseRowRefusesAFormatItCannotRead(t *testing.T) {
	for _, line := range []string{"shirt", "|7", "shirt|seven", "shirt|7 x0", "shirt|7 xlast"} {
		if _, _, err := ParseRow(line); !errors.Is(err, ErrFormat) {
			t.Fatalf("ParseRow(%q) = %v, want ErrFormat", line, err)
		}
	}
}

func TestDecodeExpandsARepeatFactor(t *testing.T) {
	rows, err := Decode("v1 cols=2 rows=2\nshirt|7\nsocks|3 x3")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := []Row{{Label: "shirt", Value: 7}, {Label: "socks", Value: 3}, {Label: "socks", Value: 3}, {Label: "socks", Value: 3}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("Decode = %+v", rows)
	}
}

func TestDecodeRefusesAHeaderCountThatDisagreesWithTheBlock(t *testing.T) {
	if _, err := Decode("v1 cols=2 rows=3\nshirt|7\nsocks|3"); !errors.Is(err, ErrFormat) {
		t.Fatalf("Decode with a short block = %v, want ErrFormat", err)
	}
}

func TestDecodeRefusesAnEmptyBody(t *testing.T) {
	if _, err := Decode(""); !errors.Is(err, ErrFormat) {
		t.Fatalf("Decode of an empty body = %v, want ErrFormat", err)
	}
}

func TestDecodeAcceptsCarriageReturns(t *testing.T) {
	rows, err := Decode("v1 cols=2 rows=1\r\nshirt|7\r\n")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Decode = %+v", rows)
	}
}

func TestEncodeRoundTripsADecodedBlock(t *testing.T) {
	block := "v1 cols=2 rows=2\nshirt|7\nsocks|3 x2\n"
	rows, err := Decode(block)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := Encode(rows); got != block {
		t.Fatalf("Encode = %q, want %q", got, block)
	}
}

func TestEncodeWritesAnEmptyBlock(t *testing.T) {
	if got := Encode(nil); got != "v1 cols=2 rows=0\n" {
		t.Fatalf("Encode(nil) = %q", got)
	}
}

func TestSummaryTotalsPerLabelInOrder(t *testing.T) {
	rows, err := Decode("v1 cols=2 rows=3\nsocks|3\nshirt|7\nsocks|4")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := []LabelTotal{{Label: "shirt", Value: 7}, {Label: "socks", Value: 7}}
	if got := Summary(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("Summary = %+v", got)
	}
}
