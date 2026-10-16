package sequence

type State struct {
	Values   []int
	Issued   int
	Reserved int
}
type Store struct{ Primary, Replica State }
type Sequence struct{ Store *Store }

func clone(s State) State        { s.Values = append([]int(nil), s.Values...); return s }
func New(store *Store) *Sequence { return &Sequence{Store: store} }
func (s *Sequence) Next() int {
	p := &s.Store.Primary
	p.Issued++
	if p.Issued > p.Reserved {
		p.Reserved = p.Issued + 31
	}
	p.Values = append(p.Values, p.Issued)
	return p.Issued
}
func (s *Sequence) Replicate() { s.Store.Replica = clone(s.Store.Primary) }
func (s *Sequence) Promote() {
	s.Store.Primary = clone(s.Store.Replica)
}
func (s *Sequence) Recover() {
	s.Store.Primary.Issued = s.Store.Primary.Reserved
}
func (s *Sequence) Values() []int { return append([]int(nil), s.Store.Primary.Values...) }
