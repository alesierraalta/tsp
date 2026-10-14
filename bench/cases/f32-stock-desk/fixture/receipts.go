package stockdesk

import "fmt"

type Receipts struct{ entries map[string]Shipment }

func NewReceipts() *Receipts {
	return &Receipts{entries: make(map[string]Shipment)}
}

func (r *Receipts) Find(key string) (Shipment, bool) {
	receipt, ok := r.entries[key]
	return receipt, ok
}

func (r *Receipts) Put(key string, receipt Shipment) error {
	if err := validID(key); err != nil {
		return err
	}
	if _, exists := r.entries[key]; exists {
		return fmt.Errorf("duplicate receipt")
	}
	r.entries[key] = receipt
	return nil
}

func (r *Receipts) List() []Shipment {
	result := make([]Shipment, 0, len(r.entries))
	for _, receipt := range r.entries {
		result = append(result, receipt)
	}
	return result
}

func (r *Receipts) Count() int { return len(r.entries) }
