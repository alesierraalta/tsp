package bench

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/hookcmd"
)

// RedCheckReport is what the red check found: which test files the agent added or changed, and which of the
// tests they introduce fail on the unmodified code (a witness) or pass there (they pin nothing).
type RedCheckReport struct {
	Suite       string        `json:"suite"`
	Uncollected []Uncollected `json:"uncollected,omitempty"`
	TestFiles   []string      `json:"test_files"`
	Red         []string      `json:"red"`
	PinsNothing []string      `json:"pins_nothing"`
}

// RedCheck runs the tests added or changed in the repository holding dir against the code its HEAD commit
// holds, so an edit to the source never decides the verdict. A test is new when its name does not exist when
// HEAD's own tests run, which covers a new row added to an existing table test. An empty suite is inferred
// from the repository: the Go packages or Node files that hold the changed tests.
func RedCheck(dir, suite string, timeout time.Duration) (RedCheckReport, error) {
	return redCheckAt(dir, suite, "HEAD", nil, timeout)
}

// RedCheckRange asks the same question for a commit range: the candidate is `base...HEAD`, so the code
// the change started from is base, and the tests are the ones the range touched. Reading HEAD's tree
// instead would compare a commit against itself and report that no test came with it.
func RedCheckRange(dir, suite, base string, changed []string, timeout time.Duration) (RedCheckReport, error) {
	tests := make([]string, 0, len(changed))
	for _, p := range changed {
		if isTestFile(p) {
			tests = append(tests, p)
		}
	}
	if len(tests) == 0 {
		return RedCheckReport{Suite: suite, TestFiles: []string{}}, nil
	}
	return redCheckAt(dir, suite, base, tests, timeout)
}

func redCheckAt(dir, suite, rev string, agentFiles []string, timeout time.Duration) (RedCheckReport, error) {
	var rep RedCheckReport
	top, err := gitRun(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return rep, fmt.Errorf("not inside a git repository: %s", strings.TrimSpace(top))
	}
	root := strings.TrimSpace(top)
	head, err := os.MkdirTemp("", "tsp-redcheck-")
	if err != nil {
		return rep, err
	}
	defer removeTree(head)
	if err := extractRev(root, rev, head); err != nil {
		return rep, err
	}
	files := agentFiles
	if files == nil {
		var err error
		files, err = agentTestFiles(head, root)
		if err != nil {
			return rep, err
		}
	}
	rep.TestFiles = files
	if argv, _, e := perTestCommand(suite); e == nil && len(argv) > 1 && argv[0] == "cargo" && argv[1] == "test" {
		_, out := runCapture(root, argv, timeout)
		unsupported, sawFile := false, false
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 2 && fields[0] == "Running" {
				file := fields[1]
				if file == "unittests" {
					file = fields[2]
				}
				unsupported, sawFile = !isTestFile(file), true
			}
			if m := cargoLine.FindStringSubmatch(strings.TrimSpace(line)); unsupported && m != nil && m[2] == "FAILED" {
				return rep, fmt.Errorf("%s: inline Rust unit tests are not supported; move the test to tests/", m[1])
			}
		}
		// --quiet hides Cargo's file headings. Conservatively diagnose a failing
		// name also defined as a unit test outside the supported test paths.
		if !sawFile {
			failures := parseCargo(out)
			for _, line := range strings.Split(out, "\n") {
				if name, failed := strings.CutSuffix(strings.TrimSpace(line), " --- FAILED"); failed {
					failures[name] = false
				}
			}
			var inline string
			_ = filepath.WalkDir(root, func(p string, entry os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if entry.IsDir() {
					if entry.Name() == "target" || entry.Name() == ".git" || entry.Name() == "node_modules" {
						return filepath.SkipDir
					}
					return nil
				}
				rel, _ := filepath.Rel(root, p)
				if !strings.HasSuffix(rel, ".rs") || isTestFile(rel) {
					return nil
				}
				data, err := os.ReadFile(p)
				if err != nil {
					return nil
				}
				for _, m := range cargoUnitTest.FindAllStringSubmatch(string(data), -1) {
					for name, passed := range failures {
						if !passed && (name == m[1] || strings.HasSuffix(name, "::"+m[1])) && (inline == "" || name < inline) {
							inline = name
						}
					}
				}
				return nil
			})
			if inline != "" {
				return rep, fmt.Errorf("%s: inline Rust unit tests are not supported; move the test to tests/", inline)
			}
		}
	}
	if len(files) == 0 {
		return rep, nil
	}
	if suite == "" {
		if suite, err = inferRedCheckSuite(root, files); err != nil {
			return rep, err
		}
	}
	rep.Suite = suite
	// HEAD's own tests on HEAD's code: every name here existed before the agent wrote anything. A suite that
	// cannot run at HEAD (a package the agent created) leaves the set empty, and every test it then runs is new.
	before, _, _, _ := perTest(head, suite, timeout)
	after, _, out, err := testsOn(head, "", root, files, suite, timeout)
	if err != nil {
		return rep, err
	}
	rep.Uncollected = pytestUncollected(out)
	// Without one outcome per test name (a build failure, a runner the parser cannot read) nothing is known about
	// any single test, and a failed build must never read as a red witness.
	if _, wholeSuite := after["suite"]; len(after) == 0 || (len(after) == 1 && wholeSuite) {
		detail := tail(out, 600)
		if len(rep.Uncollected) > 0 {
			var diagnostics []string
			for _, file := range rep.Uncollected {
				diagnostics = append(diagnostics, "could not collect: "+file.File+" ("+file.Reason+")")
			}
			detail = strings.Join(diagnostics, "; ") + `; select the project environment with --suite "<venv>/bin/python -m pytest ..."`
		}
		return rep, fmt.Errorf("the changed tests could not be run test by test with %q: %s", suite, detail)
	}
	for name, passed := range after {
		if newParentCovers(after, before, name) {
			continue
		}
		if hasOwnName(before, name) {
			// A test that existed keeps its name when the agent adds a row to it without a subtest. It passed at
			// HEAD and fails with the agent's version on the same code: the agent's change made it red.
			if before[name] && !passed {
				rep.Red = append(rep.Red, name)
			}
			continue
		}
		if passed {
			rep.PinsNothing = append(rep.PinsNothing, name)
		} else {
			rep.Red = append(rep.Red, name)
		}
	}
	sort.Strings(rep.Red)
	sort.Strings(rep.PinsNothing)
	return rep, nil
}

var cargoUnitTest = regexp.MustCompile(`(?s)#\[\s*test\s*\]\s*(?:#\[[^\]]*\]\s*)*(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z_0-9]*)\s*\(`)

// hasOwnName reports whether name ran at HEAD, whatever its outcome there.
func hasOwnName(before map[string]bool, name string) bool {
	_, ok := before[name]
	return ok
}

// newParentCovers reports whether name is a subtest whose parent is itself new: the parent's verdict already
// says everything about it. A new subtest under a parent that existed at HEAD is judged on its own.
func newParentCovers(after, before map[string]bool, name string) bool {
	parent := parentTest(name)
	if parent == "" {
		return false
	}
	_, existed := before[parent]
	_, ran := after[parent]
	return ran && !existed
}

// parentTest is the top-level test a subtest belongs to: "pkg.TestX/case" and "describe/it" both name their
// parent before the first slash that follows the test name, never a slash inside a Go package path.
func parentTest(name string) string {
	start := 0
	if i := strings.Index(name, ".Test"); i >= 0 {
		start = i + 1
	}
	j := strings.Index(name[start:], "/")
	if j < 0 {
		return ""
	}
	return name[:start+j]
}

// inferRedCheckSuite picks the command that runs the changed tests: their Go packages, the Python files under
// pytest, or the Node files.
func inferRedCheckSuite(root string, files []string) (string, error) {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		seen := map[string]bool{}
		var pkgs []string
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") {
				continue
			}
			pkg := "./" + path.Dir(f)
			if pkg == "./." {
				pkg = "."
			}
			if !seen[pkg] {
				seen[pkg] = true
				pkgs = append(pkgs, pkg)
			}
		}
		if len(pkgs) > 0 {
			sort.Strings(pkgs)
			return "go test -count=1 " + strings.Join(pkgs, " "), nil
		}
	}
	var py []string
	for _, f := range files {
		if strings.HasSuffix(f, ".py") {
			py = append(py, f)
		}
	}
	if len(py) > 0 {
		return pytestInterpreter(root, py) + " -m pytest " + strings.Join(py, " "), nil
	}
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct{ Scripts map[string]string }
		if json.Unmarshal(data, &pkg) == nil {
			words, err := hookcmd.ShellWords(pkg.Scripts["test"])
			if err == nil && len(words) > 0 {
				if words[0] == "npx" {
					words = words[1:]
				}
				if len(words) > 0 {
					switch words[0] {
					case "jest":
						return "npx jest " + strings.Join(files, " "), nil
					case "vitest":
						return "npx vitest run " + strings.Join(files, " "), nil
					}
				}
			}
		}
		return "node --test " + strings.Join(files, " "), nil
	}
	return "", errors.New("cannot infer how to run the changed tests: pass --suite")
}

// pytestInterpreter uses the nearest repository environment, keeping its absolute path
// because the red check runs against extracted trees without the environment itself.
func pytestInterpreter(root string, files []string) string {
	best, bestDistance := "", int(^uint(0)>>1)
	_ = filepath.WalkDir(root, func(p string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		depth := 0
		if rel != "." {
			depth = len(strings.Split(rel, string(filepath.Separator)))
		}
		if entry.Name() == ".git" || entry.Name() == "node_modules" || depth > 3 {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(p, "pyvenv.cfg")); err != nil {
			return nil
		}
		for _, binary := range []string{"bin/python", "Scripts/python.exe"} {
			candidate := filepath.Join(p, filepath.FromSlash(binary))
			if st, err := os.Stat(candidate); err != nil || st.IsDir() {
				continue
			}
			distance := int(^uint(0) >> 1)
			for _, file := range files {
				r, err := filepath.Rel(filepath.Dir(filepath.Join(root, file)), p)
				if err != nil {
					continue
				}
				d := 0
				if r != "." {
					d = len(strings.Split(r, string(filepath.Separator)))
				}
				if d < distance {
					distance = d
				}
			}
			if distance < bestDistance || (distance == bestDistance && (best == "" || candidate < best)) {
				best, bestDistance = candidate, distance
			}
		}
		return filepath.SkipDir
	})
	if best == "" {
		return "python3"
	}
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(best, "\\", "\\\\"), "\"", "\\\"") + "\""
}

// extractHead writes the files HEAD records into dst, the code the agent's edits must not decide the verdict.
func extractHead(root, dst string) error { return extractRev(root, "HEAD", dst) }

// extractRev writes the tree rev records in the repository at root into dst, without its history.
func extractRev(root, rev, dst string) error {
	cmd := exec.Command("git", "-C", root, "archive", "--format=tar", rev)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	tr := tar.NewReader(stdout)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = cmd.Wait()
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(filepath.Separator)) {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.Symlink(h.Linkname, target)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode).Perm()|0o200)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive HEAD: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
