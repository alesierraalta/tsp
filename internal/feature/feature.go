// Package feature is the registry of optional capabilities: what can be turned on, how it is
// asked, and whether it is on. A slice of declarations, never a plugin system.
package feature

import (
	"fmt"
	"time"

	"github.com/alesierraalta/tsp/internal/state"
)

// Feature is one optional capability the human can toggle.
type Feature struct {
	ID      string // "feedback"
	Title   string // "Feedback"
	Prompt  string // the exact question shown during install/configure
	Help    string // one line under the question
	Preview string // what turning it on does, shown before the toggle moves
	Default bool   // false: opt-in
}

// The ids a caller may name without duplicating a declaration's string.
const (
	HeadlessLoopID = "headless-loop"
	TelemetryID    = "telemetry"
)

var declarations = []Feature{
	{
		ID:      "feedback",
		Title:   "Feedback",
		Prompt:  "¿Quieres activar el feedback para ayudar a mejorar la herramienta?",
		Help:    "Records are anonymized and kept in two local files that are never publishable.",
		Preview: "Feedback records are anonymized in two local files (run-feedback.jsonl and run-feedback.md); both files are never publishable.",
		Default: false,
	},
	{
		// Opt-in and local-only: off writes nothing and creates nothing, so an install that never asks
		// for telemetry carries no directory, no salt and no ledger.
		ID:      TelemetryID,
		Title:   "Telemetry",
		Prompt:  "¿Quieres activar la telemetría de uso local para medir qué hace TSP en cada ejecución?",
		Help:    "Counts, durations and salted pseudonyms in one local file; no code, prompts or paths, and nothing is ever sent anywhere.",
		Preview: "Usage telemetry appends one JSON line per tsp run and one per host run the agent reports to <state root>/telemetry/events.jsonl: subcommand, flag names, exit code, wall time, token counts, model, tool counts and salted repository, machine and session pseudonyms. No code, prompt, finding text, flag value, positional argument or path is ever recorded, and the file stays local.",
		Default: false,
	},
	{
		ID:      "auto-run",
		Title:   "Automatic TSP run",
		Prompt:  "¿Quieres que una sesión donde el repositorio exige TSP lo ejecute sin preguntar?",
		Help:    "On, a required verdict starts the run; off, it asks first. A repository that leaves the decision to you is never affected.",
		Preview: "With the automatic run off, a required verdict asks the same question a repository that leaves the decision to you asks: Sí runs TSP, No declines it for that file.",
		Default: true,
	},
	{
		// Opt-in: the hard23 A/B of the headless loop read flat recall (70/91) at +91% cost, so driving the loop is
		// a session's own choice, never something every headless session pays for.
		ID:      HeadlessLoopID,
		Title:   "Headless TSP loop",
		Prompt:  "¿Quieres que una sesión sin interfaz arregle y vuelva a verificar sola lo que TSP encuentre?",
		Help:    "Driving fix, round and record with no UI costs more model turns: the loop is bounded at twice its round limit.",
		Preview: "On, a session without a UI keeps driving the TSP loop itself: it sends the fix, round and record continuations (and asks the agent to commit), at most twice the loop's round limit per session, so it costs more model turns. Off, a session without a UI stops after the TSP run.",
		Default: false,
	},
}

// All returns every feature in a stable order.
func All() []Feature {
	return append([]Feature(nil), declarations...)
}

// Get answers one feature by id.
func Get(id string) (Feature, bool) {
	for _, feature := range declarations {
		if feature.ID == id {
			return feature, true
		}
	}
	return Feature{}, false
}

// Enabled answers the persisted toggle; a state with no entry answers the feature's Default.
func Enabled(id string) (bool, error) {
	return enabled(id, state.Load)
}

// EnabledReadOnly answers the persisted toggle without resolving the state root by migration. A
// caller that must not move or create anything reads through here.
func EnabledReadOnly(id string) (bool, error) {
	return enabled(id, state.LoadReadOnly)
}

func enabled(id string, load func() (*state.State, error)) (bool, error) {
	declaration, ok := Get(id)
	if !ok {
		return false, unknownFeature(id)
	}
	current, err := load()
	if err != nil {
		return false, err
	}
	if saved, ok := current.Features[id]; ok {
		return saved.Enabled, nil
	}
	return declaration.Default, nil
}

// Set persists a toggle and reports the timestamp written.
func Set(id string, enabled bool) (string, error) {
	declaration, ok := Get(id)
	if !ok {
		return "", unknownFeature(id)
	}
	current, err := state.Load()
	if err != nil {
		return "", err
	}
	previous, hadPrevious := current.Features[id]
	wasEnabled := declaration.Default
	if hadPrevious {
		wasEnabled = previous.Enabled
	}
	if hadPrevious && wasEnabled == enabled {
		if _, err := current.Save(); err != nil {
			return "", err
		}
		return previous.UpdatedAt, nil
	}

	if current.Features == nil {
		current.Features = make(map[string]state.FeatureState)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	current.Features[id] = state.FeatureState{Enabled: enabled, UpdatedAt: timestamp}
	if _, err := current.Save(); err != nil {
		return "", err
	}
	return timestamp, nil
}

// Preview shows what turning the feature on would do, without turning it on.
func Preview(id string) (string, error) {
	declaration, ok := Get(id)
	if !ok {
		return "", unknownFeature(id)
	}
	return declaration.Preview, nil
}

func unknownFeature(id string) error {
	return fmt.Errorf("unknown feature %q", id)
}
