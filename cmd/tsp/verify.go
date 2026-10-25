package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/verify"
)

// runVerify is the blind verification of a candidate: it freezes the change, hands a second session
// a package with no path to the writer's conversation, and returns what that session decided. The
// exit code is the contract: 0 pinned, 1 blocking findings, 2 the verifier could not produce a
// verdict at all — never 0 by default, because an unverified change must not look verified.
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "repository to verify (default: current directory)")
	base := fs.String("base", "HEAD", "commit the change is applied on")
	sessionCmd := fs.String("session-cmd", "", "command that runs the verifier session; it receives the package directory as its last argument")
	task := fs.String("task", "", "the task the writer says it was asked to do")
	untrackedFlag := fs.String("include-untracked", "", "comma-separated untracked files to copy into the frozen tree")
	planPath := fs.String("plan", "", "plan JSON to hand the verifier")
	checkOut := fs.String("check-output", "", "file holding the check output for this candidate")
	redcheckOut := fs.String("redcheck-output", "", "file holding the redcheck output for this candidate")
	timeout := fs.Duration("timeout", 30*time.Minute, "time limit for the verifier session")
	keep := fs.Bool("keep", false, "leave the frozen tree and package on disk")
	publishDir := fs.String("publish", "", "publish the verdict a host already wrote into this package directory, instead of running a session: the RDD flow, where the review already happened")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if strings.TrimSpace(*publishDir) != "" {
		if strings.TrimSpace(*sessionCmd) != "" {
			fmt.Fprintln(os.Stderr, "verify: --publish reads a verdict; --session-cmd runs a session to produce one: name one of them")
			return 2
		}
		result, err := verify.Publish(*publishDir, *base, time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, "verify:", err)
			return 2
		}
		return reportVerify(result, *asJSON)
	}

	opts := verify.Options{
		Cwd:              *cwd,
		Base:             *base,
		SessionCmd:       *sessionCmd,
		Task:             *task,
		IncludeUntracked: splitList(*untrackedFlag),
		PlanPath:         *planPath,
		CheckText:        readOptional(*checkOut),
		RedCheckText:     readOptional(*redcheckOut),
		Timeout:          *timeout,
		Keep:             *keep,
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result, err := verify.Run(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify:", err)
		return 2
	}

	return reportVerify(result, *asJSON)
}

// reportVerify prints a published result and returns the exit code its verdict carries.
func reportVerify(result verify.Result, asJSON bool) int {
	if asJSON {
		encoded, err := json.MarshalIndent(result.Verdict, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "verify:", err)
			return 2
		}
		fmt.Println(string(encoded))
	} else {
		printVerifyText(result)
	}
	return verifyExit(result)
}

// verifyExit turns a verdict into the process status every caller of this command shares: 1 when
// the second session refused the change, 0 when it did not.
func verifyExit(result verify.Result) int {
	if result.Blocking() {
		return 1
	}
	return 0
}

// printVerifyText states the verdict, the evidence behind each finding, and where the candidate
// went, so a reader can disagree with the machine in the same breath.
func printVerifyText(result verify.Result) {
	fmt.Printf("tsp verify: verdict %s, %d finding(s), candidate on %s\n", result.Verdict.Verdict, len(result.Verdict.Findings), result.Base)
	for _, finding := range result.Verdict.Findings {
		fmt.Printf("  %s:%d %s — %s\n", finding.File, finding.Line, finding.Claim, finding.Evidence)
	}
	if result.Verdict.Notes != "" {
		fmt.Printf("  notes: %s\n", result.Verdict.Notes)
	}
	if result.Kept && result.FrozenDir != "" {
		fmt.Printf("tsp verify: kept %s (frozen tree) and %s (package)\n", result.FrozenDir, result.PackageDir)
	} else if result.Kept {
		fmt.Printf("tsp verify: published the verdict the host wrote in %s\n", result.PackageDir)
	}
}

// readOptional reads a file the caller named, or returns an empty string so the package records
// that the output was not produced instead of inventing a passing one.
func readOptional(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
