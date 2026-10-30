#!/usr/bin/env bash
# Asserts paired.sh's contract: 0 identical, 1 differ, 2 setup failure, 3 both sides failed.
# Run after editing the script: assets/selftest.sh from the repository root.
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
paired="$here/paired.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0

expect() { # expect <name> <expected-exit> <args...>
	local name=$1 want=$2
	shift 2
	"$paired" "$@" >"$tmp/out" 2>&1
	local got=$?
	if [ "$got" -eq "$want" ]; then
		echo "ok   $name (exit $got)"
	else
		echo "FAIL $name: want exit $want, got $got"
		sed 's/^/     /' "$tmp/out"
		fails=$((fails + 1))
	fi
}

# A command that cannot differ between the two sides.
expect "identical output" 0 HEAD 'echo same'

# The base side runs in a throwaway worktree, so its path always differs from this one.
expect "differing output" 1 HEAD 'pwd'

# The base side is checked out at the ref, not at whatever this worktree holds.
expect "base runs at the ref" 0 HEAD 'git rev-parse --short HEAD'

# Both sides fail the same way: compared two failures, not two behaviours.
expect "both sides fail" 3 HEAD 'exit 7'

expect "unknown flag" 2 --nope HEAD 'true'
expect "unresolvable ref" 2 'no-such-ref-for-selftest' 'echo same'

# Instrument noise is normalized away; assertions are not.
if command -v go >/dev/null && [ -f go.mod ]; then
	expect "instrument noise normalized" 0 -n 's/[0-9]+\.[0-9]+s/<s>/' HEAD 'go test ./internal/plan -count=1'
else
	echo "skip instrument noise normalized (no go toolchain or not at the repository root)"
fi

echo
if [ "$fails" -eq 0 ]; then
	echo "selftest: all contracts hold"
else
	echo "selftest: $fails contract(s) broken"
	exit 1
fi
