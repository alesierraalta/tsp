package worker

// Sealed trigger probes: never copied into a workspace. Copy this file into the fixture (or a variant)
// to reproduce a planted defect; see KEY.json.

import (
	"os"
	"sync"
	"testing"
)

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

func TestRefundCapacityTrigger(t *testing.T) {
	if os.Getenv("TPP_INVENTORY_REFUND_TRIGGER") != "1" {
		t.Skip("trigger probe")
	}

	const capacity = 5
	inventory := NewInventory(capacity, nil)
	inventory.Buy()
	inventory.Refund(2)

	stock := inventory.Stock()
	t.Logf("actual: stock=%d capacity=%d", stock, capacity)
	if stock > capacity {
		t.Fail()
	}
}
