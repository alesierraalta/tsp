package findings

// SameSuite exposes the suite identity to the package's external tests.
var SameSuite = sameSuite

// SetJudgedCommitHook runs f after a round is judged and before record re-reads the commit, so a test can move HEAD
// while the round is judged.
func SetJudgedCommitHook(f func(top string)) { judgedCommitHook = f }

// SetNextJudgedHook replaces the seam Next uses to move HEAD while it judges.
func SetNextJudgedHook(hook func(top string)) { nextJudgedHook = hook }
