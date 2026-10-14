package stockdesk

type Event struct {
	Sequence int
	Action   string
	OrderID  string
	Quantity int
}

type Journal struct{ events []Event }

func (j *Journal) Append(action, id string, quantity int) {
	j.events = append(j.events, Event{
		Sequence: len(j.events) + 1, Action: action, OrderID: id, Quantity: quantity,
	})
}

func (j *Journal) Since(sequence int) []Event {
	result := make([]Event, 0)
	for _, event := range j.events {
		if event.Sequence > sequence {
			result = append(result, event)
		}
	}
	return result
}

func (j *Journal) Totals() map[string]int {
	totals := make(map[string]int)
	for _, event := range j.events {
		totals[event.Action] += event.Quantity
	}
	return totals
}

func (j *Journal) Len() int { return len(j.events) }
