package telemetry

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// NewCommand describes one run of one subcommand: its name, the documented sub it ran, the flag
// names it was given, its exit code and its wall time. commands is the dispatch table's own names,
// so a command the CLI accepts is never recorded as unknown; help is every help text the CLI prints,
// so the documented subcommands and flag names are never duplicated here.
func NewCommand(help string, commands []string, name string, args []string, exit int, duration time.Duration) Command {
	if duration < 0 {
		duration = 0
	}
	return Command{
		Name:       CommandName(commands, name),
		Sub:        Subcommand(help, name, args),
		Flags:      FlagNames(help, name, args),
		Exit:       exit,
		DurationMS: duration.Milliseconds(),
	}
}

// flagNameRe is the shape a flag NAME must have to be recorded: a lowercase identifier, never a
// value. A token like "-1" (a negative argument), "-" or "--Definition=x" is dropped with it.
var flagNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// flagTokenRe matches a flag the usage text documents, long or short, anywhere in a command's entry.
var flagTokenRe = regexp.MustCompile(`(?:^|[\s(\[,])--?([a-z][a-z0-9-]{0,39})(?:\s|$|=|[,)\]])`)

// documentedSubcommands is the explicit allow-list of nested subcommands. A word the usage prose
// merely mentions is not here, so `tsp telemetry reads` can never record `reads`.
var documentedSubcommands = map[string][]string{
	"probe":     {"env", "arch", "quality"},
	"bench":     {"run", "score", "history", "compare", "rescore", "adjudicate", "gate", "verify-oracle"},
	"plan":      {"init", "check", "gaps", "upgrade", "add-finding", "add-evidence", "admit", "count", "export"},
	"findings":  {"record", "check", "next", "verdict"},
	"telemetry": {"record", "stats", "export"},
	"feature":   {"list", "enable", "disable"},
}

// CommandName answers the dispatched command name. commands is the same table dispatch uses, so a
// command the CLI accepts is always known and an argument it never dispatched is "unknown".
func CommandName(commands []string, name string) string {
	if slices.Contains(commands, name) {
		return name
	}
	return "unknown"
}

// FlagNames answers the NAMES of the flags in args that the command's usage text documents, in the
// order given, deduplicated. A token starting with '-' is a flag candidate; the name is what remains
// after stripping the dashes and cutting at '=', and it is kept only when it has the shape of a flag
// name AND the command's usage documents it. That second rule is what keeps a dash-prefixed VALUE
// such as `--file -private` from becoming a flag named `private`. Scanning stops at "--", after
// which everything is a positional argument.
func FlagNames(usage, name string, args []string) []string {
	documented := documentedFlags(usage, name)
	names := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if cut := strings.IndexByte(name, '='); cut >= 0 {
			name = name[:cut]
		}
		if !flagNameRe.MatchString(name) || !documented[name] || slices.Contains(names, name) {
			continue
		}
		names = append(names, name)
	}
	return names
}

// Subcommand answers the nested subcommand of one run, and "" when name does not have args[0] in
// its explicit documented allow-list. A positional path, a run slug or a word from the usage prose
// can never land in the field.
func Subcommand(_ string, name string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	candidate := args[0]
	if !flagNameRe.MatchString(candidate) || !slices.Contains(documentedSubcommands[name], candidate) {
		return ""
	}
	return candidate
}

// documentedFlags reads the flag names the usage text documents for one command: every line that
// starts with the command and the indented continuation lines under it, plus the shared flag blocks
// whose header names the command.
func documentedFlags(usage, name string) map[string]bool {
	flags := map[string]bool{}
	collect := func(line string) {
		for _, match := range flagTokenRe.FindAllStringSubmatch(line, -1) {
			flags[match[1]] = true
		}
	}
	lines := strings.Split(usage, "\n")
	commandLine := regexp.MustCompile(`^(?: {0}|  )(?:tsp )?` + regexp.QuoteMeta(name) + `(?:\s|$|\[)`)
	for i := 0; i < len(lines); i++ {
		if !commandLine.MatchString(lines[i]) {
			continue
		}
		collect(lines[i])
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" || !strings.HasPrefix(lines[j], " ") {
				break
			}
			collect(lines[j])
		}
	}
	// "flags shared by gate, setup, ...:" documents the same names for every command it lists.
	for i, line := range lines {
		if !strings.HasPrefix(line, "flags ") || !strings.HasSuffix(strings.TrimSpace(line), ":") {
			continue
		}
		if !sharedBlockNames(line, name) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" || !strings.HasPrefix(lines[j], " ") {
				break
			}
			collect(lines[j])
		}
	}
	return flags
}

// sharedBlockNames reports whether a "flags ... by a, b, c:" header lists this command.
func sharedBlockNames(header, name string) bool {
	header = strings.TrimSuffix(strings.TrimSpace(header), ":")
	for _, field := range strings.FieldsFunc(header, func(r rune) bool {
		return r == ',' || r == ' ' || r == ':'
	}) {
		if field == name {
			return true
		}
	}
	return false
}
