import test from "node:test";
import assert from "node:assert/strict";
import { memoize } from "../src/memo.js";

test("reuses primitive keys and separates different primitive values", () => {
  let calls = 0;
  const lookup = memoize((value) => `${value}:${++calls}`);
  assert.equal(lookup("alpha"), "alpha:1");
  assert.equal(lookup("alpha"), "alpha:1");
  assert.equal(lookup("beta"), "beta:2");
});

test("refreshes a primitive-key result after the documented TTL", () => {
  let clock = 0;
  let source = "first";
  const lookup = memoize(() => source, { now: () => clock });
  assert.equal(lookup("account"), "first");
  source = "second";
  assert.equal(lookup("account"), "first");
  clock = 60_001;
  assert.equal(lookup("account"), "second");
});
