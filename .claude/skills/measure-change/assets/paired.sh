#!/usr/bin/env bash
# paired.sh — run one command against a base commit and against the working tree, then report what
# changed in stdout, stderr and exit status. This is the "before" half of a paired reading: the base
# side runs in a throwaway worktree at the base commit, so uncommitted work never leaks into it and
# the repository itself is never touched.
#
#   .claude/skills/measure-change/assets/paired.sh HEAD~1 'go test ./internal/plan -count=1' \
#       -n 's/[0-9]+\.[0-9]+s/<s>/'
#   .claude/skills/measure-change/assets/paired.sh origin/main 'bin/tsp check --base HEAD~1'
#
# -n/--normalize takes a sed -E expression, applied to both sides before the diff, and may repeat.
# Normalize only what the instrument says about itself — elapsed time, temporary paths, ports — never
# the assertion text: a normalizer that hides the payload turns a real difference into a green.
#
# The command runs through a shell, in the worktree root on one side and in the repository root on
# the other. Anything it writes into the tree it runs in lands in a temporary directory on the base
# side and in your working tree on the candidate side: pass a command that only reads, or that writes
# somewhere disposable.
#
# Exit: 0 the two sides are identical, 1 they differ, 2 setup failure, 3 identical because both sides
# failed the same way — that compared two failures, not two behaviours, and is no reading.
set -euo pipefail

sed_exprs=()
while [ $# -gt 0 ]; do
	case $1 in
	-n | --normalize)
		if [ $# -lt 2 ]; then
			echo "paired.sh: $1 needs a sed -E expression" >&2
			exit 2
		fi
		sed_exprs+=("$2")
		shift 2
		;;
	--)
		shift
		break
		;;
	-*)
		echo "paired.sh: unknown flag: $1" >&2
		exit 2
		;;
	*)
		break
		;;
	esac
done

if [ $# -lt 2 ]; then
	echo "usage: paired.sh [-n <sed -E expr>]... <base-ref> <command>" >&2
	exit 2
fi

base=$1
shift
command_text=$*

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
	echo "paired.sh: not inside a git worktree" >&2
	exit 2
}
if ! git -C "$root" rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
	echo "paired.sh: base ref not resolvable: $base" >&2
	exit 2
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/paired.XXXXXX")
cleanup() {
	git -C "$root" worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT

if ! git -C "$root" worktree add --detach --quiet "$tmp/base" "$base"; then
	echo "paired.sh: could not create a worktree at $base" >&2
	exit 2
fi

run_side() { # run_side <dir> <label>
	local dir=$1 label=$2
	set +e
	(cd "$dir" && eval "$command_text") >"$tmp/$label.out" 2>"$tmp/$label.err"
	echo $? >"$tmp/$label.code"
	set -e
}

normalize() { # normalize <file>
	local file=$1 expr
	if [ "${#sed_exprs[@]}" -eq 0 ]; then
		return 0
	fi
	for expr in "${sed_exprs[@]}"; do
		sed -E -e "$expr" "$file" >"$file.norm"
		mv "$file.norm" "$file"
	done
}

run_side "$tmp/base" base
run_side "$root" candidate
for side in base candidate; do
	normalize "$tmp/$side.out"
	normalize "$tmp/$side.err"
done

echo "paired: base=$base"
echo "paired: command=$command_text"
if [ "${#sed_exprs[@]}" -gt 0 ]; then
	echo "paired: normalized with ${sed_exprs[*]}"
fi
base_code=$(cat "$tmp/base.code")
candidate_code=$(cat "$tmp/candidate.code")
echo "paired: exit base=$base_code candidate=$candidate_code"

lines=${PAIRED_LINES:-40}
differing=0
for stream in out err code; do
	case $stream in
	out) name=stdout ;;
	err) name=stderr ;;
	code) name="exit status" ;;
	esac
	if ! diff -u "$tmp/base.$stream" "$tmp/candidate.$stream" >"$tmp/$stream.diff" 2>/dev/null; then
		differing=$((differing + 1))
		echo "paired: $name differs"
		head -n "$lines" "$tmp/$stream.diff"
		total=$(wc -l <"$tmp/$stream.diff")
		if [ "$total" -gt "$lines" ]; then
			echo "paired: ... $((total - lines)) more diff lines (PAIRED_LINES=$lines)"
		fi
	fi
done

if [ "$differing" -eq 0 ]; then
	if [ "$base_code" -ne 0 ]; then
		echo "paired: both sides exited $base_code: this compared two failures, not two behaviours"
		echo "paired: no reading"
		exit 3
	fi
	echo "paired: identical on stdout, stderr and exit status"
	exit 0
fi

echo "paired: $differing stream(s) differ"
exit 1
