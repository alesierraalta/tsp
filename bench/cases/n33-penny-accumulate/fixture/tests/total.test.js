import assert from "node:assert/strict";
import test from "node:test";
import { formatRowAmount, makeRows, total } from "../src/total.js";

test("sums a few exact-cent amounts", () => {
  assert.equal(total([{ amount: "1.25" }, { amount: "2.75" }]), 4);
});

test("returns zero for an empty batch", () => {
  assert.equal(total([]), 0);
});

test("builds reusable penny rows", () => {
  assert.deepEqual(makeRows(3), [
    { id: 1, amount: "0.01" },
    { id: 2, amount: "0.01" },
    { id: 3, amount: "0.01" },
  ]);
});

test("rounds each displayed row to cents", () => {
  assert.equal(formatRowAmount("1.234"), "1.23");
});
