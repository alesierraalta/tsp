package transfer

import (
	"bytes"
	"reflect"
	"testing"
)

func TestFetch(t *testing.T) {
	calls := 0
	got := Fetch([]string{"a", "a", "b"}, func(key string) string {
		calls++
		return key + "!"
	})
	if !reflect.DeepEqual(got, []string{"a!", "a!", "b!"}) || calls != 2 {
		t.Fatalf("got %v with %d reads", got, calls)
	}
}

func TestSend(t *testing.T) {
	var out bytes.Buffer
	Send([]byte("abc"), 2, func(chunk []byte) { out.Write(chunk) })
	if out.String() != "abc" {
		t.Fatal(out.String())
	}
}
