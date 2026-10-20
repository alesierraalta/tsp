package backfill

import "errors"

type Options struct {
	Team    string
	Batch   int
	Preview bool
}

func (o Options) normalized() (Options, error) {
	if o.Batch < 0 {
		return o, errors.New("batch must be positive")
	}
	if o.Batch == 0 {
		o.Batch = 100
	}
	return o, nil
}

type Failure struct {
	ID      string
	Message string
}

type Report struct {
	Preview  bool
	Selected []string
	Cards    []Card
	Applied  int
	Failures []Failure
}

func (r *Report) record(p Profile, c Card) {
	r.Selected = append(r.Selected, p.ID)
	r.Cards = append(r.Cards, copyCard(c))
}

func (r *Report) failed(id string, err error) {
	r.Failures = append(r.Failures, Failure{ID: id, Message: err.Error()})
}
