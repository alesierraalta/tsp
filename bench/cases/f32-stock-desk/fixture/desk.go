package stockdesk

import "fmt"

type Desk struct {
	stock    *Inventory
	orders   *Orders
	receipts *Receipts
	journal  *Journal
}

func NewDesk() *Desk {
	return &Desk{NewInventory(), NewOrders(), NewReceipts(), &Journal{}}
}

func (d *Desk) Receive(sku string, quantity int) error {
	if err := d.stock.Receive(sku, quantity); err != nil {
		return err
	}
	d.journal.Append("receive", sku, quantity)
	return nil
}

func (d *Desk) Reserve(id, sku string, quantity int) error {
	row := Order{ID: id, SKU: sku, Quantity: quantity, Remaining: quantity, Cycle: 1, State: "open"}
	if err := row.Validate(); err != nil {
		return err
	}
	if _, err := d.orders.Get(id); err == nil {
		return fmt.Errorf("duplicate order")
	}
	if err := d.stock.Take(sku, quantity); err != nil {
		return err
	}
	if err := d.orders.Add(row); err != nil {
		return err
	}
	d.journal.Append("reserve", id, quantity)
	return nil
}

func (d *Desk) Available(sku string) int       { return d.stock.Available(sku) }
func (d *Desk) Order(id string) (Order, error) { return d.orders.Get(id) }
func (d *Desk) Events(sequence int) []Event    { return d.journal.Since(sequence) }
func (d *Desk) Shipments() []Shipment          { return d.receipts.List() }

func (d *Desk) Reopen(id string, quantity int) error {
	row, err := d.orders.Get(id)
	if err != nil {
		return err
	}
	if row.State != "closed" {
		return fmt.Errorf("order must be closed")
	}
	if err := positive(quantity); err != nil {
		return err
	}
	if err := d.stock.Take(row.SKU, quantity); err != nil {
		return err
	}
	row.Quantity, row.Remaining, row.Shipped = quantity, quantity, 0
	row.Cycle++
	row.State = "open"
	if err := d.orders.Save(row); err != nil {
		return err
	}
	d.journal.Append("reopen", id, quantity)
	return nil
}
