package telemetry

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

// Report is the answer `tsp telemetry stats` prints: the ledger it read, how many lines it could
// not decode, and one group per (tsp version, skill identity) pair.
type Report struct {
	Path           string  `json:"path"`
	ValidLines     int     `json:"valid_lines"`
	MalformedLines int     `json:"malformed_lines"`
	Groups         []Group `json:"groups"`
}

// Group is one (version, skill) pair: everything the ledger measured for that pair. Absent
// measurements stay nil or absent rather than reading as a real zero.
type Group struct {
	Version                string          `json:"version"`
	Skill                  string          `json:"skill"`
	Events                 int             `json:"events"`
	Runs                   int             `json:"runs"`
	Commands               []CommandStats  `json:"commands"`
	CheckDecisions         []DecisionCount `json:"check_decisions"`
	Blocked                BlockedStats    `json:"blocked"`
	Hosts                  HostStats       `json:"hosts"`
	TokensPerPinnedRedTest *float64        `json:"tokens_per_pinned_red_test,omitempty"`
}

// CommandStats is one subcommand's count, failures, failure rate and duration percentiles. The
// failure rate is exit != 0 over every command event for that subcommand.
type CommandStats struct {
	Name        string  `json:"name"`
	Count       int     `json:"count"`
	Failures    int     `json:"failures"`
	FailureRate float64 `json:"failure_rate"`
	P50MS       int64   `json:"p50_duration_ms"`
	P95MS       int64   `json:"p95_duration_ms"`
}

// DecisionCount is how often a check event carried one decision value.
type DecisionCount struct {
	Decision string `json:"decision"`
	Count    int    `json:"count"`
}

// BlockedStats is the blocked rate over check events. Blocked means a check event with exit 2 or
// reason "error"; Rate is absent when nothing was checked.
type BlockedStats struct {
	Checks  int      `json:"checks"`
	Blocked int      `json:"blocked"`
	Rate    *float64 `json:"rate,omitempty"`
}

// HostStats is the host side of a group: token totals, the cache hit, turns, tool calls and the
// offer distribution. CacheHit is computed only from host runs that measured input, cache_read and
// cache_write. Offers counts only values a host actually sent; an absent offer is not measured.
type HostStats struct {
	Runs            int          `json:"runs"`
	Input           int64        `json:"input"`
	Output          int64        `json:"output"`
	CacheRead       int64        `json:"cache_read"`
	CacheWrite      int64        `json:"cache_write"`
	CacheHit        *float64     `json:"cache_hit,omitempty"`
	Turns           int64        `json:"turns"`
	ToolCalls       int64        `json:"tool_calls"`
	TurnsPerRun     *float64     `json:"turns_per_run,omitempty"`
	ToolCallsPerRun *float64     `json:"tool_calls_per_run,omitempty"`
	Offers          []OfferCount `json:"offers"`
}

// OfferCount is how often host runs carried one offer outcome (accepted, declined, aborted, none).
type OfferCount struct {
	Offer string `json:"offer"`
	Count int    `json:"count"`
}

// Stats reads the ledger at Path and aggregates it. A missing ledger is an empty report, not an
// error: nothing has been recorded yet, and that is a valid answer.
func Stats() (Report, error) {
	path, err := PathReadOnly()
	if err != nil {
		return Report{}, err
	}
	return LoadReport(path)
}

// LoadReport reads one ledger file and aggregates it. Lines that are not a schema-1 JSON object are
// counted as malformed and reported, never silently dropped.
func LoadReport(path string) (Report, error) {
	lines, malformed, err := readLedger(path)
	if err != nil {
		return Report{}, err
	}
	report := Report{Path: path, ValidLines: len(lines), MalformedLines: malformed, Groups: []Group{}}
	builder := newReportBuilder()
	for _, line := range lines {
		builder.add(line.fields)
	}
	report.Groups = builder.finish()
	return report, nil
}

// ledgerLine is one decoded line plus the bytes it came from, so export can echo a valid line
// verbatim while stats reads the same decode.
type ledgerLine struct {
	raw    []byte
	fields map[string]any
}

// readLedger decodes the ledger generically into maps: check, finding and plan fields are read by
// their exact JSON names even while their Go types live in a parallel change.
func readLedger(path string) ([]ledgerLine, int, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var lines []ledgerLine
	malformed := 0
	for {
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) > 0 {
			raw = trimLine(raw)
			if len(raw) > 0 {
				var fields map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(raw)))
				decoder.UseNumber()
				if err := decoder.Decode(&fields); err != nil || fields == nil || !knownSchema(fields) {
					malformed++
				} else {
					lines = append(lines, ledgerLine{raw: raw, fields: fields})
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, 0, readErr
		}
	}
	return lines, malformed, nil
}

// trimLine removes the line ending and a stray carriage return, so exported lines stay verbatim
// without their terminator and a trailing blank line is not counted as malformed.
func trimLine(raw []byte) []byte {
	return []byte(strings.TrimRight(string(raw), "\r\n"))
}

// knownSchema refuses a line from a format this reader does not know. A missing schema is tolerated
// so older fixtures and hand-written ledgers still read; a present one must say 1.
func knownSchema(fields map[string]any) bool {
	if _, present := fields["schema"]; !present {
		return true
	}
	number, ok := numberField(fields, "schema")
	return ok && number == Schema
}

type reportBuilder struct {
	order  []string
	groups map[string]*groupBuilder
}

type groupBuilder struct {
	eventCount int
	runIDs     map[string]struct{}

	commands   map[string]*commandBuilder
	decisions  map[string]int
	checks     int
	blocked    int
	redByRun   map[string]int64
	hostTokens map[string]int64
	hosts      hostBuilder
}

type commandBuilder struct {
	count     int
	failures  int
	durations []int64
}

type hostBuilder struct {
	runs          int
	input         int64
	output        int64
	cacheRead     int64
	cacheWrite    int64
	cacheHitRead  int64
	cacheHitDenom int64
	turns         int64
	turnsRuns     int
	toolCalls     int64
	toolCallRuns  int
	offers        map[string]int
}

func newReportBuilder() *reportBuilder {
	return &reportBuilder{groups: map[string]*groupBuilder{}}
}

// groupFor answers the group a line belongs to, creating it in first-seen order.
func (b *reportBuilder) groupFor(version, skill string) *groupBuilder {
	key := version + "\x00" + skill
	if group, ok := b.groups[key]; ok {
		return group
	}
	group := &groupBuilder{
		runIDs:     map[string]struct{}{},
		commands:   map[string]*commandBuilder{},
		decisions:  map[string]int{},
		redByRun:   map[string]int64{},
		hostTokens: map[string]int64{},
		hosts:      hostBuilder{offers: map[string]int{}},
	}
	b.groups[key] = group
	b.order = append(b.order, key)
	return group
}

// add folds one decoded ledger line into its group.
func (b *reportBuilder) add(fields map[string]any) {
	version, _ := fields["version"].(string)
	skill, _ := fields["skill"].(string)
	group := b.groupFor(version, skill)
	group.eventCount++

	runID, _ := fields["run_id"].(string)
	if runID != "" {
		group.runIDs[runID] = struct{}{}
	}

	switch kind, _ := fields["kind"].(string); kind {
	case KindCommand:
		group.addCommand(fields)
	case "check":
		group.addCheck(fields, runID)
	case KindHostRun:
		group.addHost(fields, runID)
	}
}

func (g *groupBuilder) addCommand(fields map[string]any) {
	command, ok := fields["command"].(map[string]any)
	if !ok {
		return
	}
	name, _ := command["name"].(string)
	if name == "" {
		return
	}
	if sub, _ := command["sub"].(string); sub != "" {
		name += " " + sub
	}
	stats, ok := g.commands[name]
	if !ok {
		stats = &commandBuilder{}
		g.commands[name] = stats
	}
	stats.count++
	if exit, _ := numberField(command, "exit"); exit != 0 {
		stats.failures++
	}
	duration, _ := numberField(command, "duration_ms")
	stats.durations = append(stats.durations, duration)
}

func (g *groupBuilder) addCheck(fields map[string]any, runID string) {
	check, ok := fields["check"].(map[string]any)
	if !ok {
		return
	}
	g.checks++
	exit, _ := numberField(check, "exit")
	reason, _ := check["reason"].(string)
	if exit == 2 || reason == "error" {
		g.blocked++
	}
	if decision, _ := check["decision"].(string); decision != "" {
		g.decisions[decision]++
	}
	if red, measured := numberField(check, "red"); measured && red > 0 && runID != "" {
		g.redByRun[runID] += red
	}
}

func (g *groupBuilder) addHost(fields map[string]any, runID string) {
	g.hosts.runs++
	host, ok := fields["host"].(map[string]any)
	if !ok {
		return
	}
	if offer, _ := host["offer"].(string); offer != "" {
		g.hosts.offers[offer]++
	}
	if turns, measured := numberField(host, "turns"); measured {
		g.hosts.turns += turns
		g.hosts.turnsRuns++
	}
	if calls, measured := numberField(host, "tool_calls"); measured {
		g.hosts.toolCalls += calls
		g.hosts.toolCallRuns++
	}
	tokens, ok := host["tokens"].(map[string]any)
	if !ok {
		return
	}
	input, hasInput := numberField(tokens, "input")
	output, _ := numberField(tokens, "output")
	cacheRead, hasRead := numberField(tokens, "cache_read")
	cacheWrite, hasWrite := numberField(tokens, "cache_write")
	g.hosts.input += input
	g.hosts.output += output
	g.hosts.cacheRead += cacheRead
	g.hosts.cacheWrite += cacheWrite
	if runID != "" {
		g.hostTokens[runID] += input + output + cacheRead + cacheWrite
	}
	if hasInput && hasRead && hasWrite {
		g.hosts.cacheHitRead += cacheRead
		g.hosts.cacheHitDenom += input + cacheRead + cacheWrite
	}
}

// finish turns the accumulated maps into stable, sorted report values.
func (b *reportBuilder) finish() []Group {
	groups := make([]Group, 0, len(b.order))
	for _, key := range b.order {
		group := b.groups[key]
		parts := strings.SplitN(key, "\x00", 2)
		report := Group{
			Version:        parts[0],
			Skill:          parts[1],
			Events:         group.eventCount,
			Runs:           len(group.runIDs),
			Commands:       group.commandStats(),
			CheckDecisions: group.decisionStats(),
			Blocked:        group.blockedStats(),
			Hosts:          group.hostStats(),
		}
		report.TokensPerPinnedRedTest = group.tokensPerPinnedRedTest()
		groups = append(groups, report)
	}
	return groups
}

func (g *groupBuilder) commandStats() []CommandStats {
	names := make([]string, 0, len(g.commands))
	for name := range g.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	stats := make([]CommandStats, 0, len(names))
	for _, name := range names {
		command := g.commands[name]
		sort.Slice(command.durations, func(i, j int) bool { return command.durations[i] < command.durations[j] })
		entry := CommandStats{
			Name:     name,
			Count:    command.count,
			Failures: command.failures,
			P50MS:    percentile(command.durations, 50),
			P95MS:    percentile(command.durations, 95),
		}
		if command.count > 0 {
			entry.FailureRate = float64(command.failures) / float64(command.count)
		}
		stats = append(stats, entry)
	}
	return stats
}

func (g *groupBuilder) decisionStats() []DecisionCount {
	decisions := make([]string, 0, len(g.decisions))
	for decision := range g.decisions {
		decisions = append(decisions, decision)
	}
	sort.Strings(decisions)
	stats := make([]DecisionCount, 0, len(decisions))
	for _, decision := range decisions {
		stats = append(stats, DecisionCount{Decision: decision, Count: g.decisions[decision]})
	}
	return stats
}

func (g *groupBuilder) blockedStats() BlockedStats {
	stats := BlockedStats{Checks: g.checks, Blocked: g.blocked}
	if g.checks > 0 {
		rate := float64(g.blocked) / float64(g.checks)
		stats.Rate = &rate
	}
	return stats
}

func (g *groupBuilder) hostStats() HostStats {
	stats := HostStats{
		Runs:       g.hosts.runs,
		Input:      g.hosts.input,
		Output:     g.hosts.output,
		CacheRead:  g.hosts.cacheRead,
		CacheWrite: g.hosts.cacheWrite,
		Turns:      g.hosts.turns,
		ToolCalls:  g.hosts.toolCalls,
		Offers:     offerStats(g.hosts.offers),
	}
	if g.hosts.cacheHitDenom > 0 {
		hit := float64(g.hosts.cacheHitRead) / float64(g.hosts.cacheHitDenom)
		stats.CacheHit = &hit
	}
	if g.hosts.turnsRuns > 0 {
		perRun := float64(g.hosts.turns) / float64(g.hosts.turnsRuns)
		stats.TurnsPerRun = &perRun
	}
	if g.hosts.toolCallRuns > 0 {
		perRun := float64(g.hosts.toolCalls) / float64(g.hosts.toolCallRuns)
		stats.ToolCallsPerRun = &perRun
	}
	return stats
}

// offerStats answers the offer distribution in stable order, counting only offers hosts sent.
func offerStats(offers map[string]int) []OfferCount {
	names := make([]string, 0, len(offers))
	for offer := range offers {
		names = append(names, offer)
	}
	sort.Strings(names)
	stats := make([]OfferCount, 0, len(names))
	for _, offer := range names {
		stats = append(stats, OfferCount{Offer: offer, Count: offers[offer]})
	}
	return stats
}

// tokensPerPinnedRedTest is the tokens the host spent on runs that pinned at least one red test,
// divided by the red tests those runs pinned. A run with no red check does not enter either side.
func (g *groupBuilder) tokensPerPinnedRedTest() *float64 {
	var red, tokens int64
	for runID, pinned := range g.redByRun {
		red += pinned
		tokens += g.hostTokens[runID]
	}
	if red == 0 {
		return nil
	}
	value := float64(tokens) / float64(red)
	return &value
}

// percentile answers the nearest-rank percentile of an ascending slice: rank ceil(p/100*n), clamped.
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// numberField reads a JSON number from a generically decoded object. A field that is absent or not
// a number is not measured.
func numberField(object map[string]any, key string) (int64, bool) {
	switch value := object[key].(type) {
	case json.Number:
		if number, err := value.Int64(); err == nil {
			return number, true
		}
		if number, err := value.Float64(); err == nil {
			return int64(number), true
		}
	case float64:
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	}
	return 0, false
}
