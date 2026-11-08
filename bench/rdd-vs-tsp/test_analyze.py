"""Tests for analyze.py: the decision rule of docs/testing/rdd-vs-tsp-protocol.md on synthetic readings."""
import contextlib
import gzip
import io
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import analyze  # noqa: E402

MODEL = "claude-haiku-5-5"


def write_run(root, case, run, detected, total=1, model=MODEL, failed=False, arm="A", caught=0):
    d = os.path.join(root, case, str(run))
    os.makedirs(d, exist_ok=True)
    with open(os.path.join(d, "result.json"), "w") as f:
        json.dump({"total": total, "detected": detected, "found": 0, "caught": caught,
                   "unmatched_findings": 0, "cost_usd": 0.01, "seconds": 1, "failed": failed}, f)
    if arm == "A":
        with open(os.path.join(d, "reviewers.jsonl"), "w") as f:
            f.write(json.dumps({"resolved_models": [model]}) + "\n")
    else:
        with gzip.open(os.path.join(d, "transcript-1.jsonl.gz"), "wt") as f:
            f.write(json.dumps({"type": "system", "model": model}) + "\n")


def reading(pattern, a_model=MODEL, b_failed=()):
    """pattern: list of (a_detected, b_detected) per case, one run each."""
    a, b = tempfile.mkdtemp(), tempfile.mkdtemp()
    for i, (x, y) in enumerate(pattern):
        case = f"c{i:02d}"
        write_run(a, case, 1, x, model=a_model if i == 0 else MODEL, arm="A")
        write_run(b, case, 1, y, arm="B", failed=i in b_failed)
    return a, b


def verdict(a, b):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = analyze.main(a, b)
    lines = out.getvalue().splitlines()
    return code, next(line for line in lines if line.startswith("verdict:")), out.getvalue()


class DecisionRule(unittest.TestCase):
    def test_an_arm_that_detects_what_the_other_misses_everywhere_is_better(self):
        code, v, _ = verdict(*reading([(1, 0)] * 20))
        self.assertEqual(code, 0)
        self.assertIn("RDD better at detection", v)

    def test_disjoint_detections_are_complementary(self):
        code, v, _ = verdict(*reading([(1, 0)] * 10 + [(0, 1)] * 10))
        self.assertIn("complementary", v)
        self.assertNotIn("better", v)

    def test_identical_detections_are_equivalent(self):
        code, v, _ = verdict(*reading([(1, 1)] * 10 + [(0, 0)] * 10))
        self.assertEqual(v, "verdict: equivalent at this sample size")

    def test_a_run_on_another_model_or_failed_in_one_arm_is_excluded_from_both(self):
        a, b = reading([(1, 0)] * 5 + [(0, 1)] * 5, a_model="claude-opus-5-5", b_failed=(9,))
        _, _, out = verdict(a, b)
        self.assertIn("paired valid case-runs: 8 defect", out)
        self.assertIn("excluded c00 #1: model", out)
        self.assertIn("excluded c09 #1: missing or invalid", out)


if __name__ == "__main__":
    unittest.main()
