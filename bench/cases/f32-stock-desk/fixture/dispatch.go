package stockdesk

import "fmt"

func (d *Desk) Dispatch(id, request string, quantity int) (Shipment, error) {
	row, err := d.orders.Get(id)
	if err != nil {
		return Shipment{}, err
	}
	if err := validID(request); err != nil {
		return Shipment{}, err
	}
	key := fmt.Sprintf("%q/%q", id, request)
	if receipt, exists := d.receipts.Find(key); exists {
		if receipt.Quantity != quantity {
			return Shipment{}, fmt.Errorf("request conflict")
		}
		return receipt, nil
	}
	if row.State != "open" {
		return Shipment{}, fmt.Errorf("order must be open")
	}
	if err := positive(quantity); err != nil {
		return Shipment{}, err
	}
	if quantity > row.Remaining {
		return Shipment{}, fmt.Errorf("exceeds reservation")
	}
	receipt := Shipment{OrderID: id, Quantity: quantity, Cycle: row.Cycle}
	row.Remaining -= quantity
	row.Shipped += quantity
	if row.Remaining == 0 {
		row.State = "closed"
	}
	if err := d.orders.Save(row); err != nil {
		return Shipment{}, err
	}
	if err := d.receipts.Put(key, receipt); err != nil {
		return Shipment{}, err
	}
	d.journal.Append("dispatch", id, quantity)
	return receipt, nil
}
