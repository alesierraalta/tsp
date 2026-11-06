#!/usr/bin/env python3
"""Analyze an RDD vs TSP reading (docs/testing/rdd-vs-tsp-protocol.md).

Usage: analyze.py <arm A results dir: gentle-review> <arm B results dir: TSP>

Reads <dir>/<case>/<run>/result.json for both arms, pairs them by case and run index, and prints the
protocol's measures with 95% bootstrap intervals over cases and the verdict the decision rule gives.
Standard library only.
"""
import glob
import gzip
import json
import os
import random
import sys

EXPECTED_MODEL = "claude-haiku-5-5"
RESAMPLES = 10_000
SEED = 372


def runs(root):
    out = {}
    for path in glob.glob(os.path.join(root, "*", "*", "result.json")):
        case_dir, run_dir = os.path.split(os.path.dirname(path))
        try:
            with open(path) as f:
                r = json.load(f)
        except (OSError, ValueError) as e:
            print(f"unreadable {path}: {e}", file=sys.stderr)
            continue
        r["_dir"] = os.path.dirname(path)
        out[(os.path.basename(case_dir), int(run_dir))] = r
    return out


def review_models(run_dir):
    models = set()
    try:
        with open(os.path.join(run_dir, "reviewers.jsonl")) as f:
            for line in f:
                models.update(json.loads(line).get("resolved_models") or ["unknown"])
    except (OSError, ValueError):
        return {"unknown"}
    return models or {"unknown"}


def stream_models(run_dir):
    models = set()
    for path in glob.glob(os.path.join(run_dir, "transcript-*.jsonl.gz")):
        with gzip.open(path, "rt", errors="replace") as f:
            for line in f:
                try:
                    ev = json.loads(line)
                except ValueError:
                    continue
                if ev.get("type") == "system" and ev.get("model"):
                    models.add(ev["model"])
                for m in (ev.get("modelUsage") or {}):
                    models.add(m)
    return models or {"unknown"}


def valid(r):
    return not r.get("failed") and not r.get("invalid")


def model_ok(models):
    return all(m.startswith(EXPECTED_MODEL) for m in models)


def bootstrap(cases, stat):
    rng = random.Random(SEED)
    values = []
    for _ in range(RESAMPLES):
        sample = [rng.choice(cases) for _ in cases]
        v = stat(sample)
        if v is not None:
            values.append(v)
    values.sort()
    if not values:
        return (None, None)
    return (values[int(0.025 * len(values))], values[int(0.975 * len(values)) - 1])


def fmt(x):
    return "n/a" if x is None else f"{x:.3f}"


def main(a_root, b_root):
    a, b = runs(a_root), runs(b_root)
    excluded = []
    pairs = {}
    for key in sorted(set(a) | set(b)):
        ra, rb = a.get(key), b.get(key)
        if ra is None or rb is None or not valid(ra) or not valid(rb):
            excluded.append((key, "missing or invalid in one arm"))
            continue
        ma, mb = review_models(ra["_dir"]), stream_models(rb["_dir"])
        if not model_ok(ma) or not model_ok(mb):
            excluded.append((key, f"model A={sorted(ma)} B={sorted(mb)}"))
            continue
        pairs[key] = (ra, rb)

    defect = {k: v for k, v in pairs.items() if (v[0].get("total") or 0) > 0}
    controls = {k: v for k, v in pairs.items() if (v[0].get("total") or 0) == 0}
    cases = sorted({k[0] for k in defect})

    def rates(sample):
        tot = da = db = du = 0
        for c in sample:
            for (case, _), (ra, rb) in defect.items():
                if case != c:
                    continue
                t = ra.get("total") or 0
                x, y = min(ra.get("detected") or 0, t), min(rb.get("detected") or 0, t)
                tot += t
                da += x
                db += y
                # One keyed defect per study case, so either arm detecting it detects the union.
                du += t if (x or y) and t == 1 else max(x, y)
        if tot == 0:
            return None
        return da / tot, db / tot, du / tot

    point = rates(cases)
    if point is None:
        print("no valid paired defect runs")
        return 1
    A, B, U = point
    M = max(A, B)
    diff_ci = bootstrap(cases, lambda s: (lambda r: None if r is None else r[0] - r[1])(rates(s)))
    gain_ci = bootstrap(cases, lambda s: (lambda r: None if r is None else r[2] - max(r[0], r[1]))(rates(s)))
    a_ci = bootstrap(cases, lambda s: (lambda r: None if r is None else r[0])(rates(s)))
    b_ci = bootstrap(cases, lambda s: (lambda r: None if r is None else r[1])(rates(s)))
    u_ci = bootstrap(cases, lambda s: (lambda r: None if r is None else r[2])(rates(s)))

    total = sum((ra.get("total") or 0) for ra, _ in defect.values())
    pinned = sum(min(rb.get("caught") or 0, rb.get("total") or 0) for _, rb in defect.values())
    backed = sum(min(rb.get("found") or 0, rb.get("total") or 0) for _, rb in defect.values())
    only_a = sum(1 for ra, rb in defect.values() if (ra.get("detected") or 0) and not (rb.get("detected") or 0))
    only_b = sum(1 for ra, rb in defect.values() if (rb.get("detected") or 0) and not (ra.get("detected") or 0))

    def mean(vals):
        vals = list(vals)
        return sum(vals) / len(vals) if vals else None

    print(f"paired valid case-runs: {len(defect)} defect, {len(controls)} control; excluded {len(excluded)}")
    for key, why in excluded:
        print(f"  excluded {key[0]} #{key[1]}: {why}")
    print(f"cases: {len(cases)}, defects in valid runs: {total}")
    print(f"detected  A (RDD) {fmt(A)} [{fmt(a_ci[0])}, {fmt(a_ci[1])}]")
    print(f"detected  B (TSP) {fmt(B)} [{fmt(b_ci[0])}, {fmt(b_ci[1])}]")
    print(f"detected  union   {fmt(U)} [{fmt(u_ci[0])}, {fmt(u_ci[1])}]")
    print(f"A - B             {fmt(A - B)} [{fmt(diff_ci[0])}, {fmt(diff_ci[1])}]")
    print(f"union - max(A,B)  {fmt(U - M)} [{fmt(gain_ci[0])}, {fmt(gain_ci[1])}]")
    print(f"case-runs detected only by A: {only_a}, only by B: {only_b}")
    print(f"TSP pinned {pinned}/{total}, evidence-backed {backed}/{total}")
    fp_a = fmt(mean(ra.get("unmatched_findings") or 0 for ra, _ in controls.values()))
    fp_b = fmt(mean(rb.get("unmatched_findings") or 0 for _, rb in controls.values()))
    print(f"false positives on controls: A {fp_a}, B {fp_b} (findings per control run)")
    cost_a = fmt(mean(ra.get("cost_usd") or 0 for ra, _ in pairs.values()))
    cost_b = fmt(mean(rb.get("cost_usd") or 0 for _, rb in pairs.values()))
    sec_a = fmt(mean(ra.get("seconds") or 0 for ra, _ in pairs.values()))
    sec_b = fmt(mean(rb.get("seconds") or 0 for _, rb in pairs.values()))
    print(f"cost per case-run US$: A {cost_a}, B {cost_b}; seconds: A {sec_a}, B {sec_b}")

    verdicts = []
    if diff_ci[0] is not None and (diff_ci[0] > 0 or diff_ci[1] < 0):
        verdicts.append("RDD better at detection" if A > B else "TSP better at detection")
    if U - M >= 0.10 and gain_ci[0] is not None and gain_ci[0] > 0:
        verdicts.append("complementary")
    if not verdicts:
        verdicts.append("equivalent at this sample size")
    print("verdict: " + "; ".join(verdicts))
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(2)
    sys.exit(main(sys.argv[1], sys.argv[2]))
