package pager

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func TestClampLimit(t *testing.T) {
	cases := map[int]int{-5: DefaultLimit, 0: DefaultLimit, 1: 1, 50: 50, MaxLimit: MaxLimit, MaxLimit + 1: MaxLimit, 5000: MaxLimit}
	for limit, want := range cases {
		if got := ClampLimit(limit); got != want {
			t.Fatalf("ClampLimit(%d) = %d, want %d", limit, got, want)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cursor := Cursor{CreatedAt: now, ID: "order-1001"}
	token, err := Encode(cursor)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(token)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !got.CreatedAt.Equal(now) || got.ID != cursor.ID {
		t.Fatalf("Decode = %+v, want %+v", got, cursor)
	}
}

func TestEncodeRefusesACursorWithoutAnID(t *testing.T) {
	for _, cursor := range []Cursor{
		{CreatedAt: now},
		{CreatedAt: now, ID: "order 1001"},
		{CreatedAt: now, ID: "order/1001"},
		{CreatedAt: time.Time{}, ID: "order-1001"},
		{CreatedAt: now, ID: string(make([]byte, MaxID+1))},
	} {
		if _, err := Encode(cursor); !errors.Is(err, ErrToken) {
			t.Fatalf("Encode(%+v) = %v, want ErrToken", cursor, err)
		}
	}
}

func TestDecodeRefusesTokensItCannotRead(t *testing.T) {
	good, err := Encode(Cursor{CreatedAt: now, ID: "order-1001"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for _, token := range []string{
		"",
		"not base64!!",
		"bm90LWEtY3Vyc29y",
		"MjAyNi0wMy0wMQ==",
	} {
		if _, err := Decode(token); !errors.Is(err, ErrToken) {
			t.Fatalf("Decode(%q) = %v, want ErrToken", token, err)
		}
	}
	if _, err := Decode(good + "AAAA"); err == nil {
		// Appending to a valid token either breaks base64 or the payload shape; both are refused.
		t.Fatalf("Decode accepted a token with trailing bytes: %q", good+"AAAA")
	}
	long := make([]byte, MaxToken+1)
	for i := range long {
		long[i] = 'A'
	}
	if _, err := Decode(string(long)); !errors.Is(err, ErrToken) {
		t.Fatalf("Decode of an oversized token = %v, want ErrToken", err)
	}
}

func TestQueryWithoutCursorStartsAtTheBounds(t *testing.T) {
	bounds := Cursor{CreatedAt: now, ID: "order-1001"}
	page, err := Query(bounds, struct {
		Limit  int
		Cursor string
	}{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if page.Limit != 10 || page.After || page.Cursor != bounds {
		t.Fatalf("Query = %+v", page)
	}
}

func TestQueryContinuesAfterTheCursor(t *testing.T) {
	bounds := Cursor{CreatedAt: now, ID: "order-1001"}
	earlier := Cursor{CreatedAt: now.Add(-time.Hour), ID: "order-0900"}
	token, err := Encode(earlier)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	page, err := Query(bounds, struct {
		Limit  int
		Cursor string
	}{Limit: 10, Cursor: token})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !page.After || page.Cursor.ID != earlier.ID {
		t.Fatalf("Query = %+v", page)
	}
}

func TestQueryRefusesACursorPastTheBounds(t *testing.T) {
	bounds := Cursor{CreatedAt: now, ID: "order-1001"}
	late := Cursor{CreatedAt: now.Add(6 * time.Hour), ID: "order-2000"}
	token, err := Encode(late)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := Query(bounds, struct {
		Limit  int
		Cursor string
	}{Limit: 10, Cursor: token}); !errors.Is(err, ErrRange) {
		t.Fatalf("Query past the bounds = %v, want ErrRange", err)
	}
}

func TestQueryRefusesAMalformedCursor(t *testing.T) {
	bounds := Cursor{CreatedAt: now, ID: "order-1001"}
	if _, err := Query(bounds, struct {
		Limit  int
		Cursor string
	}{Limit: 10, Cursor: "not-a-token"}); !errors.Is(err, ErrToken) {
		t.Fatalf("Query with a malformed cursor = %v, want ErrToken", err)
	}
}

func TestWindow(t *testing.T) {
	width, err := Window(now, now.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if width != 90*time.Minute {
		t.Fatalf("Window = %v", width)
	}
	if _, err := Window(now, now); !errors.Is(err, ErrRange) {
		t.Fatalf("Window of an empty range = %v, want ErrRange", err)
	}
	if _, err := Window(now, now.Add(-time.Minute)); !errors.Is(err, ErrRange) {
		t.Fatalf("Window of a backward range = %v, want ErrRange", err)
	}
}
