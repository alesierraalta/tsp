package worker

import (
	"os"
	"sync"
	"testing"
)

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

func TestConcurrentOversellTrigger(t *testing.T) {
	if os.Getenv("TPP_INVENTORY_TRIGGER") != "1" {
		t.Skip("trigger probe")
	}

	const capacity = 50
	const buyers = 100
	ready := sync.WaitGroup{}
	ready.Add(buyers)
	release := make(chan struct{})
	inventory := NewInventory(capacity, func() {
		ready.Done()
		<-release
	})

	var finished sync.WaitGroup
	finished.Add(buyers)
	for range buyers {
		go func() {
			defer finished.Done()
			inventory.Buy()
		}()
	}

	ready.Wait()
	close(release)
	finished.Wait()

	sold := inventory.Sold()
	stock := inventory.Stock()
	t.Logf("sold=%d capacity=%d stock=%d", sold, capacity, stock)
	if sold > capacity {
		t.Errorf("sold %d exceeds capacity %d", sold, capacity)
	}
}
