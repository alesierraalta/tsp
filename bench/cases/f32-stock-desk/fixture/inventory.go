package stockdesk

import "fmt"

type Inventory struct{ available map[string]int }

func NewInventory() *Inventory {
	return &Inventory{available: make(map[string]int)}
}

func (i *Inventory) Receive(sku string, quantity int) error {
	if err := validID(sku); err != nil {
		return err
	}
	if err := positive(quantity); err != nil {
		return err
	}
	i.available[sku] += quantity
	return nil
}

func (i *Inventory) Take(sku string, quantity int) error {
	if err := positive(quantity); err != nil {
		return err
	}
	if i.available[sku] < quantity {
		return fmt.Errorf("insufficient stock")
	}
	i.available[sku] -= quantity
	return nil
}

func (i *Inventory) Available(sku string) int {
	return i.available[sku]
}

func (i *Inventory) Snapshot() map[string]int {
	result := make(map[string]int, len(i.available))
	for sku, quantity := range i.available {
		result[sku] = quantity
	}
	return result
}
