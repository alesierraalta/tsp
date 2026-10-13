import assert from "node:assert/strict";
import test from "node:test";
import { truncate } from "../src/truncate.js";

test("keeps text that fits the requested character limit", () => {
  assert.equal(truncate("worker ready", 20), "worker ready");
});

test("truncates ordinary single-unit text", () => {
  assert.equal(truncate("worker ready", 6), "worker");
});

test("strips one trailing LF before truncating", () => {
  assert.equal(truncate("worker ready\n", 20), "worker ready");
});
