package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/plan"
)

// runPin holds the contract of chaining: another reviewer found something, and this run either pins it with a test
// that fails now, or records why it could not. The CLI never writes the test — the agent does — but it refuses a pin
// it cannot demonstrate, which is the difference between a claim and a guard (#401).
func runPin(args []string) int {
	fs := flag.NewFlagSet("pin", flag.ContinueOnError)
	path := fs.String("path", "", "plan file")
	findingsPath := fs.String("findings", "", "a review's findings: gentle-ai review JSON, the bench reviewers array, or a Findings table")
	record := fs.String("record", "", "pin id to record the test for")
	block := fs.String("block", "", "pin id to record as blocked")
	refute := fs.String("refute", "", "pin id to record as rejected")
	test := fs.String("test", "", "the test that pins it, as <suite path> :: <test name> (with --record)")
	reason := fs.String("reason", "", "why it is blocked or rejected (with --block or --refute)")
	timeout := fs.Duration("timeout", 5*time.Minute, "cap for the witness run (with --record); a run that does not finish pins nothing")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		return 1
	}
	planPath, err := plan.ResolveFromRoot(root, "--path", *path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		return 1
	}
	switch {
	case *findingsPath != "":
		data, err := os.ReadFile(*findingsPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pin:", err)
			return 1
		}
		pins, err := parseFindings(string(data))
		if err != nil {
			fmt.Fprintln(os.Stderr, "pin:", err)
			return 1
		}
		added, err := plan.AddPins(planPath, pins)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pin:", err)
			return 1
		}
		fmt.Printf("%d finding(s) read, %d added to %s\n", len(pins), added, planPath)
		return 0
	case *record != "":
		if *test == "" {
			fmt.Fprintln(os.Stderr, "pin: --record needs --test '<suite path> :: <test name>'")
			return 2
		}
		// The witness is named from the repository root, the way every plan in this repository names one, so the
		// plan's own directory is the wrong place to look: a plan under docs/testing would report a test that is
		// there as missing (#430).
		root := witnessRoot(planPath)
		argv, err := witnessCommand(root, *test)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pin: %s cannot be run: %v\n", *test, err)
			return 1
		}
		// A deadline through the context, because processGroup points Cancel at the group: exec refuses a Cancel it
		// did not create.
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir = root
		processGroup(cmd)
		runErr := cmd.Run()
		timedOut := ctx.Err() != nil
		switch {
		case timedOut:
			fmt.Fprintf(os.Stderr, "pin: %s did not finish within %s, so it proves nothing; %s stays open\n", *test, *timeout, *record)
			return 1
		case runErr == nil:
			fmt.Fprintf(os.Stderr, "pin: %s passes on this code, so it pins nothing; %s stays open\n", *test, *record)
			return 1
		default:
			// Only a test that ran and failed pins anything: a witness that could not start is not evidence.
			var exit *exec.ExitError
			if !errors.As(runErr, &exit) {
				fmt.Fprintf(os.Stderr, "pin: %s could not run: %v; %s stays open\n", *test, runErr, *record)
				return 1
			}
		}
		if err := plan.SetPinStatus(planPath, *record, "pinned", *test, "-"); err != nil {
			fmt.Fprintln(os.Stderr, "pin:", err)
			return 1
		}
		fmt.Printf("%s pinned by %s\n", *record, *test)
		return 0
	case *block != "" || *refute != "":
		id, status := *block, "blocked"
		if *refute != "" {
			id, status = *refute, "rejected"
		}
		if strings.TrimSpace(*reason) == "" {
			fmt.Fprintf(os.Stderr, "pin: --%s needs --reason '<why>'\n", status)
			return 2
		}
		if err := plan.SetPinStatus(planPath, id, status, "-", *reason); err != nil {
			fmt.Fprintln(os.Stderr, "pin:", err)
			return 1
		}
		fmt.Printf("%s %s\n", id, status)
		return 0
	}
	data, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		return 1
	}
	_, rows := plan.Table(string(data), plan.PinningSection)
	counts := map[string]int{}
	for _, r := range rows {
		if len(r) < 6 {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(r[3]))
		counts[status]++
		fmt.Printf("%s %s %s\n", strings.TrimSpace(r[0]), status, strings.TrimSpace(r[4]))
	}
	fmt.Printf("%d pinned, %d open, %d blocked, %d rejected\n", counts["pinned"], counts["open"], counts["blocked"], counts["rejected"])
	return 0
}

// parseFindings reads the shapes a review produces: gentle-ai's artifact (`findings`), the bench's `reviewers`
// array of those artifacts, or a plan Findings table. Every finding becomes one open pin.
func parseFindings(text string) ([]plan.Pin, error) {
	type finding struct {
		Location string `json:"location"`
		Claim    string `json:"claim"`
	}
	var artifact struct {
		Findings  []finding `json:"findings"`
		Reviewers []struct {
			Findings []finding `json:"findings"`
		} `json:"reviewers"`
	}
	var found []finding
	if json.Unmarshal([]byte(text), &artifact) == nil && (artifact.Findings != nil || artifact.Reviewers != nil) {
		found = append(found, artifact.Findings...)
		for _, r := range artifact.Reviewers {
			found = append(found, r.Findings...)
		}
	} else {
		// A Markdown table: the plan's own Findings shape, read by the plan reader.
		doc := text
		if !strings.Contains(doc, "## ") {
			doc = "## Findings\n\n" + doc
		}
		header, rows := plan.Table(doc, "Findings")
		loc, claim := findingsColumn(header, "finding"), -1
		if loc < 0 {
			loc = 1
		}
		for i, h := range header {
			if strings.Contains(strings.ToLower(h), "finding") {
				claim = i
			}
		}
		for _, r := range rows {
			if len(r) <= loc {
				continue
			}
			c := ""
			if claim >= 0 && claim < len(r) {
				c = r[claim]
			}
			found = append(found, finding{Location: strings.Trim(strings.TrimSpace(r[loc]), "`"), Claim: strings.TrimSpace(c)})
		}
	}
	if len(found) == 0 {
		return nil, errors.New("the findings file holds no finding")
	}
	var pins []plan.Pin
	for i, f := range found {
		loc := strings.TrimSpace(f.Location)
		if !strings.Contains(loc, ":") {
			return nil, fmt.Errorf("finding %d cites no path:line (%q), so it cannot be pinned or checked", i+1, loc)
		}
		pins = append(pins, plan.Pin{ID: fmt.Sprintf("P%d", i+1), Location: loc, Claim: strings.TrimSpace(f.Claim), Status: "open", Test: "-", Reason: "-"})
	}
	sort.SliceStable(pins, func(i, j int) bool { return pins[i].ID < pins[j].ID })
	return pins, nil
}
