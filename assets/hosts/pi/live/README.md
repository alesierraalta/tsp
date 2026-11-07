# Live proof of the Pi TSP extension

Runs the real Pi, the real `assets/hosts/pi/tsp.ts` and a real `tsp` binary against a scripted model, so the offer, the answers and the silent cases are observed in a live session without network or cost. Findings: [docs/pi-live-proof.md](../../../../docs/pi-live-proof.md).

```sh
make build
TSP_BIN=./bin/tsp node assets/hosts/pi/live/run.mjs            # every case
TSP_BIN=./bin/tsp node assets/hosts/pi/live/run.mjs "No is"    # cases whose name contains the text
PI_LIVE_WITHOUT_TSP=1 TSP_BIN=./bin/tsp node assets/hosts/pi/live/run.mjs "offer fires for an edit"   # control: must FAIL
PI_LIVE_EXTENSION=/path/to/older/tsp.ts TSP_BIN=./bin/tsp node assets/hosts/pi/live/run.mjs        # another extension build, to see a case fail on it
```

Needs `pi` on PATH. It is not part of `go test` or CI because it drives a real Pi.

- `scripted-model.ts` registers a `scripted` provider whose answers come from `PI_LIVE_SCRIPT`: text, a tool call, or a hang that ends as aborted.
- `run.mjs` builds a scratch git repository per case, isolates Pi (`PI_CODING_AGENT_DIR`, `XDG_CONFIG_HOME`, `--no-extensions`, `--no-session`) and puts a logging `tsp` wrapper first on PATH, which is how a silent extension is told apart from a broken one.
- A case with `then` runs a second Pi process on the same stored session (`--continue`), which is how a resume is observed.
- A case with `gap: "#N"` documents behaviour the extension does not have yet. It passes while the gap is open and fails as `GAP CLOSED` when the wanted behaviour appears, so the marker is removed by the change that closes it.
- A case counts only if the session settled and the repository really changed; a script that changed nothing would otherwise fake a gap.
