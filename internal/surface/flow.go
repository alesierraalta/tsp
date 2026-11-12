package surface

import "strings"

// taint is one input the function reads, and every name it was assigned to inside the function.
type taint struct {
	label  string
	origin string
	first  int
	// values are the names that carry the input's value, which is what a guard has to mention. A status
	// result reached through a call is tainted for the sinks it feeds, but a check on it says the call
	// failed, not that the input was examined, so it is bound without landing here.
	values map[string]bool
}

// guard is one check seen in the function and the names its condition mentions.
type guard struct {
	line  int
	text  string
	names map[string]bool
}

// flow is the state one function's inventory shares across languages: the inputs it reads, the guards that
// stand in the way, and the rows they produce. An extractor owns how a language is read; this owns the books,
// so a row means the same thing whether it came from go/ast or from a lexer.
type flow struct {
	path         string
	function     string
	functionLine int
	taints       map[string]*taint
	guards       []guard
	rows         []Row
}

// newFlow opens the books for one function.
func newFlow(path, function string, line int) *flow {
	return &flow{path: path, function: function, functionLine: line, taints: map[string]*taint{}}
}

// newTaint records one input.
func (f *flow) newTaint(label, origin string, line int) *taint {
	return &taint{label: label, origin: origin, first: line, values: map[string]bool{}}
}

// bind says that name now carries the taint, which is how a derived value keeps its origin.
func (f *flow) bind(name string, t *taint) {
	if name == "" || name == "_" {
		return
	}
	f.taints[name] = t
	t.values[name] = true
}

// bindVerdict says that name carries a value computed from the taint but is not the value itself: the error
// one call returns about work it did with the input, or the status a check assigns. Sinks on it are still
// reported; a guard on it is not, because `if err != nil` says the call failed, not that the input is safe.
func (f *flow) bindVerdict(name string, t *taint) {
	if name == "" || name == "_" {
		return
	}
	f.taints[name] = t
}

// add records one row, with the guard that stands between the input's first use and this line.
func (f *flow) add(t *taint, kind, call string, line int) {
	seen := f.guardFor(t, line)
	f.rows = append(f.rows, Row{
		File:         f.path,
		Function:     f.function,
		FunctionLine: f.functionLine,
		Input:        t.label,
		Origin:       t.origin,
		Sink:         kind,
		SinkCall:     call,
		SinkLine:     line,
		Guard:        seen,
		Guarded:      seen != "",
	})
}

// guardFor returns the guard text between an input's first use and a sink, preferring the closest one, or
// an empty string when nothing between them mentions the value.
func (f *flow) guardFor(t *taint, line int) string {
	text, at := "", -1
	for _, g := range f.guards {
		if g.line < t.first || g.line > line || g.line < at {
			continue
		}
		if !shares(g.names, t.values) {
			continue
		}
		text, at = g.text, g.line
	}
	return text
}

// shares reports whether two name sets meet.
func shares(a, b map[string]bool) bool {
	for name := range a {
		if b[name] {
			return true
		}
	}
	return false
}

// has reports whether a chain contains a segment.
func has(segments []string, want string) bool {
	for _, segment := range segments {
		if segment == want {
			return true
		}
	}
	return false
}

// moneyWords name a total whose arithmetic is money: cents, prices, balances. Arithmetic on one of these is
// the sink a rounding or a truncation defect hides in, in every language.
var moneyWords = []string{"amount", "price", "total", "sum", "balance", "cost", "fee", "cents", "money", "budget", "charge", "credit", "debit"}

// moneyLike reports a name that holds money by the word it is written with.
func moneyLike(name string) bool {
	lower := strings.ToLower(name)
	for _, word := range moneyWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// verdictNames are the names a function gives a call's outcome instead of the value it produced. A check on
// one of them says the call failed, so it does not stand between an input and a sink.
var verdictNames = map[string]bool{"err": true, "error": true, "ok": true, "exists": true, "found": true, "failed": true, "success": true}

// isVerdictName reports a name convention says holds a call's outcome.
func isVerdictName(name string) bool { return verdictNames[name] }

// handleNames are the parameters that carry a runtime object instead of data: a request, a response, a
// database session, a logger. An attacker cannot send one across the boundary, so they are not inputs; what a
// request is asked for (`req.query`, `request.args`) is, and the extractors name those separately.
var handleNames = map[string]bool{
	"req": true, "request": true, "res": true, "response": true, "reply": true, "next": true,
	"ctx": true, "context": true, "self": true, "cls": true, "db": true, "conn": true, "connection": true,
	"cursor": true, "session": true, "tx": true, "logger": true, "log": true, "cache": true,
}

// handleTypes are the parameter type names that say the same thing in TypeScript or a Python annotation.
var handleTypes = []string{
	"Request", "Response", "Context", "Session", "Client", "Connection", "Cursor", "Logger", "Pool",
	"FastifyRequest", "FastifyReply", "IncomingMessage", "ServerResponse",
}

// isHandle reports a parameter that carries a runtime handle instead of data. A type the language cannot
// resolve is not guessed at: only a written type name counts.
func isHandle(name, typeName string) bool {
	if handleNames[name] || isVerdictName(name) {
		return true
	}
	for _, handle := range handleTypes {
		if typeName == handle || typeName == "*"+handle || typeName == handle+"?" {
			return true
		}
	}
	return false
}
