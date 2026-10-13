// Package bench runs the testing skill against fixture projects with sealed answer keys and
// scores what its persisted plan reports against those keys.
package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Trigger is the minimal reproduction recorded for a planted defect.
type Trigger struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Defect is one planted defect of a case; file paths are relative to the fixture root.
type Defect struct {
	ID          string   `json:"id"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Class       string   `json:"class"`
	Keywords    []string `json:"keywords"`
	Description string   `json:"description"`
	Trigger     Trigger  `json:"trigger"`
	WhyMissed   string   `json:"why_missed"`
}

// Trap is one documented behavior planted next to a case's real defects. A flow that reports a
// trap instead of probing against the fixture README earns a false positive: Documented names the
// README line that states the behavior is intentional. Traps are never defects, so nothing that
// counts, matches, or requires fixes for defects may look at them.
type Trap struct {
	ID          string   `json:"id"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Class       string   `json:"class"`
	Keywords    []string `json:"keywords"`
	Description string   `json:"description"`
	Documented  string   `json:"documented"`
}

// Key is the sealed answer key of a case: KEY.json next to the fixture directory.
type Key struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Suite    string `json:"suite"`
	Surface  string `json:"surface"`
	Control  string `json:"control,omitempty"`
	// Request is the bounded unit of work a run is asked to test, when the case has one. A case without
	// it is the generic case the bench has always run. The pointer is what tells the two apart: a key
	// that supplies a blank request is a mistake, not a generic case, and is refused rather than read
	// back as "no request".
	Request *string  `json:"request,omitempty"`
	Defects []Defect `json:"defects"`
	// Traps holds documented false-positive bait; optional, and read only by adjudication — never
	// by defect scoring, which must stay trap-blind by construction.
	Traps []Trap `json:"traps,omitempty"`
}

// RequestText is the case's bounded request, or "" when it declares none.
func (k Key) RequestText() string {
	if k.Request == nil {
		return ""
	}
	return strings.TrimSpace(*k.Request)
}

func controlProblems(control string, defectCount int) []string {
	switch control {
	case "":
		if defectCount == 0 {
			return []string{"no defects"}
		}
	case ControlClean:
		if defectCount > 0 {
			return []string{fmt.Sprintf("clean control has %d defect(s)", defectCount)}
		}
	default:
		return []string{fmt.Sprintf("control %q is not accepted; accepted value is %q", control, ControlClean)}
	}
	return nil
}

// KeyFile is the answer-key file name; it must never be copied into a workspace.
const KeyFile = "KEY.json"

const ControlClean = "clean"

// IsCleanControl reports whether the key describes a reviewed clean negative control.
func (k Key) IsCleanControl() bool {
	return k.Control == ControlClean
}

// LoadKey reads and validates a case's KEY.json.
func LoadKey(caseDir string) (Key, error) {
	raw, err := os.ReadFile(filepath.Join(caseDir, KeyFile))
	if err != nil {
		return Key{}, fmt.Errorf("read key: %w", err)
	}
	var k Key
	if err := json.Unmarshal(raw, &k); err != nil {
		return Key{}, fmt.Errorf("parse key: %w", err)
	}
	if err := k.Validate(); err != nil {
		return Key{}, fmt.Errorf("invalid key: %w", err)
	}
	return k, nil
}

// Validate rejects keys the scorer could not apply unambiguously.
func (k Key) Validate() error {
	var problems []string
	if strings.TrimSpace(k.ID) == "" {
		problems = append(problems, "id is empty")
	}
	if k.Language != "node" && k.Language != "go" {
		problems = append(problems, fmt.Sprintf("language %q is not node or go", k.Language))
	}
	if strings.TrimSpace(k.Suite) == "" {
		problems = append(problems, "suite is empty")
	}
	if k.Request != nil && k.RequestText() == "" {
		problems = append(problems, "request is present but empty")
	}
	problems = append(problems, controlProblems(k.Control, len(k.Defects))...)
	seen := map[string]bool{}
	for i, d := range k.Defects {
		switch {
		case strings.TrimSpace(d.ID) == "":
			problems = append(problems, fmt.Sprintf("defect %d has no id", i))
		case seen[d.ID]:
			problems = append(problems, fmt.Sprintf("defect id %q repeated", d.ID))
		}
		seen[d.ID] = true
		if strings.TrimSpace(d.File) == "" {
			problems = append(problems, fmt.Sprintf("defect %q has no file", d.ID))
		}
		if d.Line <= 0 {
			problems = append(problems, fmt.Sprintf("defect %q has no positive line", d.ID))
		}
		if len(d.Keywords) == 0 {
			problems = append(problems, fmt.Sprintf("defect %q has no keywords", d.ID))
		}
	}
	for i, tp := range k.Traps {
		switch {
		case strings.TrimSpace(tp.ID) == "":
			problems = append(problems, fmt.Sprintf("trap %d has no id", i))
		case seen[tp.ID]:
			problems = append(problems, fmt.Sprintf("trap id %q repeated", tp.ID))
		}
		seen[tp.ID] = true
		if strings.TrimSpace(tp.File) == "" {
			problems = append(problems, fmt.Sprintf("trap %q has no file", tp.ID))
		}
		if tp.Line <= 0 {
			problems = append(problems, fmt.Sprintf("trap %q has no positive line", tp.ID))
		}
		if len(tp.Keywords) == 0 {
			problems = append(problems, fmt.Sprintf("trap %q has no keywords", tp.ID))
		}
		if strings.TrimSpace(tp.Documented) == "" {
			problems = append(problems, fmt.Sprintf("trap %q has no documented line", tp.ID))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
