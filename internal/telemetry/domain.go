package telemetry

// The domain event kinds: what the testing discipline itself did, as opposed to a command or a host
// run. Each is a pointer field of the same name on Event.
const (
	KindCheck   = "check"
	KindFinding = "finding"
	KindPlan    = "plan"
)

// Count renders a measured count as the pointer the schema uses, so a zero stays a measurement while
// a field nobody filled stays absent. A negative count is refused by becoming a zero: no measurement
// is ever negative.
func Count(n int) *int64 {
	if n < 0 {
		n = 0
	}
	value := int64(n)
	return &value
}

// Check is one verdict's content-free shape: the decision and its reason category, the runner the
// check used and the runners the changed languages imply, the changed languages, files and lines, and
// the witness counts. Every count is a pointer so absent means "not measured", never zero.
type Check struct {
	Decision    string   `json:"decision,omitempty"`
	Exit        int      `json:"exit"`
	Reason      string   `json:"reason,omitempty"`
	Suite       string   `json:"suite,omitempty"`
	Runners     []string `json:"runners,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	Files       *int64   `json:"files,omitempty"`
	Lines       *int64   `json:"lines,omitempty"`
	TestFiles   *int64   `json:"test_files,omitempty"`
	Red         *int64   `json:"red,omitempty"`
	PinsNothing *int64   `json:"pins_nothing,omitempty"`
	NewCode     *int64   `json:"new_code,omitempty"`
	Waiver      bool     `json:"waiver,omitempty"`
	OptOut      bool     `json:"opt_out,omitempty"`
}

// Finding is one findings command's event: the action it ran, the defect class it declared, the
// verdict it recorded, and how many findings it touched. Free text is never carried.
type Finding struct {
	Action  string `json:"action,omitempty"`
	Class   string `json:"class,omitempty"`
	Verdict string `json:"verdict,omitempty"`
	Count   *int64 `json:"count,omitempty"`
}

// Plan is one plan command's event: the action it ran and the row/problem counts it could read from
// the document. A count that could not be read stays absent.
type Plan struct {
	Action   string `json:"action,omitempty"`
	Findings *int64 `json:"findings,omitempty"`
	Evidence *int64 `json:"evidence,omitempty"`
	Problems *int64 `json:"problems,omitempty"`
}
