package bench

import (
	"os"
	"path/filepath"
	"strings"

	plancheck "github.com/alesierraalta/tsp/internal/plan"
)

// RecordInstruction is what a run that pinned a test and recorded nothing is told, once, when the runner enforces
// recording. The plan is what every later reader sees, so a finding that lives only in the chat is lost (#398).
const RecordInstruction = `You changed a test file but the plan holds no Findings row, so whatever you found is lost to every reader of the plan. If the repository has no plan yet, create one with ` + "`tsp plan init`" + `; then record each defect you found with ` + "`tsp plan add-finding`" + ` now: its path:line, severity, the evidence row that observed it, and the test that fails on this code as its \u0060Pinning test\u0060, with --causality introduced or pre-existing. Do not change any source file.`

// changedTests returns the test files the run left changed in ws, from git's own view of the worktree.
func changedTests(ws string) []string {
	out, err := gitRun(ws, "status", "--porcelain")
	if err != nil {
		return nil
	}
	var tests []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		path := strings.Trim(line[3:], `"`)
		if isTestFile(path) {
			tests = append(tests, path)
		}
	}
	return tests
}

// planFindings counts the Findings rows the run's plan holds. A plan that does not exist yet holds none, which is
// the case a run that never recorded anything leaves behind.
func planFindings(ws string) int {
	path, _ := resolvePlanPath(ws)
	data, err := os.ReadFile(filepath.Join(ws, path))
	if err != nil {
		return 0
	}
	_, tableRows := plancheck.Table(string(data), "Findings")
	return len(tableRows)
}

// recordedAfterNudge answers whether the run recorded a finding after the nudge. The plan held no finding when the
// nudge was sent, so any finding it holds now is the recording the ask asked for (#398).
func recordedAfterNudge(ws string) bool { return planFindings(ws) > 0 }

// needsRecordNudge answers whether the runner should ask this workspace to record what it found: it changed a test,
// and the plan holds no finding — including the case where no plan was written at all.
func needsRecordNudge(ws string) bool {
	if len(changedTests(ws)) == 0 {
		return false
	}
	return planFindings(ws) == 0
}
