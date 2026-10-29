package plan

import "testing"

// A JSON object with the same key twice has no single meaning, and encoding/json settles it by
// keeping the last one. That turns a merge that left both sides' gate into a live policy change:
// {"gate":"always","gate":"never"} reads as never and silences the gate without a word. A
// declaration the check cannot read has one answer, and it is an error, not a value.
func TestDeclarationRefusesTheSameGateKeyTwice(t *testing.T) {
	_, err := DeclaredGate(t.TempDir(), declares(`{"gate":"always","gate":"never"}`))
	if err == nil {
		t.Fatal("a declaration that says both must be refused, not resolved to the last one")
	}
}
