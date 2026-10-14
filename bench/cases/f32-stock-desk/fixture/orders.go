package stockdesk

import "fmt"

type Orders struct{ rows map[string]Order }

func NewOrders() *Orders { return &Orders{rows: make(map[string]Order)} }

func (o *Orders) Add(row Order) error {
	if err := row.Validate(); err != nil {
		return err
	}
	if _, exists := o.rows[row.ID]; exists {
		return fmt.Errorf("duplicate order")
	}
	o.rows[row.ID] = row
	return nil
}

func (o *Orders) Get(id string) (Order, error) {
	row, ok := o.rows[id]
	if !ok {
		return Order{}, fmt.Errorf("order not found")
	}
	return row, nil
}

func (o *Orders) Save(row Order) error {
	if _, err := o.Get(row.ID); err != nil {
		return err
	}
	if err := row.Validate(); err != nil {
		return err
	}
	o.rows[row.ID] = row
	return nil
}

func (o *Orders) List() []Order {
	result := make([]Order, 0, len(o.rows))
	for _, row := range o.rows {
		result = append(result, row)
	}
	return result
}
