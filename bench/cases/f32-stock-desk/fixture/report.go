package stockdesk

import "sort"

type Summary struct {
	Available     map[string]int
	OpenOrders    int
	Reserved      int
	ShipmentCount int
	Events        int
}

func (d *Desk) Summary() Summary {
	result := Summary{Available: d.stock.Snapshot(), ShipmentCount: d.receipts.Count(), Events: d.journal.Len()}
	for _, row := range d.orders.List() {
		if row.State == "open" {
			result.OpenOrders++
			result.Reserved += row.Remaining
		}
	}
	return result
}

func (d *Desk) ListOrders() []Order {
	rows := d.orders.List()
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func (d *Desk) ActionTotals() map[string]int { return d.journal.Totals() }
