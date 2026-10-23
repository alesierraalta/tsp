# Breakcheck readiness report

## Candidate identity

- Project/root: `rdd-plus` at worktree `../rdd-plus-worktrees/ci-run-the-suite`
- Execution label: `breakcheck-ci-2026-09-18`
- Candidate: base `b210de6`. One new workflow (`.github/workflows/ci.yml`), three modified (`Makefile`, `README.md`, `docs/testing/dogfooding-log.md`) and this report, also new. Uncommitted at campaign time.
- Scope: whether the CI job actually runs what it claims and fails when it should. Not claimed: GitHub's own behaviour (the first real run is the proof), branch protection, or any other workflow.
- Environment and isolation: throwaway copies under `/tmp` at specific commits; probes run as shell commands and one compiled test binary with a reduced `PATH`. No model call, no repo writes, no GitHub mutations.

## Budget

- Probe budget: 5 probes. Five ran. Time budget: 20 minutes.

## Risks and probes

| Risk | Probe | Expectation | Observed | Evidence |
|---|---|---|---|---|
| **The claim this change exists for**: the job would not have caught the red merge | P1: the worker's command set (`go test ./... -count=1`) at commit `9ead735`, the merge that shipped without #107's guard | the suite fails there | `FAIL github.com/alesierraalta/rdd-plus/internal/bench 38.6s` … `exit 1` | probe P1 |
| `gofmt` is listed but does not fail the job | P2: an unformatted file in a clean copy, then the CI step | the step exits non-zero | step `test -z "$(gofmt -l .)"` → `exit=1` | probe P2 |
| The Makefile's own `vet` target hides the same thing | P2b: `make vet` with the Makefile of `master` on the same unformatted tree | it should fail; if it prints and succeeds, it is a trap | printed the file and **`exit=0`**; with this branch's Makefile, `exit=2` | probe P2b |
| The JavaScript half of the suite is skipped in silence when `node` is missing | P3: a compiled test binary that needs node, run with `PATH` reduced so `node` is absent | observe which it does — fail, or skip while the run still reports success | `node not on PATH` · `--- SKIP: TestScaffoldCopiesFixtureNeverTheKey` · **`PASS`, exit 0** — the skip does look green, which is why the workflow installs node | probe P3 |
| The workflow is malformed or triggers where it should not | P4: parse it and read its triggers | `push` on `master` and `pull_request`; one job running the steps in order | parses; triggers `['pull_request', 'push']`, `push.branches=['master']`; runner `ubuntu-latest`, timeout 20, seven steps in order `checkout`, `setup-go`, `setup-node`, `gofmt`, `vet`, `build`, `test` | probe P4 |

## Findings

None against the candidate. The probes that check what the workflow claims matched their expectations, and P3 turned a design intuition ("install node") into a measured fact — without it the suite reports success while skipping the JavaScript cases.

The independent verifier corrected this report on four points, all applied: the candidate inventory omitted this report and the modified dogfooding log, the step count read "five" where the job has seven, "every probe matched its expectation" was literally false because of that count, and the P3 row quoted `node not installed` where the test prints `node not on PATH`. None of them changed a probe result: the verifier reproduced the exit 1 at `9ead735` and the exit 0 at the base, and confirmed that no step swallows a failure.

Two things the campaign did **not** settle, recorded rather than assumed:

- **Go 1.26 availability.** `actions/setup-go` reads `go-version-file: go.mod`, which asks for `go 1.26`. The first real run settles it; if the manifest does not carry 1.26, the job fails at setup and that is a fact about the repo's toolchain, not about this file.
- **Action tags are not pinned to commit SHAs.** `actions/checkout@v4` and friends follow a mutable tag. Pinning SHAs is stricter supply-chain hygiene and is deliberately out of scope here: it trades readability for a guarantee this repository has not asked for. Recorded as an accepted risk.

## Omissions

| Risk omitted | Reason | Residual risk |
|---|---|---|
| An actual GitHub Actions run | It costs a push and a wait; the first run after merge is the real proof | A YAML detail GitHub rejects would only surface then; P4 covers syntax and shape only |
| Branch protection (making the check required) | A repository-settings mutation, not a file change; it is the user's call | The job can fail while a merge still proceeds, which is exactly how the red merge happened |
| Windows and macOS runners | The suite is Linux-oriented and the repo's fixtures assume a POSIX shell | None claimed |
| `-short` skipping integration tests in general | The runner does not use `-short`, and P3 measured the one conditional skip that matters here | Other `t.Skip` paths stay unmeasured |

## Readiness disposition

- Status: `PASS`, with two recorded unverified assumptions (Go 1.26 resolution, action pinning) and one omission that matters more than the rest.
- Rationale: the job runs the commands that catch the class of failure this repository actually suffered, the `gofmt` step fails as a step should, and the node dependency is installed because a measured skip would otherwise hide the JavaScript half of the suite.
- Accepted risks: the omissions above. The meaningful one is branch protection: **a CI that nobody requires still allows the merge that started this** — the file is necessary and not sufficient.
- Accepting authority: the maintainer.
- Residual risk: until a run happens, the workflow is verified as a specification and not as an execution.
