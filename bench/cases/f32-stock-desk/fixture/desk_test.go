package stockdesk

import "testing"

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestFulfillment(t *testing.T) {
	d := NewDesk()
	must(t, d.Receive("book", 20))
	must(t, d.Reserve("a", "book", 6))
	r, err := d.Dispatch("a", "one", 2)
	must(t, err)
	if r.Quantity != 2 || r.Cycle != 1 {
		t.Fatal(r)
	}
	r2, err := d.Dispatch("a", "one", 2)
	must(t, err)
	if r2 != r {
		t.Fatal(r2)
	}
	_, err = d.Dispatch("a", "one", 3)
	if err == nil {
		t.Fatal("expected conflict")
	}
	_, err = d.Dispatch("a", "two", 4)
	must(t, err)
	row, err := d.Order("a")
	must(t, err)
	if row.State != "closed" || row.Shipped != 6 {
		t.Fatal(row)
	}
	must(t, d.Reopen("a", 3))
	_, err = d.Dispatch("a", "three", 3)
	must(t, err)
	if d.Available("book") != 11 {
		t.Fatal(d.Summary())
	}
	if len(d.Shipments()) != 3 {
		t.Fatal(d.Shipments())
	}
	if d.ActionTotals()["dispatch"] != 9 {
		t.Fatal(d.ActionTotals())
	}
}

func TestReleaseAndReports(t *testing.T) {
	d := NewDesk()
	must(t, d.Receive("book", 10))
	must(t, d.Reserve("b", "book", 3))
	must(t, d.Reserve("a", "book", 2))
	s := d.Summary()
	if s.Reserved != 5 || s.OpenOrders != 2 || s.Available["book"] != 5 {
		t.Fatal(s)
	}
	s.Available["book"] = 99
	if d.Available("book") != 5 {
		t.Fatal("shared map")
	}
	must(t, d.Release("b"))
	if d.Available("book") != 8 {
		t.Fatal(d.Summary())
	}
	if d.ListOrders()[0].ID != "a" {
		t.Fatal(d.ListOrders())
	}
	if len(d.Events(2)) != 2 {
		t.Fatal(d.Events(2))
	}
	events := d.Events(0)
	events[0].Quantity = 999
	if d.Events(0)[0].Quantity != 10 {
		t.Fatal("shared journal")
	}
	if d.Release("b") == nil {
		t.Fatal("closed release")
	}
	if d.Reopen("a", 2) == nil {
		t.Fatal("open reopen")
	}
}

func TestValidation(t *testing.T) {
	d := NewDesk()
	if d.Receive("", 2) == nil || d.Receive("x", 0) == nil {
		t.Fatal("invalid receive")
	}
	if d.Reserve("a", "x", 1) == nil {
		t.Fatal("stock check")
	}
	must(t, d.Receive("x", 5))
	must(t, d.Reserve("a", "x", 2))
	before := d.Summary()
	if d.Reserve("a", "x", 1) == nil {
		t.Fatal("duplicate")
	}
	if d.Reserve("", "x", 1) == nil {
		t.Fatal("invalid id")
	}
	if d.Reserve("b", "x", -1) == nil {
		t.Fatal("invalid quantity")
	}
	if _, err := d.Dispatch("missing", "r", 1); err == nil {
		t.Fatal("missing order")
	}
	if _, err := d.Dispatch("a", "", 1); err == nil {
		t.Fatal("invalid request")
	}
	if _, err := d.Dispatch("a", "r", 3); err == nil {
		t.Fatal("excess quantity")
	}
	if _, err := d.Dispatch("a", "r", 0); err == nil {
		t.Fatal("invalid quantity")
	}
	if d.Release("missing") == nil || d.Reopen("missing", 1) == nil {
		t.Fatal("missing")
	}
	if after := d.Summary(); after.Events != before.Events || after.Available["x"] != before.Available["x"] {
		t.Fatal(after)
	}
}

func TestComponents(t *testing.T) {
	stock := NewInventory()
	must(t, stock.Receive("x", 4))
	must(t, stock.Take("x", 2))
	if stock.Take("x", 0) == nil || stock.Take("x", 3) == nil {
		t.Fatal("invalid take")
	}
	orders := NewOrders()
	row := Order{ID: "a", SKU: "x", Quantity: 2, Remaining: 2, Cycle: 1, State: "open"}
	must(t, orders.Add(row))
	row.Remaining = 1
	row.Shipped = 1
	must(t, orders.Save(row))
	if orders.Add(row) == nil {
		t.Fatal("duplicate")
	}
	row.ID = "b"
	if orders.Save(row) == nil {
		t.Fatal("missing")
	}
	row.ID = "a"
	row.Remaining = -1
	if orders.Save(row) == nil {
		t.Fatal("invalid")
	}
	row.Remaining = 2
	row.Shipped = 1
	if row.Validate() == nil {
		t.Fatal("excess")
	}
	receipts := NewReceipts()
	receipt := Shipment{OrderID: "a", Quantity: 1, Cycle: 1}
	must(t, receipts.Put("a/1/r", receipt))
	must(t, receipts.Put("a/2/r", Shipment{OrderID: "a", Quantity: 1, Cycle: 2}))
	if receipts.Count() != 2 {
		t.Fatal(receipts.List())
	}
	if receipts.Put("a/1/r", receipt) == nil || receipts.Put("", receipt) == nil {
		t.Fatal("invalid receipt")
	}
	if got, ok := receipts.Find("a/1/r"); !ok || got != receipt {
		t.Fatal(got)
	}
}
