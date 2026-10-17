package bench

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RedCheckReport is what the red check found: which test files the agent added or changed, and which of the
// tests they introduce fail on the unmodified code (a witness) or pass there (they pin nothing).
type RedCheckReport struct {
	Suite       string   `json:"suite"`
	TestFiles   []string `json:"test_files"`
	Red         []string `json:"red"`
	PinsNothing []string `json:"pins_nothing"`
}

// RedCheck runs the tests added or changed in the repository holding dir against the code its HEAD commit
// holds, so an edit to the source never decides the verdict. A test is new when its name does not exist when
// HEAD's own tests run, which covers a new row added to an existing table test. An empty suite is inferred
// from the repository: the Go packages or Node files that hold the changed tests.
func RedCheck(dir, suite string, timeout time.Duration) (RedCheckReport, error) {
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
	if err := extractHead(root, head); err != nil {
		return rep, err
	}
	files, err := agentTestFiles(head, root)
	if err != nil {
		return rep, err
	}
	rep.TestFiles = files
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
	// Without one outcome per test name (a build failure, a runner the parser cannot read) nothing is known about
	// any single test, and a failed build must never read as a red witness.
	if _, wholeSuite := after["suite"]; len(after) == 0 || (len(after) == 1 && wholeSuite) {
		return rep, fmt.Errorf("the changed tests could not be run test by test with %q: %s", suite, tail(out, 600))
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
		return "python3 -m pytest " + strings.Join(py, " "), nil
	}
	if _, err := os.Stat(filepath.Join(root, "package.json")); err == nil {
		return "node --test " + strings.Join(files, " "), nil
	}
	return "", errors.New("cannot infer how to run the changed tests: pass --suite")
}

// extractHead writes the files HEAD records into dst, the code the agent's edits must not decide the verdict.
func extractHead(root, dst string) error {
	cmd := exec.Command("git", "-C", root, "archive", "--format=tar", "HEAD")
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
