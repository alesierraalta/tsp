package stockdesk

import "fmt"

func (d *Desk) Release(id string) error {
	row, err := d.orders.Get(id)
	if err != nil {
		return err
	}
	if row.State != "open" {
		return fmt.Errorf("order must be open")
	}
	returned := row.Quantity
	if err := d.stock.Receive(row.SKU, returned); err != nil {
		return err
	}
	row.Remaining = 0
	row.State = "closed"
	if err := d.orders.Save(row); err != nil {
		return err
	}
	d.journal.Append("release", id, returned)
	return nil
}
