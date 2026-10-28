package findings

// Snapshot exposes the working-tree snapshot to the package's external tests.
var Snapshot = snapshot

// SetJudgedCommitHook runs f after a round is judged and before record re-reads the commit, so a test can move HEAD
// while the round is judged.
func SetJudgedCommitHook(f func(top string)) { judgedCommitHook = f }
