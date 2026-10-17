package multiplexer

import "fmt"

// Frame is the shared wire format for both handler generations.
type Frame struct {
	Key   string
	Flags byte
}

const inspectFlag byte = 1 << 0

func Read(key string) Frame    { return Frame{Key: key} }
func Inspect(key string) Frame { return Frame{Key: key, Flags: inspectFlag} }
func Replicate(key string) Frame {
	return Frame{Key: key, Flags: replicaFlag}
}

// Data holds response values and counts of replication effects.
type Data struct {
	Values   map[string]string
	Replicas map[string]int
}

const replicaFlag byte = 0x01

// Handle executes a frame on an injected data store. Generation one supports
// reads and replication; generation two also supports inspection.
func Handle(generation int, frame Frame, data *Data) string {
	if generation >= 2 && frame.Flags&inspectFlag != 0 {
		return fmt.Sprintf("value=%s replicas=%d", data.Values[frame.Key], data.Replicas[frame.Key])
	}
	if frame.Flags&replicaFlag != 0 {
		data.Replicas[frame.Key]++
	}
	return data.Values[frame.Key]
}

// Transport emits exactly one ordered response for every submitted frame.
type Transport interface{ Send(Frame) }

type pending struct {
	id      uint64
	deliver func(string)
}

type Mux struct {
	transport Transport
	next      uint64
	pending   []pending
}

func New(transport Transport) *Mux { return &Mux{transport: transport} }

func (m *Mux) Submit(frame Frame, deliver func(string)) uint64 {
	m.next++
	m.pending = append(m.pending, pending{id: m.next, deliver: deliver})
	m.transport.Send(frame)
	return m.next
}

// Cancel stops delivery to the submitted callback.
func (m *Mux) Cancel(id uint64) {
	for i := range m.pending {
		if m.pending[i].id == id {
			m.pending[i].deliver = nil
			return
		}
	}
}

// Receive advances the transport by one response.
func (m *Mux) Receive(value string) {
	if len(m.pending) == 0 {
		return
	}
	p := m.pending[0]
	m.pending = m.pending[1:]
	if p.deliver != nil {
		p.deliver(value)
	}
}
