#!/bin/sh
# run.sh runs the trigger harness against one variant of this case, without copying anything: it
# hands the Go toolchain an -overlay that replaces the defective files of fixture/ with the files of
# the variant, which is the same replacement the bench runner performs when it copies fix/<variant>
# over a copy of the fixture.
#
#   sh testdata/triggers/run.sh fixture     the project the agent sees (trigger.actual)
#   sh testdata/triggers/run.sh all         fix/all (trigger.expected)
#   sh testdata/triggers/run.sh keep-D1     every defect fixed except D1
#   sh testdata/triggers/run.sh keep-D2     every defect fixed except D2
#
# The same overlay argument runs the suite:
#   sh testdata/triggers/run.sh all --test
set -eu
case "$(basename "$PWD")" in
	w0*-*) case_dir="$PWD" ;;
	*) echo "run this from the case directory" >&2; exit 2 ;;
esac

variant="${1:-fixture}"
run_tests="${2:-}"
overlay="${TMPDIR:-/tmp}/tsp-$(basename "$case_dir")-$variant-overlay.json"

emit_overlay() {
	printf '{"Replace":{'
	first=1
	for file in $(cd "$case_dir" && find fixture -name '*.go' -type f | sort); do
		rel=${file#fixture/}
		if [ "$variant" = "fixture" ] || [ ! -f "$case_dir/fix/$variant/$rel" ]; then
			continue
		fi
		if [ "$first" -eq 1 ]; then first=0; else printf ','; fi
		printf '"%s/%s":"%s/fix/%s/%s"' "$case_dir" "$file" "$case_dir" "$variant" "$rel"
	done
	printf '}}\n'
}

if [ "$variant" != "fixture" ] && [ ! -d "$case_dir/fix/$variant" ]; then
	echo "no fix variant $variant" >&2
	exit 2
fi
emit_overlay >"$overlay"

cd "$case_dir/fixture"
if [ "$run_tests" = "--test" ]; then
	exec go test -overlay "$overlay" ./...
fi
exec go run -overlay "$overlay" ../testdata/triggers/main.go
