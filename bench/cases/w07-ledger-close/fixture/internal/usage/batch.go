package usage

// Batch is the set of events one ingest request carries. Its rules live with the decoders, because a
// batch is never valid or invalid independently of how it arrived.
type Batch struct {
	Events []Event
}

// Len reports how many events the batch carries.
func (b Batch) Len() int { return len(b.Events) }
