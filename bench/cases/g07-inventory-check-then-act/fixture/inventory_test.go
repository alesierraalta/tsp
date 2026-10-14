package worker

import "testing"

func TestSequentialPurchasesStayWithinStock(t *testing.T) {
	inventory := NewInventory(2, nil)
	if !inventory.Buy() || !inventory.Buy() || inventory.Buy() {
		t.Fatal("sequential purchases did not stop at capacity")
	}
	if got := inventory.Sold(); got != 2 {
		t.Fatalf("sold=%d, want 2", got)
	}
	if got := inventory.Stock(); got != 0 {
		t.Fatalf("stock=%d, want 0", got)
	}
}
