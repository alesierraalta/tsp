package plan

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// addPlan is the smallest plan carrying one evidence row: a finding has something real to cite, and the
// checker has something to hold the candidate row to.
func addPlan(rows ...string) string {
	return header + strings.Join(rows, "") + ledger +
		"| E1 | it drops the comma | `node --test` | `a,b` | 3 fields | reverted -> red | same input | observado |\n"
}

// openFinding is a row the checker accepts as it stands, so each test varies exactly one thing.
func TestAddFindingWritesReproduction(t *testing.T) {
	body, err := embeddedTemplate(MicroTemplatePath)
	if err != nil {
		t.Fatal(err)
	}
	body = strings.Replace(body, "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|", "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n| E1 | claim | run | | input | observed | | | | | | control | repro | observado |", 1)
	p := write(t, t.TempDir(), "plan.md", body)
	f := openFinding()
	f.Reproduction = "false"
	if _, err := AddFinding(p, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "| false |") {
		t.Fatalf("missing reproduction: %s", got)
	}
}

func TestReproductionRejectsNewline(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	f := openFinding()
	f.Reproduction = "false\ntrue"
	if _, err := AddFinding(p, f); err == nil || !strings.Contains(err.Error(), "newline") {
		t.Fatalf("expected newline refusal, got %v", err)
	}
}

func openFinding() Finding {
	return Finding{
		ID: "F1", Location: "src/a.js:5", Severity: "data loss", DataSafe: "yes", Evidence: "E1",
		Status: "open", VerdictBy: "me / 2026-09-10", Reason: "not pinned",
	}
}

// edited returns openFinding with one field changed, so a refusal test varies exactly one thing.
func edited(apply func(*Finding)) Finding {
	f := openFinding()
	apply(&f)
	return f
}

// add-finding exists so an operator stops hand-editing a ten-column row: it writes the row the checker
// already accepts and reports the line it landed on.
func TestAddFindingWritesARowTheCheckerAccepts(t *testing.T) {
	cases := []struct {
		name string
		plan string
	}{
		{"a table with no rows lands under the separator", addPlan()},
		{"a table with rows lands under the last one",
			addPlan("| F0 | `src/b.js:9` y | M | yes | E1 | t.js :: y | open | me | - | - |\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, t.TempDir(), "plan.md", tc.plan)
			line, err := AddFinding(p, openFinding())
			if err != nil {
				t.Fatalf("AddFinding: %v", err)
			}
			got, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if problems := CheckDocument(string(got)); len(problems) != 0 {
				t.Fatalf("the row add-finding wrote is rejected by its own checker: %v", problems)
			}
			lines := strings.Split(string(got), "\n")
			if line < 1 || line > len(lines) || !strings.HasPrefix(lines[line-1], "| F1 |") {
				t.Fatalf("the reported line %d is not the new row", line)
			}
		})
	}
}

// Every refusal leaves the file byte-identical. A command that half-writes a plan is worse than one that
// refuses, because the loss is silent: the runner closes on a plan that is missing the run.
func TestAddFindingRefusesWithoutMovingTheFile(t *testing.T) {
	cases := []struct {
		name    string
		plan    string
		finding Finding
		want    string
		usage   bool // the value refusals the CLI maps to exit 2
	}{
		{"a status outside the vocabulary", addPlan(), edited(func(f *Finding) { f.Status = "resolved" }),
			"open, confirmed, fixed, gap-closed, rejected, wontfix", true},
		{"a settled status with no pinning test", addPlan(), edited(func(f *Finding) { f.Status = "confirmed" }),
			"test", true},
		{"a location that is not path:line", addPlan(), edited(func(f *Finding) { f.Location = "src/a.js" }),
			"path:line", true},
		{"evidence that is not a ledger row", addPlan(), edited(func(f *Finding) { f.Evidence = "E1, E9" }),
			"E9", false},
		{"a value carrying a newline", addPlan(), edited(func(f *Finding) { f.Reason = "first\nsecond" }),
			"newline", true},
		{"a fingerprint that is neither a digest, a SHA nor a placeholder", addPlan(),
			edited(func(f *Finding) { f.Fingerprint = "TestFoo" }), "--fingerprint", true},
		{"a plan the checker already rejects",
			addPlan("| F0 | no path here | M | yes | E1 | | open | me | - | - |\n"), openFinding(),
			"would not pass plan check", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, t.TempDir(), "plan.md", tc.plan)
			before, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = AddFinding(p, tc.finding)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal %q does not name %q", err, tc.want)
			}
			if got := errors.Is(err, ErrUsage); got != tc.usage {
				t.Fatalf("errors.Is(err, ErrUsage) = %v, want %v: %q", got, tc.usage, err)
			}
			after, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("a refusal must leave the plan byte-identical")
			}
		})
	}
}

// A git SHA is a fingerprint the checker accepts, so add-finding writes it into the row as given.
func TestAddFindingWritesAGitSHAFingerprint(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	if _, err := AddFinding(p, edited(func(f *Finding) { f.Fingerprint = "953908a" })); err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "| 953908a |") {
		t.Fatalf("the row does not carry the SHA fingerprint:\n%s", got)
	}
}

// Applying the command twice refuses and changes nothing: two rows for one finding score one defect twice.
func TestAddFindingRefusesADuplicateID(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	if _, err := AddFinding(p, openFinding()); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddFinding(p, openFinding()); err == nil || !strings.Contains(err.Error(), "already row") {
		t.Fatalf("the second apply must refuse the duplicate, got %v", err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the refused second apply must not move the file")
	}
}

// The command writes findings only: the ledger an operator already audited stays exactly as it was.
func TestAddFindingWritesNoEvidenceLedgerRow(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	original, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddFinding(p, openFinding()); err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	before, after := strings.Split(string(original), "\n"), strings.Split(string(got), "\n")
	bh, be := sectionRegion(before, "Evidence ledger")
	ah, ae := sectionRegion(after, "Evidence ledger")
	if bh < 0 || ah < 0 {
		t.Fatal("the plan has no Evidence ledger")
	}
	if want, have := strings.Join(before[bh:be], "\n"), strings.Join(after[ah:ae], "\n"); want != have {
		t.Fatalf("the Evidence ledger moved:\nbefore:\n%s\nafter:\n%s", want, have)
	}
}

// setUserCacheDir points the per-user cache at dir for this process, so the plan lock directory under test is a
// test-owned one rather than the developer's real cache.
func setUserCacheDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LocalAppData", dir)
	t.Setenv("HOME", dir)
}

// Simultaneous calls used to lose rows: each one read the document, built its candidate and wrote it into place,
// so the losers were overwritten by the last write — and the plan still checked as `well formed`, which is what
// made the loss silent. The read, the validation and the write are one critical section held across processes,
// so every call's distinct id survives to the checker.
func TestAddFindingSerialisesConcurrentCalls(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "plan.md", addPlan())
	setUserCacheDir(t, t.TempDir())
	// The same plan reached through a relative and an absolute path is one file, so it has to be one lock: a lock
	// keyed on the spelling rather than on the file would serialise neither spelling against the other.
	t.Chdir(dir)

	ids := []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12"}
	start := make(chan struct{})
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-start
			f := openFinding()
			f.ID = id
			path := p
			if i%2 == 1 {
				path = filepath.Base(p)
			}
			_, errs[i] = AddFinding(path, f)
		}(i, id)
	}
	close(start)
	wg.Wait()

	for i, id := range ids {
		if err := errs[i]; err != nil {
			t.Fatalf("call %d adding %s: %v", i, id, err)
		}
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if n := strings.Count(string(got), "| "+id+" |"); n != 1 {
			t.Fatalf("finding %s appears %d times, want exactly once:\n%s", id, n, got)
		}
	}
	if problems := CheckDocument(string(got)); len(problems) != 0 {
		t.Fatalf("a plan every concurrent call contributed to is not well formed: %v", problems)
	}
	// The command writes findings only, however many callers ran at once: the ledger keeps the row it had.
	if ledger := scanSection(strings.Split(string(got), "\n"), "Evidence ledger"); len(ledger.rows) != 1 {
		t.Fatalf("the Evidence ledger has %d rows, want the one already there", len(ledger.rows))
	}
	// The lock lives outside the repository and the write leaves nothing behind: the plan's own directory holds
	// the plan and nothing else.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	if len(names) != 1 || names[0] != "plan.md" {
		t.Fatalf("the plan directory must hold the plan alone, found %v", names)
	}
}

// The write replaces the destination through a temp file and a rename, and the temp file carries whatever mode
// it was created with. A plan an operator tightened to 0600 must not come back 0644: the write has no licence
// to widen a file nobody asked it to widen.
func TestAddFindingKeepsThePlanMode(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFinding(p, openFinding()); err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the plan was written mode %o, want the 600 the operator set", perm)
	}
}

// Row 35 of docs/testing/test-plan.md once split into extra cells because a hand-edited cell carried a
// raw pipe, and every column to its right moved. The escaping is what keeps the row a row.
func TestAddFindingEscapesAPipeInACell(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	f := openFinding()
	f.Reason = "a|b"
	line, err := AddFinding(p, f)
	if err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(got), "\n")
	row := lines[line-1]
	if !strings.Contains(row, `a\|b`) {
		t.Fatalf("the pipe must be escaped in the cell: %q", row)
	}
	cells := split(row)
	if want := len(split(findingsHeaderLine(t, lines))); len(cells) != want {
		t.Fatalf("the escaped pipe changed the cell count: %d cells, want %d", len(cells), want)
	}
	if cells[8] != "a|b" {
		t.Fatalf("the reason cell reads %q, want the text the operator typed", cells[8])
	}
	if problems := CheckDocument(string(got)); len(problems) != 0 {
		t.Fatalf("the escaped row is not accepted: %v", problems)
	}
}

// TestAddFindingHelperProcess is not a test: it is the one writer the process-level tests re-execute. It does
// nothing unless the parent handed it a plan and an id, and it fails the way any test fails, so the parent
// reads a real exit status rather than a guessed one.
func TestAddFindingHelperProcess(t *testing.T) {
	plan, id := os.Getenv(addFindingChildPlan), os.Getenv(addFindingChildID)
	if plan == "" || id == "" {
		t.Skip("not the helper process")
	}
	f := openFinding()
	f.ID = id
	if _, err := AddFinding(plan, f); err != nil {
		t.Fatalf("add-finding: %v", err)
	}
}

// A fenced example above the real Findings table is documentation, never the table: the insertion line comes
// from the scan's own heading, where a raw walk would see the example's rows and write the row into the fence.
func TestAddFindingLandsInTheRealTableUnderAFencedExample(t *testing.T) {
	doc := "## Findings\n\n```markdown\n| Id | Finding |\n|---|---|\n| - | example |\n```\n\n" +
		strings.TrimPrefix(header, "## Findings\n\n") + ledger + "| E1 | c | cmd | i | o | m | r | observado |\n"
	p := write(t, t.TempDir(), "plan.md", doc)
	if _, err := AddFinding(p, openFinding()); err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "```markdown\n| Id | Finding |\n|---|---|\n| - | example |\n```") {
		t.Fatalf("the fenced example must be untouched:\n%s", text)
	}
	if problems := CheckDocument(text); len(problems) != 0 {
		t.Fatalf("the written plan must pass: %v", problems)
	}
	if _, err := AddFinding(p, openFinding()); err == nil || !strings.Contains(err.Error(), "already row") {
		t.Fatalf("a second call with the same id must refuse, got %v", err)
	}
}

// Two processes writing the same id: the second one has to read what the first wrote and refuse. Without the
// lock both read the same document, both find no duplicate, and both rename a row the other never saw.
func TestAddFindingRefusesTheSameIDFromTwoProcesses(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "plan.md", addPlan())
	setUserCacheDir(t, t.TempDir())
	tmpdir := t.TempDir()

	errs := make([]error, 2)
	outs := make([]string, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			outs[i], errs[i] = addFindingChild(p, "F1", tmpdir)
		}(i)
	}
	close(start)
	wg.Wait()

	winners, refusers := 0, 0
	for i := range errs {
		switch {
		case errs[i] == nil:
			winners++
		case strings.Contains(outs[i], "already row"):
			refusers++
		default:
			t.Fatalf("a writer failed for the wrong reason: %v\n%s", errs[i], outs[i])
		}
	}
	if winners != 1 || refusers != 1 {
		t.Fatalf("want one winner and one duplicate refusal, got %d winners and %d refusals", winners, refusers)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(got), "| F1 |"); n != 1 {
		t.Fatalf("the contested id appears %d times, want exactly once:\n%s", n, got)
	}
	if problems := CheckDocument(string(got)); len(problems) != 0 {
		t.Fatalf("the surviving row must pass the checker: %v", problems)
	}
}

// The L5 sequence applies the command twice. The second call refuses and changes nothing, because two
// rows for one finding score one defect twice.
func TestAddFindingRefusesToApplyTwice(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	if _, err := AddFinding(p, openFinding()); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddFinding(p, openFinding()); err == nil {
		t.Fatal("the second apply must refuse")
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the refused second apply must not move the file")
	}
}

// A stable root that cannot be resolved is a refusal, never a write without a lock: a plan that accepts an
// unserialised write is the silent row loss this lock exists to prevent.
func TestAddFindingRefusesWithoutAStableLockDirectory(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Every variable os.UserCacheDir reads, so the refusal is reached on whichever platform the suite runs.
	for _, name := range []string{"XDG_CACHE_HOME", "LocalAppData", "HOME"} {
		t.Setenv(name, "")
	}
	if _, err := AddFinding(p, openFinding()); err == nil {
		t.Fatal("a plan with no resolvable lock directory must refuse to be written")
	} else if !strings.Contains(err.Error(), "serialization") {
		t.Fatalf("the refusal must say why it will not write: %v", err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a refused write must leave the plan byte-identical")
	}
}

// Eight real writers, half of them handed one TMPDIR and half another, all writing the same plan: independent
// verification ran exactly this and four rows vanished while all eight processes reported success. Every
// distinct id has to survive, the ledger keeps its row, and neither temporary directory may hold a lock.
func TestAddFindingSerialisesWritersWithDifferentTempDirs(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "plan.md", addPlan())
	cache := t.TempDir()
	setUserCacheDir(t, cache)
	tmpdirs := []string{filepath.Join(t.TempDir(), "one"), filepath.Join(t.TempDir(), "two")}
	for _, d := range tmpdirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	ids := []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8"}
	errs := make([]error, len(ids))
	outs := make([]string, len(ids))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-start
			outs[i], errs[i] = addFindingChild(p, id, tmpdirs[i%len(tmpdirs)])
		}(i, id)
	}
	close(start)
	wg.Wait()

	// No lock under either TMPDIR: a lock per temporary namespace is the defect, because two writers handed
	// two TMPDIRs then queue behind two different locks and the later rename erases the earlier row.
	for _, d := range tmpdirs {
		for _, name := range entryNames(t, d) {
			if strings.HasSuffix(name, ".lock") {
				t.Fatalf("a lock file was left in TMPDIR %s: %s", d, name)
			}
		}
	}

	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("writer %d adding %s failed: %v\n%s", i, id, errs[i], outs[i])
		}
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if n := strings.Count(string(got), "| "+id+" |"); n != 1 {
			t.Fatalf("finding %s survived %d times, want exactly once:\n%s", id, n, got)
		}
	}
	if problems := CheckDocument(string(got)); len(problems) != 0 {
		t.Fatalf("a plan every writer contributed to is not well formed: %v", problems)
	}
	if ledger := scanSection(strings.Split(string(got), "\n"), "Evidence ledger"); len(ledger.rows) != 1 {
		t.Fatalf("the Evidence ledger has %d rows, want the one already there", len(ledger.rows))
	}
	// One lock for every writer, under the per-user cache, still there afterwards: the file is never unlinked
	// while another writer may be waiting on it.
	lock, err := planLockPath(canonicalPath(p))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(lock, cache) {
		t.Fatalf("the lock %s is not under the per-user cache %s", lock, cache)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("the lock file has to persist in the cache: %v", err)
	}
	// The plan's own directory holds the plan and nothing else: no lock file, no half-written temp file.
	if names := entryNames(t, dir); len(names) != 1 || names[0] != "plan.md" {
		t.Fatalf("the plan directory must hold the plan alone, found %v", names)
	}
}

// One plan has many spellings — a relative name, an absolute name, a symlinked file, a file under a symlinked
// directory — and they are all one file, so they owe one lock. Writers that keyed the lock on the spelling
// instead of on the file queued behind different locks and lost each other's rows.
func TestAddFindingSharesOneLockAcrossEverySpellingOfThePlan(t *testing.T) {
	dir := t.TempDir()
	target := write(t, dir, "real.md", addPlan())
	link := filepath.Join(dir, "link.md")
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	if err := os.Symlink(dir, alias); err != nil {
		t.Skipf("directory symlinks are unavailable here: %v", err)
	}
	setUserCacheDir(t, t.TempDir())
	t.Chdir(dir)

	spellings := []string{target, link, filepath.Base(target), filepath.Join("alias", "real.md")}
	ids := []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8"}
	start := make(chan struct{})
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-start
			f := openFinding()
			f.ID = id
			_, errs[i] = AddFinding(spellings[i%len(spellings)], f)
		}(i, id)
	}
	close(start)
	wg.Wait()

	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("writer %d adding %s through %s: %v", i, id, spellings[i%len(spellings)], errs[i])
		}
	}
	at, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if n := strings.Count(string(at), "| "+id+" |"); n != 1 {
			t.Fatalf("finding %s survived %d times, want exactly once:\n%s", id, n, at)
		}
	}
	through, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if string(through) != string(at) {
		t.Fatal("every spelling must read the one plan")
	}
	if problems := CheckDocument(string(at)); len(problems) != 0 {
		t.Fatalf("the plan is not well formed after eight spellings wrote to it: %v", problems)
	}
}

// A settled finding takes the pinning-test column it needs, and a plan whose table has one accepts it.
func TestAddFindingWritesASettledRowWithItsPinningTest(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", addPlan())
	f := openFinding()
	f.Status = "confirmed"
	f.Test = "tests/a.test.js :: clamps the boundary"
	if _, err := AddFinding(p, f); err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if problems := CheckDocument(string(got)); len(problems) != 0 {
		t.Fatalf("a settled row with its test is not accepted: %v", problems)
	}
	if !strings.Contains(string(got), "tests/a.test.js :: clamps the boundary") {
		t.Fatal("the pinning test was not written into the row")
	}
}

// A plan reached through a symlink is the file the link names. The transaction used to read the link and then
// rename a new file over it, so the link became a regular file holding the row while the real plan never
// moved: the link saw the row, every other spelling of the plan did not, and the loss was silent.
func TestAddFindingWritesThroughASymlinkWithoutReplacingIt(t *testing.T) {
	dir := t.TempDir()
	target := write(t, dir, "real.md", addPlan())
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	line, err := AddFinding(link, openFinding())
	if err != nil {
		t.Fatalf("AddFinding through the link: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the plan path stopped being a symlink: it is now %v", info.Mode())
	}
	if real, err := os.Readlink(link); err != nil || real != target {
		t.Fatalf("the link must still point at %s, got %q (%v)", target, real, err)
	}

	through, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	at, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(through) != string(at) {
		t.Fatal("the link and its target must be the same plan")
	}
	if !bytes.Contains(at, []byte("| F1 |")) {
		t.Fatalf("the target must carry the new row:\n%s", at)
	}
	if bytes.Equal(before, at) {
		t.Fatal("the target must have moved: the row exists nowhere else")
	}
	lines := strings.Split(string(at), "\n")
	if line < 1 || line > len(lines) || !strings.HasPrefix(lines[line-1], "| F1 |") {
		t.Fatalf("the reported line %d is not the new row", line)
	}
	if problems := CheckDocument(string(at)); len(problems) != 0 {
		t.Fatalf("the plan the link names must pass the checker: %v", problems)
	}

	// The duplicate refusal still holds through the link, and it still leaves both spellings untouched.
	if _, err := AddFinding(link, openFinding()); err == nil || !strings.Contains(err.Error(), "already row") {
		t.Fatalf("a second add through the link must refuse the duplicate, got %v", err)
	}
	if after, err := os.ReadFile(target); err != nil || !bytes.Equal(after, at) {
		t.Fatalf("the refusal must leave the file byte-identical (%v)", err)
	}
}

// The plan lock is keyed on the plan, never on a temporary namespace. A lock file under os.TempDir() gave
// every TMPDIR of one user its own lock, so eight writers with four different TMPDIR values all reported
// success while all but four rows were lost. This asserts the property directly; the process-level tests
// below prove it end to end.
// The plan lock is keyed on the plan, never on a temporary namespace. A lock file under os.TempDir() gave
// every TMPDIR of one user its own lock, so eight writers with four different TMPDIR values all reported
// success while all but four rows were lost. This asserts the property directly; the process-level tests
// below prove it end to end.
func TestPlanLockPathIgnoresTempDir(t *testing.T) {
	cache := t.TempDir()
	plan := filepath.Join(t.TempDir(), "plan.md")
	setUserCacheDir(t, cache)

	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "one"))
	first, err := planLockPath(canonicalPath(plan))
	if err != nil {
		t.Fatalf("planLockPath: %v", err)
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "two"))
	second, err := planLockPath(canonicalPath(plan))
	if err != nil {
		t.Fatalf("planLockPath: %v", err)
	}
	if first != second {
		t.Fatalf("two TMPDIR values gave two locks for one plan:\n%s\n%s", first, second)
	}
	if !strings.HasPrefix(first, cache) {
		t.Fatalf("the lock %s is not under the per-user cache %s", first, cache)
	}
}

// addFindingChild runs one real writer process against plan with its own TMPDIR and returns everything it
// printed, so a failure names the refusal instead of only the exit status.
func addFindingChild(plan, id, tmpdir string) (string, error) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestAddFindingHelperProcess$", "-test.count=1")
	cmd.Env = append(environWithout("TMPDIR", addFindingChildPlan, addFindingChildID),
		"TMPDIR="+tmpdir, addFindingChildPlan+"="+plan, addFindingChildID+"="+id)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// entryNames lists a directory, so a test can say what is in it without guessing.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// environWithout copies the environment with every entry naming one of names dropped: a duplicated TMPDIR
// would leave which value a child sees up to the reader.
func environWithout(names ...string) []string {
	out := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if name, _, _ := strings.Cut(e, "="); slices.Contains(names, name) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// findingsHeaderLine finds the Findings table's header, which is also the cell count every row owes.
func findingsHeaderLine(t *testing.T, lines []string) string {
	t.Helper()
	for _, l := range lines {
		if strings.HasPrefix(l, "| Id |") {
			return l
		}
	}
	t.Fatal("no Findings header line")
	return ""
}

// The two variables the process-level tests hand to the child they re-execute.
const (
	addFindingChildPlan = "TPP_TEST_ADD_FINDING_PLAN"
	addFindingChildID   = "TPP_TEST_ADD_FINDING_ID"
)
