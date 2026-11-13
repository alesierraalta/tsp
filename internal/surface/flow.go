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
	line int
	text string
	// body is the decisive expression of the function the guard calls, when this package or file defines it:
	// what the check actually does, which is where a weak check gives itself away.
	body  string
	names map[string]bool
}

// builtValue is how a value was built: the returned sink a caller receives for it, and the call or literal that
// built it. A name with no entry holds the input itself, unchanged, which is not a returned sink.
type builtValue struct{ kind, call string }

// flow is the state one function's inventory shares across languages: the inputs it reads, the guards that
// stand in the way, and the rows they produce. An extractor owns how a language is read; this owns the books,
// so a row means the same thing whether it came from go/ast or from a lexer.
type flow struct {
	path         string
	function     string
	functionLine int
	taints       map[string]*taint
	guards       []guard
	// built records, per name that carries an input, how the value in it was built. It is what decides
	// whether handing the value back is a returned sink.
	built map[string]builtValue
	rows  []Row
}

// newFlow opens the books for one function.
func newFlow(path, function string, line int) *flow {
	return &flow{
		path:         path,
		function:     function,
		functionLine: line,
		taints:       map[string]*taint{},
		built:        map[string]builtValue{},
	}
}

// noteBuilt records how a name's value was built, which is what a returned row is about. A name that holds a
// call's verdict was not built by the call, so it never carries a returned sink.
func (f *flow) noteBuilt(name string, value builtValue) {
	if name == "" || name == "_" || isVerdictName(name) || value.kind == "" {
		return
	}
	f.built[name] = value
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

// add records one row, with the guard that stands between the input's first use and this line. A guard that
// calls a function the extractor could read is printed with that function's decisive expression after an
// arrow, because the sentence a reader has to attack is what the check does, not what it is called.
func (f *flow) add(t *taint, kind, call string, line int) {
	text, body := f.guardFor(t, call, line)
	f.addGuarded(t, kind, call, line, text, body)
}

// addGuarded records one row with a guard the caller already read: a row found one hop away carries the check
// the helper wrote, because that is where the input was examined.
func (f *flow) addGuarded(t *taint, kind, call string, line int, text, body string) {
	seen := text
	if body != "" {
		seen = text + " -> " + body
	}
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
		Guarded:      text != "",
		GuardBody:    body,
	})
}

// guardFor returns the guard text and the guard's own body between an input's first use and a sink, preferring
// the closest one, or two empty strings when nothing between them mentions the value. A check written on the
// sink's own line is not one of them: a case-folding comparison is the access decision this row is about, so
// the same expression cannot also be the check that makes the decision safe.
func (f *flow) guardFor(t *taint, call string, line int) (string, string) {
	text, body, at := "", "", -1
	for _, g := range f.guards {
		if g.line < t.first || g.line > line || g.line < at {
			continue
		}
		if g.line == line && isSinkExpression(g.text, call) {
			continue
		}
		if !shares(g.names, t.values) {
			continue
		}
		text, body, at = g.text, g.body, g.line
	}
	return text, body
}

// isSinkExpression reports a guard that is the sink's own call: the condition and the sink are one expression,
// so the check is the promise a reader has to attack rather than the answer to it. A lexer renders a guard with
// a space around every token, so the spaces are what makes the two spellings comparable.
func isSinkExpression(text, call string) bool {
	if call == "" {
		return false
	}
	squashed := strings.ReplaceAll(text, " ", "")
	return squashed == call || strings.HasPrefix(squashed, call+"(") || strings.HasPrefix(squashed, call+"[")
}

// addReturned records the row for an exported function handing a built value back to its caller.
func (f *flow) addReturned(t *taint, built builtValue, line int) {
	f.add(t, built.kind, built.call, line)
}

// taintsOf answers every distinct input the named values carry, in the order the names are written, so a value
// built from two parameters reports both of them. A name holding a call's verdict is not the input: returning
// `err` hands back what the call said, not the value it was given.
func taintsOf(names []string, taints map[string]*taint) []*taint {
	var found []*taint
	seen := map[*taint]bool{}
	for _, name := range names {
		if isVerdictName(name) {
			continue
		}
		t := taints[name]
		if t == nil || seen[t] {
			continue
		}
		seen[t] = true
		found = append(found, t)
	}
	return found
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
