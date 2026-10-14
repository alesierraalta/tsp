package stockdesk

import "fmt"

type Order struct {
	ID        string
	SKU       string
	Quantity  int
	Remaining int
	Shipped   int
	Cycle     int
	State     string
}

type Shipment struct {
	OrderID  string
	Quantity int
	Cycle    int
}

func positive(value int) error {
	if value <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	return nil
}

func validID(value string) error {
	if value == "" {
		return fmt.Errorf("id required")
	}
	return nil
}

func (o Order) Validate() error {
	if err := validID(o.ID); err != nil {
		return err
	}
	if err := validID(o.SKU); err != nil {
		return err
	}
	if err := positive(o.Quantity); err != nil {
		return err
	}
	if o.Remaining < 0 || o.Shipped < 0 {
		return fmt.Errorf("negative units")
	}
	if o.Remaining+o.Shipped > o.Quantity {
		return fmt.Errorf("excess units")
	}
	return nil
}
