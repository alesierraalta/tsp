package main

import "testing"

func TestCommands(t *testing.T) {
	if err := run([]string{"receive:x:10", "reserve:a:x:2", "dispatch:a:r:2", "reopen:a:1", "release:a", "balance:x", "order:a"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"other"}, {"reserve"}, {"dispatch:a:r:no"}, {"receive:x:no"}, {"release"}, {"balance"}, {"order"}, {"reopen:missing:1"}} {
		if run(args) == nil {
			t.Fatal(args)
		}
	}
}
