package surface

import (
	"go/ast"
	"go/token"
	"strings"
)

// The sink kinds. The names are the vocabulary of the report and of the skill step that reads it, so a
// JavaScript or Python extractor adds rows of these kinds instead of inventing its own.
const (
	SinkFile         = "file"
	SinkExec         = "exec"
	SinkSQL          = "sql"
	SinkNetwork      = "network"
	SinkTemplate     = "template"
	SinkDecode       = "decode"
	SinkRegex        = "regex"
	SinkAllocation   = "allocation"
	SinkLoop         = "loop"
	SinkAccumulation = "accumulation"
	SinkCase         = "case"
	SinkTimezone     = "timezone"
)

// The returned-value kinds. The sink of a value an exported function builds and hands back is in the caller,
// which this probe does not follow, so the promise is reported where the value leaves the function and the kind
// says what the caller receives.
const (
	SinkReturnedPath   = "returned-path"
	SinkReturnedMarkup = "returned-markup"
	SinkReturnedQuery  = "returned-query"
	SinkReturnedURL    = "returned-url"
)

// sinkRule is one line of the catalog: a call the value must not reach, and which arguments carry it.
type sinkRule struct {
	Kind string
	// Names are the last segment of the callee: Open, Command, Query.
	Names []string
	// Stems are name prefixes that make a call the sink too, continued by nothing or by an upper-case
	// letter: a statement executor is written QueryText or ExecBatch as often as Query, while Execute is a
	// template and PrepareStatement a noun.
	Stems []string
	// Pkgs pins the rule to a package when the name alone is too common (Open, Parse, Get).
	Pkgs []string
	// From is the first argument index that carries the sensitive value; 1 skips a writer or format.
	From int
	// NotFirst names first arguments that mean this call is not the sink the rule describes, so a log
	// line written to stdout is not reported as an HTTP response.
	NotFirst []string
	// LocalWriter says the first argument has to be a stream outside this function: a call that writes into a
	// buffer the function declared builds a value here, and whatever that value reaches is the sink.
	LocalWriter bool
}

// sinkCatalog is the table other languages will reuse: a sink kind, the callee it answers to, and which
// argument carries the value. It is data on purpose — a reviewer can read it, and a second language adds its
// own rows without touching the flow analysis.
var sinkCatalog = []sinkRule{
	{Kind: SinkFile, Names: []string{"Open", "OpenFile", "Create", "ReadFile", "WriteFile", "Remove", "RemoveAll", "Mkdir", "MkdirAll", "Stat", "Lstat", "ReadDir", "Truncate", "Chmod", "Chown"}, Pkgs: []string{"os", "ioutil"}},
	{Kind: SinkExec, Names: []string{"Command", "CommandContext", "StartProcess"}, Pkgs: []string{"exec", "os"}},
	{Kind: SinkSQL, Names: []string{"Query", "QueryContext", "Exec", "ExecContext", "QueryRow", "QueryRowContext", "Prepare", "PrepareContext"}, Stems: []string{"Query", "Exec", "Prepare"}},
	{Kind: SinkNetwork, Names: []string{"Get", "Post", "PostForm", "NewRequest", "NewRequestWithContext", "Head", "Do"}, Pkgs: []string{"http"}},
	{Kind: SinkNetwork, Names: []string{"Dial", "DialTimeout", "DialTCP", "LookupHost"}, Pkgs: []string{"net"}},
	{Kind: SinkNetwork, Names: []string{"Parse", "ParseRequestURI", "ParseQuery"}, Pkgs: []string{"url"}},
	{Kind: SinkTemplate, Names: []string{"Execute", "ExecuteTemplate", "Parse", "ParseFiles", "ParseGlob"}, Pkgs: []string{"template", "html"}},
	{Kind: SinkTemplate, Names: []string{"WriteString"}, Pkgs: []string{"io"}, From: 1},
	{Kind: SinkTemplate, Names: []string{"Fprintf", "Fprint", "Fprintln"}, Pkgs: []string{"fmt"}, From: 1, NotFirst: []string{"os.Stdout", "os.Stderr", "os.Stdin", "stdout", "stderr", "log.Writer()"}, LocalWriter: true},
	{Kind: SinkDecode, Names: []string{"Unmarshal", "UnmarshalStrict", "Decode", "DecodeValue"}, Pkgs: []string{"json", "xml", "gob", "yaml", "toml"}},
	{Kind: SinkRegex, Names: []string{"Compile", "MustCompile", "CompilePOSIX", "MustCompilePOSIX"}, Pkgs: []string{"regexp"}},
	{Kind: SinkAllocation, Names: []string{"make", "new"}, From: 1},
	{Kind: SinkTimezone, Names: []string{"Parse", "ParseInLocation", "LoadLocation", "LoadLocationFromTZData", "In", "Date", "FixedZone"}, Pkgs: []string{"time"}},
}

// matches reports whether a call answers to this rule.
func (r sinkRule) matches(segments []string, call *ast.CallExpr, text func(ast.Expr) string) bool {
	last := segments[len(segments)-1]
	if !anyOf(last, r.Names) && !byStem(last, r.Stems) {
		return false
	}
	if len(r.Pkgs) > 0 && !anyOf(segments[0], r.Pkgs) {
		return false
	}
	if len(r.NotFirst) > 0 && len(call.Args) > 0 {
		first := text(call.Args[0])
		for _, excluded := range r.NotFirst {
			if first == excluded {
				return false
			}
		}
	}
	if r.LocalWriter && len(call.Args) > 0 && isLocalAddress(call.Args[0]) {
		return false
	}
	if r.From >= len(call.Args) {
		return false
	}
	return true
}

// byStem reports a name a stem introduces: the stem itself, or the stem and a CamelCase word after it, which is
// how a method that runs a statement is written. A stem followed by a lower-case letter is a longer word and
// not this call: `Execute` is a template, not a statement.
func byStem(name string, stems []string) bool {
	for _, stem := range stems {
		if !strings.HasPrefix(name, stem) {
			continue
		}
		rest := name[len(stem):]
		if rest == "" || (rest[0] >= 'A' && rest[0] <= 'Z') {
			return true
		}
	}
	return false
}

// isLocalAddress reports &name: the address of something this function builds, not a stream its caller chose.
func isLocalAddress(expr ast.Expr) bool {
	unary, ok := expr.(*ast.UnaryExpr)
	return ok && unary.Op == token.AND
}

// validatorPrefixes name the calls that clean a value: a function whose name starts with one of these is a
// guard a reviewer can argue with.
var validatorPrefixes = []string{"validate", "sanitize", "sanitise", "escape", "quote", "allow", "check", "verify"}

// validatorNames are the library calls that bound, escape or parse a value into something safe.
var validatorNames = []string{"Atoi", "ParseInt", "ParseUint", "ParseFloat", "QuoteMeta", "EscapeString", "HTMLEscapeString", "JSEscapeString", "URLQueryEscape", "PathEscape", "min"}

// isValidator reports a call that cleans a value.
func isValidator(name string) bool {
	for _, exact := range validatorNames {
		if name == exact {
			return true
		}
	}
	lower := strings.ToLower(name)
	for _, prefix := range validatorPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// isCaseCall reports the string calls that fold case or compare without it: the sink behind a locale or
// Unicode assumption in an access check.
func isCaseCall(name string) bool {
	switch name {
	case "ToLower", "ToUpper", "Title", "ToTitle", "EqualFold", "ToLowerSpecial", "ToUpperSpecial":
		return true
	}
	return false
}

// parseNames are the calls whose error is a verdict on the value they were handed: the parse rejected it or
// it did not. They are not validators — a parsed URL is still attacker-shaped — but the error they return
// belongs to the input, so a check on it is a check on the input.
var parseNames = []string{"Parse", "ParseRequestURI", "ParseQuery", "ParseForm", "Unmarshal", "UnmarshalStrict", "Decode", "DecodeValue"}

// parses reports a call that reads the value it was given and answers whether it makes sense.
func parses(name string) bool {
	return anyOf(name, parseNames)
}

// anyOf reports whether one of the wanted strings is the given one.
func anyOf(value string, wanted []string) bool {
	for _, candidate := range wanted {
		if candidate == value {
			return true
		}
	}
	return false
}
